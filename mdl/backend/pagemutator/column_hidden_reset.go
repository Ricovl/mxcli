// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// A column property the widget hides must hold its declared default.
//
// `set (ColumnWidth: manual, Size: 220)` and then `set (ColumnWidth: autoFill)`
// left `size` at 220. Data grid 2 hides `size` unless `width` is "manual"
// (editorConfig: `"manual"!==n.width&&hidePropertyIn(…,"columns",o,"size")`),
// and a hidden property holding anything but its default is CE0463 (measured on
// 11.6.6 and 11.15.0, mendixlabs/mxcli#1519). The create path writes hidden
// properties at their default; the ALTER setter changed one property and left
// the ones that depend on it alone.
//
// Which properties depend on which is the WIDGET's to say, so the rules are the
// editorConfig's own (lifted by the caller from the installed .mpk), and the
// defaults are the ones the stored Type declares — nothing here names `size`.

// ResetHiddenColumnProperties brings a grid column back in line with its
// widget's visibility rules after a SET: every primitive property a rule now
// hides is reset to its declared default. A property the statement itself named
// (setProps, by MDL name) is never silently reset — holding a non-default value
// while hidden, it is refused instead, as MDL-WIDGET10 refuses it on create.
//
// It returns the schema keys it reset. Rules it cannot decide (a condition on a
// property it cannot read) change nothing.
func (m *Mutator) ResetHiddenColumnProperties(gridRef, columnRef string, rules []types.WidgetVisibilityRule, setProps []string) ([]string, error) {
	col, err := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if err != nil {
		return nil, err
	}
	grid := m.widgetFinder(m.rawData, gridRef)
	if grid == nil {
		return nil, fmt.Errorf("widget %q not found", gridRef)
	}
	listKey := types.DataGridColumnsKey
	defaults := columnPropDefaults(grid.widget, listKey)
	explicit := make(map[string]bool, len(setProps))
	for _, p := range setProps {
		if key := resolveColumnPropertyKey(p, col.colPropKeys); key != "" {
			explicit[key] = true
		}
	}
	return resetHiddenItemProps(col.widget, col.colPropKeys, col.colPropKinds, defaults,
		primitiveValuesByKey(grid.widget, buildPropKeyMap(grid.widget)), listKey, rules, explicit)
}

// resetHiddenItemProps is the document-level half, separate so it can be tested
// without a page. itemKeys/itemKinds map TypePointer ids to schema keys and
// value kinds; defaults and widgetValues are keyed by schema key.
func resetHiddenItemProps(item bson.D, itemKeys, itemKinds, defaults, widgetValues map[string]string,
	listKey string, rules []types.WidgetVisibilityRule, explicit map[string]bool) ([]string, error) {
	reset := map[string]bool{}
	// A reset can hide another property in turn (`minWidth` back to "auto"
	// hides `minWidthLimit`), so repeat until nothing changes. Each pass only
	// ever moves a value TO its default, so this terminates.
	for pass := 0; pass < len(rules)+1; pass++ {
		values := primitiveValuesByKey(item, itemKeys)
		changed := false
		for _, rule := range rules {
			if rule.ListPropertyKey != listKey || rule.HiddenWhen == nil {
				continue
			}
			fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
				src := widgetValues
				if c.Scope == types.ConditionScopeItem {
					src = values
				}
				v, ok := src[c.PropertyKey]
				return v, ok
			})
			if !determinable || !fires {
				continue
			}
			key := rule.PropertyKey
			def, hasDefault := defaults[key]
			cur, isPrimitive := values[key]
			if !hasDefault || !isPrimitive || cur == def {
				continue
			}
			if explicit[key] {
				return nil, fmt.Errorf(
					"column property %q is hidden under this column's configuration (%s), so it must keep its default %q — "+
						"a hidden property with another value fails the build with CE0463",
					key, describeConditions(rule), def)
			}
			if !setPrimitiveByKey(item, itemKeys, itemKinds, key, def) {
				continue
			}
			reset[key] = true
			changed = true
		}
		if !changed {
			break
		}
	}
	out := make([]string, 0, len(reset))
	for k := range reset {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func describeConditions(rule types.WidgetVisibilityRule) string {
	parts := []string{}
	for _, c := range rule.Conditions() {
		switch c.Operator {
		case "eq":
			parts = append(parts, fmt.Sprintf("%s is %q", c.PropertyKey, c.Value))
		case "ne":
			parts = append(parts, fmt.Sprintf("%s is not %q", c.PropertyKey, c.Value))
		default:
			parts = append(parts, fmt.Sprintf("%s %s %s", c.PropertyKey, c.Operator, c.Value))
		}
	}
	return strings.Join(parts, " and ")
}

// primitiveValuesByKey reads every primitive-valued property of a widget object
// (or of the widget's Object), keyed by schema key.
func primitiveValuesByKey(doc bson.D, keys map[string]string) map[string]string {
	obj := doc
	if o := bsonnav.DGetDoc(doc, "Object"); o != nil {
		obj = o
	}
	out := map[string]string{}
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		key := keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))]
		if key == "" {
			continue
		}
		val := bsonnav.DGetDoc(pd, "Value")
		if val == nil {
			continue
		}
		if s, ok := bsonnav.DGet(val, "PrimitiveValue").(string); ok {
			out[key] = s
		}
	}
	return out
}

// setPrimitiveByKey writes a primitive property of a widget object by schema key.
func setPrimitiveByKey(item bson.D, keys, kinds map[string]string, key, value string) bool {
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(item, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))
		if keys[id] != key {
			continue
		}
		if field, _ := columnValueField(kinds[id]); field != "PrimitiveValue" {
			return false
		}
		val := bsonnav.DGetDoc(pd, "Value")
		return val != nil && bsonnav.DSet(val, "PrimitiveValue", value)
	}
	return false
}

// columnPropDefaults reads the declared DefaultValue of every property of the
// object list listKey from the stored widget Type, keyed by schema key.
func columnPropDefaults(widgetDoc bson.D, listKey string) map[string]string {
	out := map[string]string{}
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(widgetDoc, "Type"), "ObjectType")
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok || bsonnav.DGetString(ptDoc, "PropertyKey") != listKey {
			continue
		}
		itemType := bsonnav.DGetDoc(bsonnav.DGetDoc(ptDoc, "ValueType"), "ObjectType")
		for _, cpt := range bsonnav.DGetArrayElements(bsonnav.DGet(itemType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := bsonnav.DGetString(cptDoc, "PropertyKey")
			vt := bsonnav.DGetDoc(cptDoc, "ValueType")
			if key == "" || vt == nil {
				continue
			}
			if d, ok := bsonnav.DGet(vt, "DefaultValue").(string); ok {
				out[key] = d
			}
		}
	}
	return out
}
