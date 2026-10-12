// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func navItemNode(textName, content string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "item",
		Children: []*ast.WidgetV3{{
			Type: "dynamictext", Name: textName, Properties: map[string]any{"Content": content},
		}},
	}
}

// mendixlabs/mxcli#1388: `insert into navList1 { item … }` failed with
// "failed to build widgets: failed to build widget : item must be a direct
// child of navigationlist" — the INSERT built each node as a widget, and a lone
// item is refused by the builder. It must build the item and hand it to
// InsertNavigationListItems with the target and position the statement named.
func TestAlterPageInsertNavigationListItem_RoutesToItems(t *testing.T) {
	var gotTarget string
	var gotPos backend.InsertPosition
	var gotItems []*pages.NavigationListItem
	mutator := &mock.MockPageMutator{
		InsertNavigationListItemsFunc: func(target string, pos backend.InsertPosition, items []*pages.NavigationListItem) error {
			gotTarget, gotPos, gotItems = target, pos, items
			return nil
		},
		InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
			t.Error("a navigation list item went through InsertWidget — it would land in a widget list")
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "navList1"},
		Widgets:  []*ast.WidgetV3{navItemNode("t2", "Second")},
	})
	if err != nil {
		t.Fatalf("insert into navList1 { item … }: %v", err)
	}
	if gotTarget != "navList1" || !strings.EqualFold(string(gotPos), "INTO") {
		t.Errorf("InsertNavigationListItems(%q, %q), want (navList1, INTO)", gotTarget, gotPos)
	}
	if len(gotItems) != 1 {
		t.Fatalf("got %d items, want 1", len(gotItems))
	}
	it := gotItems[0]
	if it.TypeName != "Forms$NavigationListItem" {
		t.Errorf("item type = %s, want Forms$NavigationListItem", it.TypeName)
	}
	if len(it.Widgets) != 1 || it.Widgets[0].GetName() != "t2" {
		t.Errorf("item widgets = %d, want the dynamic text t2", len(it.Widgets))
	}
}

// An item and an ordinary widget go to different lists; one INSERT cannot mean
// both, so it is refused rather than half-applied.
func TestAlterPageInsertNavigationListItem_MixedRefused(t *testing.T) {
	mutator := &mock.MockPageMutator{
		InsertNavigationListItemsFunc: func(string, backend.InsertPosition, []*pages.NavigationListItem) error {
			t.Error("InsertNavigationListItems called for a mixed insert")
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "navList1"},
		Widgets: []*ast.WidgetV3{
			navItemNode("t2", "Second"),
			{Type: "dynamictext", Name: "plain", Properties: map[string]any{"Content": "x"}},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "mixing `item`")
}

// ast.NavItemRef.Spell writes the address the visitor stores in Widget, and
// backend.ParseNavItemSelector is what the mutator reads it with. They live in
// packages that cannot import each other, so this pins them together.
func TestNavItemRef_SpellingIsTheBackendSelector(t *testing.T) {
	for _, r := range []ast.NavItemRef{
		{List: "nav", Widget: "t1"},
		{List: "nav", Text: "it's"},
		{List: "nav", Ordinal: 2},
		{List: "nav", Text: "Home", Ordinal: 3},
	} {
		sel, ok := backend.ParseNavItemSelector(r.Spell())
		if !ok || sel != (backend.NavItemSelector{List: r.List, Widget: r.Widget, Text: r.Text, Ordinal: r.Ordinal}) || sel.String() != r.Spell() {
			t.Errorf("%q: parsed %+v, %v; spelled back %q", r.Spell(), sel, ok, sel.String())
		}
	}
}

func navItemTarget(r ast.NavItemRef) ast.WidgetRef {
	return ast.WidgetRef{Widget: r.Spell(), Item: &r}
}

// `replace nav item@2 with { item … }` replaces that item with the new ones —
// not a widget REPLACE, which would write an item into a widget list.
func TestAlterPageReplaceNavigationListItem_RoutesToItems(t *testing.T) {
	var gotTarget string
	var gotItems []*pages.NavigationListItem
	mutator := &mock.MockPageMutator{
		ReplaceNavigationListItemFunc: func(target string, items []*pages.NavigationListItem) error {
			gotTarget, gotItems = target, items
			return nil
		},
		ReplaceWidgetFunc: func(string, string, []pages.Widget) error {
			t.Error("an item replacement went through ReplaceWidget")
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.ReplaceWidgetOp{
		Target:     navItemTarget(ast.NavItemRef{List: "navList1", Ordinal: 2}),
		NewWidgets: []*ast.WidgetV3{navItemNode("t9", "New")},
	})
	if err != nil {
		t.Fatalf("replace navList1 item@2 with { item … }: %v", err)
	}
	if gotTarget != "navList1 item@2" || len(gotItems) != 1 || gotItems[0].Widgets[0].GetName() != "t9" {
		t.Errorf("ReplaceNavigationListItem(%q, %d items), want (navList1 item@2, the t9 item)", gotTarget, len(gotItems))
	}
}

// `insert after nav item(t1) { item … }` places the item next to that item.
func TestAlterPageInsertNavigationListItem_AfterAnItem(t *testing.T) {
	var gotTarget string
	var gotPos backend.InsertPosition
	mutator := &mock.MockPageMutator{
		InsertNavigationListItemsFunc: func(target string, pos backend.InsertPosition, _ []*pages.NavigationListItem) error {
			gotTarget, gotPos = target, pos
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "AFTER",
		Target:   navItemTarget(ast.NavItemRef{List: "navList1", Widget: "t1"}),
		Widgets:  []*ast.WidgetV3{navItemNode("t2", "Second")},
	})
	if err != nil {
		t.Fatalf("insert after navList1 item(t1) { item … }: %v", err)
	}
	if gotTarget != "navList1 item(t1)" || !strings.EqualFold(string(gotPos), "AFTER") {
		t.Errorf("InsertNavigationListItems(%q, %q), want (navList1 item(t1), AFTER)", gotTarget, gotPos)
	}
}

// `check` refuses an @n on a widget target with no project; the item's @n is
// part of the item address and must not trip that rule.
func TestValidateAlterPageAddresses_AcceptsNavItemAddress(t *testing.T) {
	stmt := &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "M", Name: "S"},
		Operations: []ast.AlterPageOperation{
			&ast.DropWidgetOp{Targets: []ast.WidgetRef{navItemTarget(ast.NavItemRef{List: "nav", Text: "Home", Ordinal: 2})}},
		},
	}
	if v := validateAlterPageAddresses(stmt); len(v) != 0 {
		t.Errorf("violations = %+v, want none", v)
	}
}
