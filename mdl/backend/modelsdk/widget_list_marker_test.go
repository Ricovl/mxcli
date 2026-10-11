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
