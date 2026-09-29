// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// A client action's `with ( … )` settings (ako/mxcli#721 L2).
//
// Studio Pro stores, on every client action, whether the widget is disabled
// while the action runs, and on a microflow or nanoflow call a progress bar, a
// progress message and a confirmation. None of it had an MDL spelling, so the
// writer hard-coded the defaults and `describe` → `exec` removed a button's
// "Are you sure?" confirmation and its progress bar, and changed whether a
// widget is disabled while its action runs — with `mx check` and the build
// green.

// actionSettingKeys maps each setting key, lower-cased, to its canonical
// spelling and the action types that carry it.
var actionSettingKeys = map[string]struct {
	canonical string
	flowOnly  bool // call microflow / call nanoflow only
	mfOnly    bool // call microflow only
}{
	"disabledduringexecution": {canonical: "DisabledDuringExecution"},
	"progressbar":             {canonical: "ProgressBar", flowOnly: true},
	"progressmessage":         {canonical: "ProgressMessage", flowOnly: true},
	"confirmation":            {canonical: "Confirmation", flowOnly: true},
	"proceedcaption":          {canonical: "ProceedCaption", flowOnly: true},
	"cancelcaption":           {canonical: "CancelCaption", flowOnly: true},
	"asynchronous":            {canonical: "Asynchronous", flowOnly: true, mfOnly: true},
	"formvalidations":         {canonical: "FormValidations", flowOnly: true, mfOnly: true},
}

// canonicalProgressBar and canonicalFormValidations spell an enum value the way
// Mendix stores it, whatever case it was written in.
var canonicalProgressBar = map[string]string{
	"none": "None", "nonblocking": "NonBlocking", "blocking": "Blocking",
}

var canonicalFormValidations = map[string]string{
	"all": "All", "widget": "Widget", "none": "None",
}

// actionSettingValue returns a setting's value text and whether it was a
// string literal.
func actionSettingValue(s *parser.ActionSettingV3Context) (string, bool) {
	if str := s.STRING_LITERAL(); str != nil {
		return unquoteStringLit(str), true
	}
	if n := s.NONE(); n != nil {
		return n.GetText(), false
	}
	// The key is the first identifierOrKeyword, the value the second.
	if all := s.AllIdentifierOrKeyword(); len(all) > 1 {
		return identifierOrKeywordText(all[1]), false
	}
	return "", false
}

// buildActionSettingsV3 builds the settings leniently: a key or value it does
// not understand is skipped here and reported by ExitActionSettingsV3, so a
// caller without a Builder still gets the settings it can use.
func buildActionSettingsV3(ctx parser.IActionSettingsV3Context) *ast.ActionSettingsV3 {
	sc, ok := ctx.(*parser.ActionSettingsV3Context)
	if !ok || sc == nil {
		return nil
	}
	out := &ast.ActionSettingsV3{}
	for _, raw := range sc.AllActionSettingV3() {
		s := raw.(*parser.ActionSettingV3Context)
		key := strings.ToLower(identifierOrKeywordText(s.IdentifierOrKeyword(0)))
		val, isString := actionSettingValue(s)
		switch key {
		case "disabledduringexecution":
			if b, ok := parseSettingBool(val, isString); ok {
				out.DisabledDuringExecution = &b
			}
		case "asynchronous":
			if b, ok := parseSettingBool(val, isString); ok {
				out.Asynchronous = &b
			}
		case "progressbar":
			if !isString {
				out.ProgressBar = canonicalProgressBar[strings.ToLower(val)]
			}
		case "formvalidations":
			if !isString {
				out.FormValidations = canonicalFormValidations[strings.ToLower(val)]
			}
		case "progressmessage":
			if isString {
				out.ProgressMessage = &val
			}
		case "confirmation":
			if isString {
				out.Confirmation = &val
			}
		case "proceedcaption":
			if isString {
				out.ProceedCaption = &val
			}
		case "cancelcaption":
			if isString {
				out.CancelCaption = &val
			}
		}
	}
	return out
}

func parseSettingBool(val string, isString bool) (bool, bool) {
	if isString {
		return false, false
	}
	switch strings.ToLower(val) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// ExitActionSettingsV3 reports what buildActionSettingsV3 skipped: an unknown
// key, a repeated key, a value of the wrong kind, and a setting the action does
// not have (R11: a meaningless form is an error, not a no-op). The syntax is
// new, so the rejection applies under every language version.
func (b *Builder) ExitActionSettingsV3(ctx *parser.ActionSettingsV3Context) {
	kind := "other"
	if act, ok := ctx.GetParent().(*parser.ActionExprV3Context); ok {
		switch {
		case act.MICROFLOW() != nil:
			kind = "microflow"
		case act.NANOFLOW() != nil:
			kind = "nanoflow"
		}
	}
	seen := map[string]bool{}
	for _, raw := range ctx.AllActionSettingV3() {
		s := raw.(*parser.ActionSettingV3Context)
		tok := s.GetStart()
		pos := fmt.Sprintf("line %d:%d", tok.GetLine(), tok.GetColumn())
		written := identifierOrKeywordText(s.IdentifierOrKeyword(0))
		key := strings.ToLower(written)
		spec, known := actionSettingKeys[key]
		if !known {
			b.addError(fmt.Errorf("%s: unknown action setting %q; every action takes "+
				"DisabledDuringExecution, a flow call also ProgressBar, ProgressMessage, "+
				"Confirmation, ProceedCaption and CancelCaption, and a microflow call also "+
				"Asynchronous and FormValidations", pos, written))
			continue
		}
		if seen[key] {
			b.addError(fmt.Errorf("%s: action setting %s is written twice", pos, spec.canonical))
			continue
		}
		seen[key] = true
		if spec.mfOnly && kind != "microflow" {
			b.addError(fmt.Errorf("%s: %s is a setting of `call microflow` only", pos, spec.canonical))
			continue
		}
		if spec.flowOnly && kind == "other" {
			b.addError(fmt.Errorf("%s: %s is a setting of `call microflow` and `call nanoflow` only",
				pos, spec.canonical))
			continue
		}
		val, isString := actionSettingValue(s)
		valText := s.GetStop().GetText()
		switch key {
		case "disabledduringexecution", "asynchronous":
			if _, ok := parseSettingBool(val, isString); !ok {
				b.addError(fmt.Errorf("%s: %s takes true or false, not %s", pos, spec.canonical, valText))
			}
		case "progressbar":
			if _, ok := canonicalProgressBar[strings.ToLower(val)]; isString || !ok {
				b.addError(fmt.Errorf("%s: ProgressBar takes None, NonBlocking or Blocking, not %s", pos, valText))
			}
		case "formvalidations":
			if _, ok := canonicalFormValidations[strings.ToLower(val)]; isString || !ok {
				b.addError(fmt.Errorf("%s: FormValidations takes All, Widget or None, not %s", pos, valText))
			}
		default: // the text settings
			if !isString {
				b.addError(fmt.Errorf("%s: %s takes a string, not %s", pos, spec.canonical, valText))
			}
		}
	}
	if !seen["confirmation"] && (seen["proceedcaption"] || seen["cancelcaption"]) {
		tok := ctx.GetStart()
		b.addError(fmt.Errorf("line %d:%d: ProceedCaption and CancelCaption are the buttons of a "+
			"confirmation; write Confirmation: '<question>' as well", tok.GetLine(), tok.GetColumn()))
	}
}
