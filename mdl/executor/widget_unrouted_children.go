// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A pluggable widget's body is distributed over four passes in buildPluggable —
// declared child slots, object lists, and two auto-discovery passes — and every
// one of them SKIPS a child it does not recognise. Nothing then reported the
// skip, so a child that matched none of the four was built and thrown away:
//
//	datagrid dg (...) {
//	  column c (attribute: Name) filter f { textfilter tf (attribute: Name) }
//	}
//
// `filter` is a Gallery keyword (widgetSlotKeywordOverrides spells the same
// DataGrid property `controlbar`), and the grammar reads that line as a COLUMN
// with no body followed by a SIBLING filter widget — so it landed among the
// grid's own children, matched nothing, and vanished. `mxcli check` was silent,
// `exec` succeeded, and `DESCRIBE PAGE` showing a column with no filter was the
// only way to find out. That spelling is what `mxcli syntax page widgets`
// documented, so it was also the likeliest thing for an author to write.
//
// The drop is not datagrid-specific — it is one `continue` per pass, shared by
// every pluggable widget — which is why this is a rule about children and slots
// rather than a special case for filters.

// unroutedPluggableChildren returns the direct children of w that the engine
// has nowhere to put, given the parent's definition.
//
// It is deliberately CONSERVATIVE: it reports only what it can prove from the
// definition alone, so that `check` (which has the definition) never claims a
// drop that `exec` (which has the definition AND the widget template) would not
// make. The bail-outs below are each a case where a child may still be routed
// by information this function cannot see.
func unroutedPluggableChildren(def *WidgetDefinition, w *ast.WidgetV3) []*ast.WidgetV3 {
	if def == nil || w == nil || len(w.Children) == 0 {
		return nil
	}
	// No declared child slots: applyChildSlots returns early and the auto pass
	// hands every unmatched child to the first widgets-typed property it finds.
	// Something may well take them, and this function cannot see what.
	//
	// A definition that declares its slots per MODE (a combo box's custom
	// content, ako/mxcli#1744) is not that case, even in a mode with none: it
	// says where widgets go, and the auto pass would put them in another
	// mode's slot — a property the widget does not render in this mode.
	if !declaresChildSlots(def) {
		return nil
	}
	slots, _ := selectedChildSlots(def, w)
	// A `template` slot IS the catch-all — applyChildSlots assigns leftover
	// children to it (defaultSlotContainer). A Gallery has one, which is why an
	// arbitrary widget in a gallery body is fine and the same widget in a
	// datagrid body is not.
	if hasDefaultChildSlot(slots) {
		return nil
	}
	var out []*ast.WidgetV3
	for _, c := range w.Children {
		if c == nil || defRoutesChild(def, slots, c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// selectedChildSlots is the child slots a widget is built with — what
// selectMappings hands applyChildSlots: the definition's own, or, for a
// definition with modes, the selected mode's (nil when no mode can be
// selected, which the build refuses by itself). mode is nil without modes.
func selectedChildSlots(def *WidgetDefinition, w *ast.WidgetV3) ([]ChildSlotMapping, *WidgetMode) {
	if len(def.Modes) == 0 {
		return def.ChildSlots, nil
	}
	mode, err := (&PluggableWidgetEngine{}).selectMode(def, w)
	if err != nil || mode == nil {
		return nil, nil
	}
	return mode.ChildSlots, mode
}

// declaresChildSlots reports whether the definition declares a child slot
// anywhere — on itself or in any of its modes.
func declaresChildSlots(def *WidgetDefinition) bool {
	if len(def.ChildSlots) > 0 {
		return true
	}
	for _, m := range def.Modes {
		if len(m.ChildSlots) > 0 {
			return true
		}
	}
	return false
}

func hasDefaultChildSlot(slots []ChildSlotMapping) bool {
	for _, cs := range slots {
		if strings.EqualFold(cs.MDLContainer, defaultSlotContainer) {
			return true
		}
	}
	return false
}

// defRoutesChild mirrors the matching done by applyObjectLists, applyChildSlots
// and the auto-discovery passes; slots are the ones the widget is built with
// (selectedChildSlots). Matching is case-insensitive here even though the
// registry lower-cases MDLContainer on load, so that a hand-written .def.json
// cannot turn a casing slip into a false "this is dropped".
func defRoutesChild(def *WidgetDefinition, slots []ChildSlotMapping, c *ast.WidgetV3) bool {
	for _, ol := range def.ObjectLists {
		// By container keyword (`column c { … }`), or by name against the
		// property key (the auto-discovery pass matches a child's NAME).
		if strings.EqualFold(ol.MDLContainer, c.Type) || strings.EqualFold(ol.PropertyKey, c.Name) {
			return true
		}
	}
	isContainer := strings.EqualFold(c.Type, "container")
	for _, cs := range slots {
		if strings.EqualFold(cs.MDLContainer, c.Type) || strings.EqualFold(cs.PropertyKey, c.Name) {
			return true
		}
		// `container <slotName> { … }` — routed by the container's NAME, the
		// authoring form applyChildSlots supports for slots whose keyword reads
		// badly as a widget type.
		if isContainer && strings.EqualFold(cs.MDLContainer, c.Name) {
			return true
		}
	}
	return false
}

// otherModeSlot finds a mode, other than the selected one, that declares a
// slot under the keyword.
func otherModeSlot(def *WidgetDefinition, selected *WidgetMode, keyword string) *WidgetMode {
	for i := range def.Modes {
		m := &def.Modes[i]
		if m == selected {
			continue
		}
		for _, cs := range m.ChildSlots {
			if strings.EqualFold(cs.MDLContainer, keyword) {
				return m
			}
		}
	}
	return nil
}

// modeLabel names a mode inside a sentence: its description ("association
// mode") where it has one.
func modeLabel(m *WidgetMode) string {
	if m.Description != "" {
		return strings.ToLower(m.Description)
	}
	return m.Name + " mode"
}

// unroutedChildMessage explains one dropped child, naming what the parent does
// declare. The "spelled X on this widget" half is the answer in the case that
// prompted the rule: `filter` and `controlbar` are the SAME property under two
// widgets' conventions, so an author copying a gallery example onto a datagrid
// needs the other spelling, not a list to hunt through.
func unroutedChildMessage(def *WidgetDefinition, w, child *ast.WidgetV3) string {
	// A slot of another mode: the content would land in a property the widget
	// does not render in the mode it is built in. Say which mode that is and
	// which slot it does have (ako/mxcli#1744).
	if slots, mode := selectedChildSlots(def, w); mode != nil {
		if other := otherModeSlot(def, mode, child.Type); other != nil {
			msg := fmt.Sprintf("`%s` is the slot of %s's %s, and %s `%s` is built in %s",
				strings.ToLower(child.Type), parentLabel(def), modeLabel(other),
				strings.ToLower(def.MDLName), w.Name, modeLabel(mode))
			if len(slots) == 0 {
				return msg + ", which declares no slot — it would be dropped on write"
			}
			var kws []string
			for _, cs := range slots {
				kws = append(kws, "`"+strings.ToLower(cs.MDLContainer)+"`")
			}
			return msg + " — write it in " + strings.Join(kws, " or ") + " instead"
		}
	}
	msg := fmt.Sprintf("`%s` is not a container or slot of %s, so it would be dropped on write",
		strings.ToLower(child.Type), parentLabel(def))
	if alt := sameSlotUnderAnotherName(def, child.Type); alt != "" {
		msg += fmt.Sprintf(" — on this widget that slot is spelled `%s`", alt)
	}
	return msg + declaredContainers(def)
}

// sameSlotUnderAnotherName finds the keyword THIS widget uses for the property
// that some other widget spells `keyword`. Derived from
// widgetSlotKeywordOverrides, which is where the convention already lives, so
// the two cannot drift.
func sameSlotUnderAnotherName(def *WidgetDefinition, keyword string) string {
	var propertyKey string
	for widgetID, slots := range widgetSlotKeywordOverrides {
		if widgetID == def.WidgetID {
			continue
		}
		for key, kw := range slots {
			if strings.EqualFold(kw, keyword) {
				propertyKey = key
			}
		}
	}
	if propertyKey == "" {
		return ""
	}
	for _, cs := range def.ChildSlots {
		if strings.EqualFold(cs.PropertyKey, propertyKey) {
			return strings.ToLower(cs.MDLContainer)
		}
	}
	return ""
}

// validateUnroutedChildren is the check-time half (MDL-WIDGET30). It sits beside
// MDL-WIDGET26, which covers the neighbouring case: a container KEYWORD (`group`,
// `series` — words that are not widgets at all) under a parent that does not
// declare it. This one covers a real WIDGET in the same position, which
// MDL-WIDGET26 cannot report because a widget resolves perfectly well on its own.
func validateUnroutedChildren(w *ast.WidgetV3, def *WidgetDefinition, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, child := range unroutedPluggableChildren(def, w) {
		out = append(out, linter.Violation{
			RuleID:     "MDL-WIDGET30",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: %s", locationPrefix, unroutedChildMessage(def, w, child)),
			Suggestion: "move it into one of the parent's containers, or out of the widget's body",
		})
	}
	return out
}

// refuseUnroutedChildren is the exec-time half. `check` can be skipped
// (`exec --no-check`), and a write that silently discards part of what it was
// given is the failure this whole rule exists to stop — so the writer refuses
// rather than trusting that the checker ran.
func refuseUnroutedChildren(def *WidgetDefinition, w *ast.WidgetV3) error {
	dropped := unroutedPluggableChildren(def, w)
	if len(dropped) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("widget `%s`: %s", w.Name, unroutedChildMessage(def, w, dropped[0]))
}
