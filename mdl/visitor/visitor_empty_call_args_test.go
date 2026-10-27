// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"
)

// ako/mxcli#1516 (mendixlabs/mxcli#1606): a page action or data source that
// calls a flow with no parameters may be written with an empty argument list,
// `call microflow M.Ping()`, as a flow body writes it. It was a syntax error
// ("mismatched input ')' expecting the start of a statement") everywhere the
// shared argument rule is used, while the bracket-less form and a non-empty
// list both parsed.
//
// The empty list must mean exactly what no list means: the same statement, the
// same deprecations. An empty-but-present mapping list in the AST would be a
// change in what exec writes, which is why this compares whole statements.
//
// The `args` form is the control: it must build a DIFFERENT statement from the
// bare one, or the equality below could not detect a stray mapping.
func TestEmptyCallArgumentListMeansNoArguments(t *testing.T) {
	const page = "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($O: M.E)) { dataview dv (DataSource: $O) { %s } };"
	pg := func(widget string) string { return strings.Replace(page, "%s", widget, 1) }
	cases := []struct {
		site string
		// src holds %s where the argument list goes.
		src string
		// args is a non-empty list for the control; "" where the site has none
		// of its own (the fragment and building-block forms carry an action).
		args string
	}{
		{"button call microflow", pg("actionbutton b (Caption: 'Go', Action: call microflow M.F%s)"), "(Order = $currentObject)"},
		{"button call nanoflow", pg("actionbutton b (Caption: 'Go', Action: call nanoflow M.N%s)"), "(Order = $currentObject)"},
		{"button show page", pg("actionbutton b (Caption: 'Go', Action: show page M.Q%s)"), "(Order = $currentObject)"},
		{"button microflow (MDL-DEPR020)", pg("actionbutton b (Caption: 'Go', Action: microflow M.F%s)"), "(Order = $currentObject)"},
		{"button nanoflow (MDL-DEPR020)", pg("actionbutton b (Caption: 'Go', Action: nanoflow M.N%s)"), "(Order = $currentObject)"},
		{"button show_page (MDL-DEPR020)", pg("actionbutton b (Caption: 'Go', Action: show_page M.Q%s)"), "(Order = $currentObject)"},
		{"call microflow then settings", pg("actionbutton b (Caption: 'Go', Action: call microflow M.F%s with (ProgressBar: Blocking))"), "(Order = $currentObject)"},
		{"call microflow then returns", pg("actionbutton b (Caption: 'Go', Action: call microflow M.F%s returns ($result = $ActionReturnValue))"), "(Order = $currentObject)"},
		{"create object then show page", pg("actionbutton b (Caption: 'Go', Action: create object M.E then show page M.Q%s)"), "(Order = $currentObject)"},
		{"container OnClick", pg("container c (OnClick: call microflow M.F%s) { }"), "(Order = $currentObject)"},
		{"textbox OnChange", pg("textbox t (Attribute: Name, OnChange: call nanoflow M.N%s)"), "(Order = $currentObject)"},
		{"pluggable action property", pg("gallery g (DataSource: database M.E, onSelectionChange: call microflow M.F%s) { }"), "(Order = $currentObject)"},
		{"data source microflow", pg("listview lv (DataSource: microflow M.DS%s) { }"), "(Order = $O)"},
		{"data source nanoflow", pg("listview lv (DataSource: nanoflow M.DS%s) { }"), "(Order = $O)"},
		{"data source microflow then settings", pg("listview lv (DataSource: microflow M.DS%s with (FormValidations: None)) { }"), "(Order = $O)"},
		{"alter page set Action", "alter page M.P { set Action = call microflow M.F%s on b };", "(Order = $currentObject)"},
		{"alter page set DataSource", "alter page M.P { set DataSource = microflow M.DS%s on lv };", "(Order = $O)"},
		{"navigation menu item", "create or replace navigation Responsive home page M.Home { menu item 'Go' (OnClick: call microflow M.F%s) };", "(Order = $currentObject)"},
		{"navigation menu item show page", "create or replace navigation Responsive home page M.Home { menu item 'Go' (OnClick: show page M.Q%s) };", "(Order = $currentObject)"},
		{"menu document item", "create or modify menu M.Menu { menu item 'Go' (OnClick: call nanoflow M.N%s) };", "(Order = $currentObject)"},
		{"fragment argument action", pg("use fragment Frag($act: call microflow M.F%s)"), "(Order = $currentObject)"},
		{"building block action override", pg("use building block M.BB(Action: call microflow M.F%s)"), "(Order = $currentObject)"},
		// A fragment use has an argument list of its own.
		{"use fragment", pg("use fragment Frag%s"), "($p: $O)"},
	}
	for _, c := range cases {
		t.Run(c.site, func(t *testing.T) {
			bareSrc := strings.Replace(c.src, "%s", "", 1)
			emptySrc := strings.Replace(c.src, "%s", "()", 1)
			bare := mustBuild(t, bareSrc)
			empty := mustBuild(t, emptySrc)
			if !reflect.DeepEqual(empty.Statements, bare.Statements) {
				t.Errorf("X() builds a different statement from X:\n X():  %#v\n X:    %#v", empty.Statements, bare.Statements)
			}
			if got, want := deprecationCodes(empty), deprecationCodes(bare); !reflect.DeepEqual(got, want) {
				t.Errorf("X() recorded %v, X recorded %v", got, want)
			}
			// Control: a real argument list must change the statement, or the
			// comparison above proves nothing.
			args := mustBuild(t, strings.Replace(c.src, "%s", c.args, 1))
			if reflect.DeepEqual(args.Statements, bare.Statements) {
				t.Errorf("control: X%s builds the same statement as X, so the comparison cannot see an argument", c.args)
			}
		})
	}
}

// The flow body already took `()`; it must keep requiring the brackets (they
// are not optional there) and keep accepting the empty list.
func TestFlowBodyCallStillRequiresItsBrackets(t *testing.T) {
	mustBuild(t, "create microflow M.G () begin call microflow M.F(); end;")
	if _, errs := Build("create microflow M.G () begin call microflow M.F; end;"); len(errs) == 0 {
		t.Error("a flow-body call without () parsed; it was required before and must stay required")
	}
}
