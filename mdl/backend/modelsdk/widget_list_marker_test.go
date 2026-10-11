// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func arrayMarker(t *testing.T, doc bsonv1.D, key string) (int32, int) {
	t.Helper()
	for _, e := range doc {
		if e.Key != key {
			continue
		}
		arr, ok := e.Value.(bsonv1.A)
		if !ok || len(arr) == 0 {
			t.Fatalf("%s is %T %v, want a typed array", key, e.Value, e.Value)
		}
		m, _ := arr[0].(int32)
		return m, len(arr) - 1
	}
	t.Fatalf("%s missing from %v", key, docKeys(doc))
	return 0, 0
}

func subDocs(t *testing.T, doc bsonv1.D, key string) []bsonv1.D {
	t.Helper()
	for _, e := range doc {
		if e.Key == key {
			var out []bsonv1.D
			for _, v := range e.Value.(bsonv1.A)[1:] {
				out = append(out, v.(bsonv1.D))
			}
			return out
		}
	}
	t.Fatalf("%s missing", key)
	return nil
}

// Studio Pro writes the typed-array marker 2 on every `Widgets` list, whatever
// its first child is and whether or not it is empty: 22,000+ Widgets lists
// across ako/TestApp and PedApp, every owner type, not one exception — 8 of
// them a LayoutGridColumn whose first child is a Forms$TabControl. The encoder
// took the marker from the first child's $Type, and Forms$TabControl had no
// registration, so such a column was written with 3 and a describe → exec of
// an unchanged page rewrote it (ako/mxcli-ledger FINDINGS #170).
//
// The tab control's own TabPages list is the control: Studio Pro stores 3
// there (32 of 32), and it must stay 3.
func TestWidgetsListMarker_TabControlFirstChildIsTwo(t *testing.T) {
	lg := &pages.LayoutGrid{BaseWidget: pages.BaseWidget{Name: "lg"}}
	lg.Rows = []*pages.LayoutGridRow{{Columns: []*pages.LayoutGridColumn{{
		Widgets: []pages.Widget{&pages.TabContainer{
			BaseWidget: pages.BaseWidget{Name: "tabs"},
			TabPages: []*pages.TabPage{
				{Name: "tp1", Widgets: []pages.Widget{&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}}}},
				{Name: "tp2"},
			},
		}},
	}}}}
	el, err := widgetToGen(lg)
	if err != nil {
		t.Fatal(err)
	}
	grid := encodeElement(t, el)
	row := subDocs(t, grid, "Rows")[0]
	col := subDocs(t, row, "Columns")[0]
	if m, n := arrayMarker(t, col, "Widgets"); m != 2 || n != 1 {
		t.Errorf("LayoutGridColumn.Widgets (first child Forms$TabControl) marker = %d (len %d), want 2", m, n)
	}
	tabs := subDocs(t, col, "Widgets")[0]
	if m, _ := arrayMarker(t, tabs, "TabPages"); m != 3 {
		t.Errorf("TabControl.TabPages marker = %d, want 3 as Studio Pro stores it", m)
	}
	tps := subDocs(t, tabs, "TabPages")
	if m, _ := arrayMarker(t, tps[0], "Widgets"); m != 2 {
		t.Errorf("TabPage.Widgets marker = %d, want 2", m)
	}
}

// An empty Widgets list is marker 2 as well (TabPage 20 of 20, LayoutGridColumn
// 13 of 13, DivContainer 8 of 8 empty lists in the same sweep), where the
// encoder's empty-list default is 3. Emptying a stored list rebuilds it whole,
// which is where the empty marker is written.
func TestWidgetsListMarker_EmptiedListIsTwo(t *testing.T) {
	box := decodeStoredBox(t)
	box.RemoveWidgets(1)
	box.RemoveWidgets(0)
	doc := encodeElement(t, box)
	if m, n := arrayMarker(t, doc, "Widgets"); m != 2 || n != 0 {
		t.Errorf("emptied Widgets = marker %d len %d, want [2]", m, n)
	}
}

func decodeStoredBox(t *testing.T) *genPg.DivContainer {
	t.Helper()
	stored := bsonv1.D{
		{Key: "$ID", Value: idBinaryForTest(t)},
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "box"},
		{Key: "Widgets", Value: bsonv1.A{int32(2),
			bsonv1.D{{Key: "$ID", Value: idBinaryForTest(t)}, {Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "inner1"}, {Key: "Widgets", Value: bsonv1.A{int32(2)}}},
			bsonv1.D{{Key: "$ID", Value: idBinaryForTest(t)}, {Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "inner2"}, {Key: "Widgets", Value: bsonv1.A{int32(2)}}},
		}},
	}
	raw, err := bsonv1.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return el.(*genPg.DivContainer)
}

// A stored container whose child changed is re-encoded selectively: clean
// children pass through, and the encoder writes the list's marker itself. That
// path used 3 for any list without a property registration, so editing one
// widget inside a stored Widgets list rewrote the list's marker from 2 to 3.
func TestWidgetsListMarker_SelectiveRebuildKeepsTwo(t *testing.T) {
	box := decodeStoredBox(t)
	box.WidgetsItems()[1].(*genPg.DivContainer).SetName("renamed")
	doc := encodeElement(t, box)
	if m, n := arrayMarker(t, doc, "Widgets"); m != 2 || n != 2 {
		t.Errorf("selectively rebuilt Widgets = marker %d len %d, want marker 2 with both children", m, n)
	}
}

func idBinaryForTest(t *testing.T) any {
	t.Helper()
	return mpr.IDToBsonBinary(mpr.GenerateID())
}

// findDoc returns the first sub-document of $Type typeName, depth-first.
func findDoc(doc bsonv1.D, typeName string) bsonv1.D {
	for _, e := range doc {
		if e.Key == "$Type" && e.Value == typeName {
			return doc
		}
	}
	for _, e := range doc {
		switch v := e.Value.(type) {
		case bsonv1.D:
			if d := findDoc(v, typeName); d != nil {
				return d
			}
		case bsonv1.A:
			for _, x := range v {
				if d, ok := x.(bsonv1.D); ok {
					if found := findDoc(d, typeName); found != nil {
						return found
					}
				}
			}
		}
	}
	return nil
}

// Studio Pro never omits a `Widgets` list: every owner in ako/TestApp and
// PedApp carries the key, empty or not (TabPage 89 of 89, 20 of them empty;
// LayoutGridColumn 732 of 732, 13 empty; DivContainer 838 of 838, 8 empty;
// CustomWidgets$WidgetValue 20,775 of 20,775). A freshly built container with
// no children wrote no Widgets key at all, so an empty tab page differed from
// the same page saved by Studio Pro.
func TestWidgetsListMarker_EmptyNewOwnerWritesTwo(t *testing.T) {
	cases := []struct {
		owner  string
		widget pages.Widget
	}{
		{"Forms$TabPage", &pages.TabContainer{BaseWidget: pages.BaseWidget{Name: "tabs"}, TabPages: []*pages.TabPage{{Name: "tp"}}}},
		{"Forms$DivContainer", &pages.Container{BaseWidget: pages.BaseWidget{Name: "c"}}},
		{"Forms$GroupBox", &pages.GroupBox{BaseWidget: pages.BaseWidget{Name: "g"}}},
		{"Forms$LayoutGridColumn", &pages.LayoutGrid{BaseWidget: pages.BaseWidget{Name: "lg"}, Rows: []*pages.LayoutGridRow{{Columns: []*pages.LayoutGridColumn{{}}}}}},
		{"Forms$ScrollContainerRegion", &pages.ScrollContainer{BaseWidget: pages.BaseWidget{Name: "sc"}, Regions: []*pages.ScrollContainerRegion{{Slot: pages.ScrollSlotTop}}}},
		{"Forms$NavigationListItem", &pages.NavigationList{BaseWidget: pages.BaseWidget{Name: "nl"}, Items: []*pages.NavigationListItem{{}}}},
		{"Forms$DataView", &pages.DataView{BaseWidget: pages.BaseWidget{Name: "dv"}}},
	}
	for _, c := range cases {
		t.Run(c.owner, func(t *testing.T) {
			el, err := widgetToGen(c.widget)
			if err != nil {
				t.Fatal(err)
			}
			owner := findDoc(encodeElement(t, el), c.owner)
			if owner == nil {
				t.Fatalf("no %s in the encoded widget", c.owner)
			}
			if m, n := arrayMarker(t, owner, "Widgets"); m != 2 || n != 0 {
				t.Errorf("empty %s.Widgets = marker %d len %d, want [2]", c.owner, m, n)
			}
		})
	}
}

// The control: the mandatory list is keyed on the owner, so a type Studio Pro
// stores without a Widgets key does not gain one. Neither Forms$TabControl nor
// Forms$ScrollContainer carries Widgets in any TestApp or PedApp document.
func TestWidgetsListMarker_NoWidgetsKeyOnNonOwners(t *testing.T) {
	for _, w := range []pages.Widget{
		&pages.TabContainer{BaseWidget: pages.BaseWidget{Name: "tabs"}, TabPages: []*pages.TabPage{{Name: "tp"}}},
		&pages.ScrollContainer{BaseWidget: pages.BaseWidget{Name: "sc"}, Regions: []*pages.ScrollContainerRegion{{Slot: pages.ScrollSlotTop}}},
	} {
		el, err := widgetToGen(w)
		if err != nil {
			t.Fatal(err)
		}
		doc := encodeElement(t, el)
		if hasKey(doc, "Widgets") {
			t.Errorf("%T wrote a Widgets key Studio Pro never stores on it", w)
		}
	}
}

// Forms$TemplateGridContents holds a Widgets list too, but neither TestApp nor
// PedApp has a template grid, so #1104 left it on the child-type path. Measured
// with mx convert (Mendix 11.15.0), which re-serializes every unit: a template
// grid injected with its contents' Widgets written as [3] — empty, and with a
// Forms$TabControl first child — came back [2] both times, and a bare grid was
// given an empty Widgets [2] it did not have. Before the registration the
// encoder wrote 3 for the tab-control case and the emptied list, and no key at
// all for a new empty one.
func TestWidgetsListMarker_TemplateGridContents(t *testing.T) {
	const owner = "Forms$TemplateGridContents"

	empty := genPg.NewTemplateGridContents()
	if m, n := arrayMarker(t, encodeElement(t, empty), "Widgets"); m != 2 || n != 0 {
		t.Errorf("new empty %s.Widgets = marker %d len %d, want [2]", owner, m, n)
	}

	withTabs := genPg.NewTemplateGridContents()
	tabs := genPg.NewTabContainer()
	tabs.SetName("tabs")
	withTabs.AddWidgets(tabs)
	if m, n := arrayMarker(t, encodeElement(t, withTabs), "Widgets"); m != 2 || n != 1 {
		t.Errorf("%s.Widgets (first child Forms$TabControl) = marker %d len %d, want 2", owner, m, n)
	}

	stored := bsonv1.D{
		{Key: "$ID", Value: idBinaryForTest(t)},
		{Key: "$Type", Value: owner},
		{Key: "Widgets", Value: bsonv1.A{int32(2),
			bsonv1.D{{Key: "$ID", Value: idBinaryForTest(t)}, {Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "a"}, {Key: "Widgets", Value: bsonv1.A{int32(2)}}},
			bsonv1.D{{Key: "$ID", Value: idBinaryForTest(t)}, {Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "b"}, {Key: "Widgets", Value: bsonv1.A{int32(2)}}},
		}},
	}
	raw, err := bsonv1.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	decode := func() *genPg.TemplateGridContents {
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return el.(*genPg.TemplateGridContents)
	}
	edited := decode()
	edited.WidgetsItems()[1].(*genPg.DivContainer).SetName("renamed")
	if m, n := arrayMarker(t, encodeElement(t, edited), "Widgets"); m != 2 || n != 2 {
		t.Errorf("selectively rebuilt %s.Widgets = marker %d len %d, want 2", owner, m, n)
	}
	emptied := decode()
	emptied.RemoveWidgets(1)
	emptied.RemoveWidgets(0)
	if m, n := arrayMarker(t, encodeElement(t, emptied), "Widgets"); m != 2 || n != 0 {
		t.Errorf("emptied %s.Widgets = marker %d len %d, want [2]", owner, m, n)
	}
}
