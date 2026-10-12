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
