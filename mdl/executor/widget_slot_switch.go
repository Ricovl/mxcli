// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A child slot shown by a switch (ako/mxcli#1744).
//
// A combo box's custom content is shown while its …CustomContentType is 'yes'
// or 'listItem', and Mendix then requires it: the type written on its own —
// which MDL could do long before it could write the content — passed check and
// exec and failed mx check with CE0642 "Property 'Custom content' is
// required.". While the type is 'no' the editor hides the content, so widgets
// written into it would be stored and never shown. The definition says which
// enumeration switches which slot (ChildSlotMapping.Switch); check and exec
// read it through slotSwitchProblem, so they refuse the same statements.

// slotSwitchProblem returns why a switched slot of the widget cannot be written
// as the statement stands, "" when it can. Only the slots the widget is built
// with are judged: another mode's switch is not this rule's.
func slotSwitchProblem(def *WidgetDefinition, w *ast.WidgetV3) string {
	if def == nil || w == nil {
		return ""
	}
	slots, _ := selectedChildSlots(def, w)
	for _, slot := range slots {
		sw := slot.Switch
		if sw == nil {
			continue
		}
		kw := strings.ToLower(slot.MDLContainer)
		value, set := slotSwitchValue(w, sw)
		content := slotContentCount(slot, w)
		switch {
		case set && !strings.EqualFold(value, sw.Off) && content == 0:
			caption := sw.Caption
			if caption == "" {
				caption = slot.PropertyKey
			}
			return fmt.Sprintf("%s `%s` sets %s to '%s' but has no `%s { … }` block — Mendix requires the content "+
				"while %s is not '%s' (CE0642 \"Property '%s' is required.\"). Write the widgets in a `%s { … }` "+
				"block in the %s's body, or leave %s out",
				strings.ToLower(def.MDLName), w.Name, sw.PropertyKey, value, kw,
				sw.PropertyKey, sw.Off, caption, kw, strings.ToLower(def.MDLName), sw.PropertyKey)
		case set && strings.EqualFold(value, sw.Off) && content > 0:
			return fmt.Sprintf("%s `%s` writes a `%s { … }` block while %s is '%s', which hides it — the widgets "+
				"would be stored and never shown. Set %s to '%s', or leave it out: content sets it to '%s'",
				strings.ToLower(def.MDLName), w.Name, kw, sw.PropertyKey, sw.Off,
				sw.PropertyKey, sw.On, sw.On)
		}
	}
	return ""
}

// slotSwitchValue is the switch's value as the statement writes it.
func slotSwitchValue(w *ast.WidgetV3, sw *SlotSwitch) (string, bool) {
	raw, ok := lookupProperty(w.Properties, sw.PropertyKey)
	if !ok {
		return "", false
	}
	return stringifyAny(raw), true
}

// slotContentCount counts the widgets the statement writes into the slot,
// matched as applyChildSlots matches them: by the block's keyword, or
// `container <keyword> { … }`.
func slotContentCount(slot ChildSlotMapping, w *ast.WidgetV3) int {
	n := 0
	for _, c := range w.Children {
		if c == nil {
			continue
		}
		if strings.EqualFold(c.Type, slot.MDLContainer) ||
			(strings.EqualFold(c.Type, "container") && strings.EqualFold(c.Name, slot.MDLContainer)) {
			n += len(c.Children)
		}
	}
	return n
}

// switchedSlotsToTurnOn are the switches the build sets to On: a slot with
// content whose switch the statement leaves out. The content is what the author
// asked to show, and On is the value that shows it as written.
func switchedSlotsToTurnOn(def *WidgetDefinition, w *ast.WidgetV3) []*SlotSwitch {
	if def == nil || w == nil {
		return nil
	}
	slots, _ := selectedChildSlots(def, w)
	var out []*SlotSwitch
	for _, slot := range slots {
		if slot.Switch == nil || slotContentCount(slot, w) == 0 {
			continue
		}
		if _, set := slotSwitchValue(w, slot.Switch); !set {
			out = append(out, slot.Switch)
		}
	}
	return out
}

// validateSlotSwitches is check's half: MDL-WIDGET53, a switch that contradicts
// its slot's content — on with none (CE0642), off with some (CE0463).
func validateSlotSwitches(w *ast.WidgetV3, def *WidgetDefinition, locationPrefix string) []linter.Violation {
	msg := slotSwitchProblem(def, w)
	if msg == "" {
		return nil
	}
	return []linter.Violation{{
		RuleID:     "MDL-WIDGET53",
		Severity:   linter.SeverityError,
		Message:    locationPrefix + ": " + msg,
		Suggestion: "Write the content in its block — the switch is then set for you — or leave both out.",
	}}
}
