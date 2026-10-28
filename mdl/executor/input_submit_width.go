// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A text box's and text area's submit behaviour, and a layout grid's width
// (ako/mxcli-ledger FINDINGS #170).
//
// The writer hard-coded SubmitBehaviour "OnEndEditing", SubmitOnInputDelay 300
// and Forms$LayoutGrid.Width "FullWidth"; the model had no field for them,
// describe printed nothing and the builder read nothing. So describe → exec
// over a Studio Pro page turned a "submit while typing" field into an ordinary
// one and a fixed-width grid into a full-width one, mx check clean. And
// `SubmitBehaviour: WhileEditing` passed check (MDL-WIDGET07 only warns) and
// was dropped, while `Width: FixedWidth` on a grid did not even warn, "Width"
// being in the cross-widget known-property union.
//
// The MDL spelling is the stored property name and the metamodel's enum member:
//
//	textbox  t (Attribute: Name, SubmitBehaviour: WhileEditing, SubmitOnInputDelay: 500)
//	layoutgrid g (Width: FixedWidth) { … }
//
// The submit keys are a text box's and a text area's only. They are kept out
// of staticWidgetKnownProps (the union every widget type shares) so a date
// picker carrying one still draws MDL-WIDGET07 — the per-type exemption is
// isInputSubmitProp, the same arrangement as the #968 formatting keys. For the
// same reason this file, not a cmd_pages_builder* or describe-output file,
// holds the reads and the emits.

// inputSubmitProps are the submit properties, in canonical spelling and
// describe order.
var inputSubmitProps = []string{"SubmitBehaviour", "SubmitOnInputDelay"}

// isInputSubmitProp reports whether key is a submit property of the MDL widget
// type widgetType.
func isInputSubmitProp(widgetType, key string) bool {
	switch strings.ToLower(widgetType) {
	case "textbox", "textarea":
	default:
		return false
	}
	for _, p := range inputSubmitProps {
		if strings.EqualFold(p, key) {
			return true
		}
	}
	return false
}

// inputSubmitProblems validates a text box's or text area's submit properties.
// Each problem is a sentence fragment naming the property; nil means valid.
// The builder refuses with the same messages, so check and exec agree.
func inputSubmitProblems(w *ast.WidgetV3) []string {
	if !isInputSubmitProp(w.Type, "SubmitBehaviour") {
		return nil
	}
	var out []string
	if v, ok := lookupPropCI(w, "SubmitBehaviour"); ok {
		s, isStr := v.(string)
		if _, valid := pages.CanonicalSubmitBehaviour(s); !isStr || !valid {
			out = append(out, fmt.Sprintf("SubmitBehaviour must be OnEndEditing or WhileEditing, got %s", submitValueText(v)))
		}
	}
	if v, ok := lookupPropCI(w, "SubmitOnInputDelay"); ok {
		if n, isInt := toFormatInt(v); !isInt || n < 0 {
			out = append(out, fmt.Sprintf("SubmitOnInputDelay must be a non-negative number of milliseconds, got %s", submitValueText(v)))
		}
	}
	return out
}

// inputSubmitSettings reads a text box's or text area's submit properties.
// Unset returns "" / nil, which the writer turns into Studio Pro's defaults.
func inputSubmitSettings(w *ast.WidgetV3) (string, *int, error) {
	if problems := inputSubmitProblems(w); len(problems) > 0 {
		return "", nil, fmt.Errorf("%s %s: %s", strings.ToLower(w.Type), w.Name, strings.Join(problems, "; "))
	}
	behaviour := ""
	if v, ok := lookupPropCI(w, "SubmitBehaviour"); ok {
		s, _ := v.(string)
		behaviour, _ = pages.CanonicalSubmitBehaviour(s)
	}
	var delay *int
	if v, ok := lookupPropCI(w, "SubmitOnInputDelay"); ok {
		n, _ := toFormatInt(v)
		delay = &n
	}
	return behaviour, delay, nil
}

// submitValueText renders a refused value for a message: a plain value as
// written, anything else (a parsed expression such as `-1`) as what it is.
func submitValueText(v any) string {
	switch v.(type) {
	case string, bool, int, int32, int64, float64:
		return fmt.Sprintf("`%v`", v)
	}
	return "an expression"
}

// layoutGridWidthProblem validates a layout grid's Width; "" means valid.
func layoutGridWidthProblem(w *ast.WidgetV3) string {
	if !strings.EqualFold(w.Type, "layoutgrid") {
		return ""
	}
	v, ok := lookupPropCI(w, "Width")
	if !ok {
		return ""
	}
	s, isStr := v.(string)
	if _, valid := pages.CanonicalLayoutGridWidth(s); !isStr || !valid {
		return fmt.Sprintf("a layout grid's Width must be FullWidth or FixedWidth, got %s", submitValueText(v))
	}
	return ""
}

// layoutGridWidth reads a layout grid's Width; "" when unset.
func layoutGridWidth(w *ast.WidgetV3) (string, error) {
	if p := layoutGridWidthProblem(w); p != "" {
		return "", fmt.Errorf("layoutgrid %s: %s", w.Name, p)
	}
	v, _ := lookupPropCI(w, "Width")
	s, _ := v.(string)
	width, _ := pages.CanonicalLayoutGridWidth(s)
	return width, nil
}

// validateInputSubmitAndGridWidth (MDL-WIDGET18) reports at check time what
// the builder refuses at exec time, from the same functions.
func validateInputSubmitAndGridWidth(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	var out []linter.Violation
	for _, msg := range inputSubmitProblems(w) {
		out = append(out, violation18(locationPrefix, w, msg))
	}
	if msg := keyboardTypeProblem(w); msg != "" { // ako/mxcli#1209
		out = append(out, violation18(locationPrefix, w, msg))
	}
	if msg := layoutGridWidthProblem(w); msg != "" {
		out = append(out, violation18(locationPrefix, w, msg))
	}
	return out
}

// extractSubmitBehaviour reads a stored text box's or text area's submit
// settings: "" / nil when the key is absent.
func extractSubmitBehaviour(w map[string]any) (string, *int) {
	behaviour := extractString(w["SubmitBehaviour"])
	var delay *int
	if v, ok := w["SubmitOnInputDelay"]; ok && v != nil {
		n := extractInt(v)
		delay = &n
	}
	return behaviour, delay
}

// describeSubmitBehaviour appends a text input's submit settings when they
// differ from Studio Pro's defaults, so an ordinary input describes as before.
// The delay is printed on its own when it alone differs: Studio Pro keeps it
// when the behaviour is switched back to OnEndEditing, and dropping it would
// make the round trip rewrite it.
func describeSubmitBehaviour(props []string, w rawWidget) []string {
	if w.SubmitBehaviour != "" && w.SubmitBehaviour != pages.DefaultSubmitBehaviour {
		props = append(props, "SubmitBehaviour: "+w.SubmitBehaviour)
	}
	if w.SubmitOnInputDelay != nil && *w.SubmitOnInputDelay != pages.DefaultSubmitOnInputDelay {
		props = append(props, fmt.Sprintf("SubmitOnInputDelay: %d", *w.SubmitOnInputDelay))
	}
	return props
}

// describeLayoutGridWidth appends a layout grid's Width when it is not the
// default FullWidth.
func describeLayoutGridWidth(props []string, w rawWidget) []string {
	if w.LayoutGridWidth != "" && w.LayoutGridWidth != pages.DefaultLayoutGridWidth {
		props = append(props, "Width: "+w.LayoutGridWidth)
	}
	return props
}
