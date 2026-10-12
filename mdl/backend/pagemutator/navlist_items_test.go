// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// navStubDeps serializes a navigation list the way the codec does: a
// Forms$NavigationList whose Items are Forms$NavigationListItem with no Name.
type navStubDeps struct{ stubWidgetDeps }

func (d *navStubDeps) SerializeWidget(w pages.Widget) bson.D {
	nl, ok := w.(*pages.NavigationList)
	if !ok {
		return d.stubWidgetDeps.SerializeWidget(w)
	}
	arr := bson.A{int32(2)}
	for _, it := range nl.Items {
		widgets := bson.A{int32(2)}
		for _, c := range it.Widgets {
			widgets = append(widgets, d.stubWidgetDeps.SerializeWidget(c))
		}
		arr = append(arr, bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(it.ID))},
			{Key: "$Type", Value: "Forms$NavigationListItem"},
			{Key: "Widgets", Value: widgets},
		})
	}
	return bson.D{{Key: "$Type", Value: "Forms$NavigationList"}, {Key: "Items", Value: arr}}
}

// navItemDoc is a stored item the way Studio Pro writes it: no Name.
func navItemDoc(textName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$NavigationListItem"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DynamicText"},
			{Key: "Name", Value: textName},
		}}},
	}
}

func makeNavListPage() bson.D {
	return makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$NavigationList"},
		{Key: "Name", Value: "navList1"},
		{Key: "Items", Value: bson.A{int32(2), navItemDoc("t1")}},
	})
}

func newNavItem(textName string) *pages.NavigationListItem {
	return &pages.NavigationListItem{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$NavigationListItem"},
		Widgets:     []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: textName}}},
	}
}

// navItemTexts lists, per stored item, the name of its first widget.
func navItemTexts(t *testing.T, raw bson.D) []string {
	t.Helper()
	nl := findBsonWidget(raw, "navList1")
	if nl == nil {
		t.Fatal("navList1 no longer resolves")
	}
	items := bsonnav.ToBsonA(bsonnav.DGet(nl.widget, "Items"))
	if len(items) == 0 || !isListMarker(items[0]) {
		t.Fatalf("Items lost its list marker: %v", items)
	}
	var out []string
	for _, el := range items[1:] {
		ws := bsonnav.DGetArrayElements(bsonnav.DGet(el.(bson.D), "Widgets"))
		out = append(out, bsonnav.DGetString(ws[0].(bson.D), "Name"))
	}
	return out
}

// mendixlabs/mxcli#1388: a widget inside a navigation list item could not be
// addressed — `insert after t1 { … }` failed with `widget "t1" not found` —
// because the lookup never descended into the list's Items.
func TestNavigationListItemWidgets_AreAddressable(t *testing.T) {
	raw := makeNavListPage()
	m := New(raw, model.ID("page-1"), &navStubDeps{})
	if findBsonWidget(raw, "t1") == nil {
		t.Fatal(`findBsonWidget("t1") = nil; a widget inside a navigation list item must resolve`)
	}
	if _, ok := m.WidgetScope()["t1"]; !ok {
		t.Error("WidgetScope lacks t1: a duplicate name inside an item would not be caught")
	}
	if _, found := findNearestDataSourceDoc(raw, "t1"); !found {
		t.Error("findNearestDataSourceDoc does not find t1")
	}
	// A sibling inserted next to t1 lands in the item's own Widgets.
	if err := m.InsertWidget("t1", "", toPos("after"), []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "t1b"}}}); err != nil {
		t.Fatalf("insert after t1: %v", err)
	}
	ws := bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "t1").parentDoc, "Widgets"))
	if len(ws) != 3 || bsonnav.DGetString(ws[2].(bson.D), "Name") != "t1b" {
		t.Errorf("item widgets = %v, want [marker t1 t1b]", ws)
	}
}

// mendixlabs/mxcli#1388: `insert into navList1 { item … }` appends the item to
// the list's Items and leaves the existing item untouched.
func TestInsertNavigationListItems_IntoAppends(t *testing.T) {
	m := New(makeNavListPage(), model.ID("page-1"), &navStubDeps{})
	firstBefore := bsonnav.DGet(bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "navList1").widget, "Items"))[1].(bson.D), "$ID")
	if err := m.InsertNavigationListItems("navList1", toPos("into"), []*pages.NavigationListItem{newNavItem("t2")}); err != nil {
		t.Fatalf("insert into navList1 { item }: %v", err)
	}
	if got := strings.Join(navItemTexts(t, m.rawData), ","); got != "t1,t2" {
		t.Errorf("items = %s, want t1,t2", got)
	}
	firstAfter := bsonnav.DGet(bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "navList1").widget, "Items"))[1].(bson.D), "$ID")
	if !equalIDs(firstBefore, firstAfter) {
		t.Error("the existing item's $ID changed")
	}
}

// An empty navigation list gains an Items list with Studio Pro's marker.
func TestInsertNavigationListItems_IntoEmptyList(t *testing.T) {
	raw := makeRawPage(bson.D{{Key: "$Type", Value: "Forms$NavigationList"}, {Key: "Name", Value: "navList1"}})
	m := New(raw, model.ID("page-1"), &navStubDeps{})
	if err := m.InsertNavigationListItems("navList1", toPos("into"), []*pages.NavigationListItem{newNavItem("t2")}); err != nil {
		t.Fatalf("insert into empty navList1: %v", err)
	}
	if got := strings.Join(navItemTexts(t, m.rawData), ","); got != "t2" {
		t.Errorf("items = %s, want t2", got)
	}
}

// Items have no Name, so BEFORE/AFTER has nothing to name; a widget inside an
// item is not an item's sibling. Refuse rather than put an item into a widget
// list, and say which form works.
func TestInsertNavigationListItems_RefusesWrongTargets(t *testing.T) {
	for _, tc := range []struct{ target, pos, want string }{
		{"t1", "after", "insert into navList1"},
		{"t1", "into", "only be added to a navigation list"},
	} {
		m := New(makeNavListPage(), model.ID("page-1"), &navStubDeps{})
		err := m.InsertNavigationListItems(tc.target, toPos(tc.pos), []*pages.NavigationListItem{newNavItem("t2")})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("insert %s %s { item }: err = %v, want one naming %q", tc.pos, tc.target, err, tc.want)
		}
		if got := strings.Join(navItemTexts(t, m.rawData), ","); got != "t1" {
			t.Errorf("a refused insert changed the items: %s", got)
		}
	}
}
