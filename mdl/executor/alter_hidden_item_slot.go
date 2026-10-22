// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// objectListItemValuer is the page mutator capability the hidden-slot check
// needs; engines without it are not checked.
type objectListItemValuer interface {
	ObjectListItemValues(itemRef string) (backend.ObjectListItemValues, bool)
}

// refuseHiddenItemSlot refuses an INSERT or REPLACE that writes widgets into a
// slot of an object-list item that the widget's own configuration hides — an
// Accordion group's headerContent while headerRenderMode is 'text'. Such
// widgets are stored but never rendered, and the project as stored fails
// with CE0463 at the widget (PROPOSAL_alter_object_list_item_address.md).
// A bare item address writes into the body slot, so that is checked too.
//
// Called by exec and by check's target validation, so the refusal reaches
// the author before anything is written.
func refuseHiddenItemSlot(ctx *ExecContext, mutator any, ref ast.WidgetRef) error {
	if ref.Item == nil {
		return nil
	}
	valuer, ok := mutator.(objectListItemValuer)
	if !ok {
		return nil
	}
	vals, ok := valuer.ObjectListItemValues(ref.Widget)
	if !ok {
		return nil
	}
	registry := ctx.GetWidgetRegistry()
	if registry == nil {
		return nil
	}
	def, ok := registry.GetByWidgetID(vals.WidgetID)
	if !ok {
		return nil
	}
	slot := ref.Column
	if slot == "" {
		slot = vals.BodySlot
	}
	return itemSlotHiddenError(visibilityRulesFor(def, registry), ref.Widget, vals.ListKey, slot, vals.Item, vals.Widget)
}

// itemSlotHiddenError evaluates the nested visibility rules for one item slot.
// A rule that cannot be decided from the stored values refuses nothing.
func itemSlotHiddenError(rules []types.WidgetVisibilityRule, itemRef, listKey, slot string,
	itemValues, widgetValues map[string]string) error {
	for _, rule := range rules {
		if !rule.Nested() || rule.HiddenWhen == nil ||
			!strings.EqualFold(rule.ListPropertyKey, listKey) || !strings.EqualFold(rule.PropertyKey, slot) {
			continue
		}
		fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			values := widgetValues
			if c.Scope == types.ConditionScopeItem {
				values = itemValues
			}
			v, ok := values[strings.ToLower(c.PropertyKey)]
			return v, ok
		})
		if !determinable || !fires {
			continue
		}
		var why []string
		for _, c := range rule.Conditions() {
			v := itemValues[strings.ToLower(c.PropertyKey)]
			if c.Scope != types.ConditionScopeItem {
				v = widgetValues[strings.ToLower(c.PropertyKey)]
			}
			why = append(why, fmt.Sprintf("%s is '%s'", c.PropertyKey, v))
		}
		return fmt.Errorf("cannot write widgets into %s.%s: %s is hidden while %s, so the widgets would be stored "+
			"but not rendered, and the widget fails with CE0463 — change %s on %s first (`set (…) on %s`)",
			itemRef, strings.ToLower(slot), rule.PropertyKey, strings.Join(why, " and "),
			conditionKeys(rule), itemRef, itemRef)
	}
	return nil
}

func conditionKeys(rule types.WidgetVisibilityRule) string {
	var keys []string
	for _, c := range rule.Conditions() {
		keys = append(keys, c.PropertyKey)
	}
	return strings.Join(keys, ", ")
}

// refuseHiddenItemSlotFor is refuseHiddenItemSlot for the operations that
// write widgets into a target: INSERT INTO and REPLACE of an item slot.
func refuseHiddenItemSlotFor(ctx *ExecContext, mutator any, op ast.AlterPageOperation, ref ast.WidgetRef) error {
	switch o := op.(type) {
	case *ast.InsertWidgetOp:
		if strings.EqualFold(o.Position, "INTO") {
			return refuseHiddenItemSlot(ctx, mutator, ref)
		}
	case *ast.ReplaceWidgetOp:
		if ref.Column != "" {
			return refuseHiddenItemSlot(ctx, mutator, ref)
		}
	}
	return nil
}
