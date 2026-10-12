// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1388: a navigation list item has no name, so it is addressed
// as `nav item(widget)`, `nav item('text')` or `nav item@n`. The item's @n is
// its own, never the target's Ordinal (which a page would refuse), and Widget
// carries the address so every widget operation passes it through.
func TestAlterTarget_NavItemAddress(t *testing.T) {
	stmt := buildAlterPage(t, `alter snippet M.S {
		set (Action: show page M.Home) on nav item('Home');
		drop nav item@2, nav item(t1), nav item('it''s')@3;
		insert after nav item(t1) { item (Action: show page M.Home) { dynamictext t2 (Content: 'x') } }
		replace nav item@1 with { item (Action: show page M.Home) { dynamictext t3 (Content: 'y') } }
	};`)
	check := func(label string, got ast.WidgetRef, want ast.NavItemRef, spelled string) {
		t.Helper()
		if got.Item == nil || *got.Item != want {
			t.Errorf("%s: Item = %+v, want %+v", label, got.Item, want)
		}
		if got.Widget != spelled || got.Ordinal != 0 || got.Column != "" {
			t.Errorf("%s: Widget %q Ordinal %d Column %q, want %q, 0, \"\"", label, got.Widget, got.Ordinal, got.Column, spelled)
		}
	}
	check("set", stmt.Operations[0].(*ast.SetPropertyOp).Target, ast.NavItemRef{List: "nav", Text: "Home"}, "nav item('Home')")
	drop := stmt.Operations[1].(*ast.DropWidgetOp).Targets
	if len(drop) != 3 {
		t.Fatalf("drop has %d targets, want 3", len(drop))
	}
	check("drop 1", drop[0], ast.NavItemRef{List: "nav", Ordinal: 2}, "nav item@2")
	check("drop 2", drop[1], ast.NavItemRef{List: "nav", Widget: "t1"}, "nav item(t1)")
	check("drop 3", drop[2], ast.NavItemRef{List: "nav", Text: "it's", Ordinal: 3}, "nav item('it''s')@3")
	check("insert", stmt.Operations[2].(*ast.InsertWidgetOp).Target, ast.NavItemRef{List: "nav", Widget: "t1"}, "nav item(t1)")
	check("replace", stmt.Operations[3].(*ast.ReplaceWidgetOp).Target, ast.NavItemRef{List: "nav", Ordinal: 1}, "nav item@1")
}
