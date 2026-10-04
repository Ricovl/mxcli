// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#980: a menu item's OnClick is the widget action expression, so
// every action kind a menu item stores has a spelling, settings included.
// It used to take show page, call microflow and sign out only.
func TestNavMenuItem_OnClickTakesEveryMenuActionKind(t *testing.T) {
	cases := []struct {
		onClick  string
		kind     string
		settings bool
	}{
		{"show page M.P", "showPage", false},
		{"call microflow M.F with (ProgressBar: Blocking, Asynchronous: true)", "microflow", true},
		{"call nanoflow M.N with (Confirmation: 'Sure?', ProceedCaption: 'Yes', CancelCaption: 'No')", "nanoflow", true},
		{"open link 'https://docs.mendix.com'", "openLink", false},
		{"create object M.E then show page M.E_New", "create", false},
		{"sign out", "signOut", false},
		{"nothing", "none", false},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			prog, errs := Build("create or modify navigation Responsive {\n  menu item 'X' ( OnClick: " + c.onClick + " )\n};")
			if len(errs) > 0 {
				t.Fatalf("%q: %v", c.onClick, errs[0])
			}
			item := prog.Statements[0].(*ast.AlterNavigationStmt).MenuItems[0]
			if item.Action == nil || item.Action.Type != c.kind {
				t.Fatalf("%q: action %+v, want kind %s", c.onClick, item.Action, c.kind)
			}
			if (item.Action.Settings != nil) != c.settings {
				t.Errorf("%q: settings %+v", c.onClick, item.Action.Settings)
			}
		})
	}

	// The simple targets are still recorded for the checks that read them.
	prog, errs := Build("create or modify navigation Responsive {\n  menu item 'A' ( OnClick: show page M.P )\n  menu item 'B' page M.Q;\n};")
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	items := prog.Statements[0].(*ast.AlterNavigationStmt).MenuItems
	if items[0].Page == nil || items[0].Page.String() != "M.P" || items[1].Page == nil || items[1].Action == nil {
		t.Errorf("simple targets: %+v / %+v", items[0], items[1])
	}
}

// The actions a menu item cannot carry are refused, not stored as nothing.
func TestNavMenuItem_RefusesAnActionAMenuItemCannotCarry(t *testing.T) {
	for _, onClick := range []string{"save changes", "delete", "close page", "complete task 'Approve'", "open link $currentObject/Url"} {
		_, errs := Build("create or modify navigation Responsive {\n  menu item 'X' ( OnClick: " + onClick + " )\n};")
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), "menu item 'X'") {
			t.Errorf("%q: want a refusal naming the item, got %v", onClick, errs)
		}
	}
	// CONTROL: the same statement with an action a menu item has is accepted.
	if _, errs := Build("create or modify navigation Responsive {\n  menu item 'X' ( OnClick: call nanoflow M.N )\n};"); len(errs) > 0 {
		t.Errorf("control refused: %v", errs[0])
	}
}
