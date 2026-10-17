// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// legacyTable is a Forms$Table whose one cell holds the named widgets. The
// table keeps its widgets in Cells[].Widgets[] (gen's Forms$Table / TableCell;
// a Mendix 11 cell is stored as Forms$DbTableCell, which `mx convert` writes
// with a Widgets list — see widget_list_markers.go), so the cell's $Type is
// deliberately not one the walk could special-case.
func legacyTable(inner ...bson.D) bson.D {
	widgets := bson.A{int32(2)}
	for _, w := range inner {
		widgets = append(widgets, w)
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Table"},
		{Key: "Name", Value: "table1"},
		{Key: "Cells", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$DbTableCell"},
			{Key: "LeftColumnIndex", Value: int32(0)},
			{Key: "TopRowIndex", Value: int32(0)},
			{Key: "Widgets", Value: widgets},
		}}},
		{Key: "Columns", Value: bson.A{int32(2)}},
		{Key: "Rows", Value: bson.A{int32(2)}},
	}
}

func cellWidgetNames(t *testing.T, raw bson.D) []string {
	t.Helper()
	table := findWidgetForTest(t, raw, "table1")
	cell := bsonnav.DGetArrayElements(bsonnav.DGet(table, "Cells"))[0].(bson.D)
	var names []string
	for _, w := range bsonnav.DGetArrayElements(bsonnav.DGet(cell, "Widgets")) {
		names = append(names, bsonnav.DGetString(w.(bson.D), "Name"))
	}
	return names
}

func menuItem(name string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: name},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}, {Key: "Class", Value: ""}}},
	}
}

// mendixlabs/mxcli#1453: set / insert / replace / drop could not reach a
// widget inside a legacy table cell — findInWidgetChildren had no Cells walk.
func TestFindWidgetInLegacyTableCell_1453(t *testing.T) {
	raw := makeRawPage(legacyTable(menuItem("btnInCell")))
	if findBsonWidget(raw, "btnInCell") == nil {
		t.Fatal(`widget "btnInCell" inside a table cell not found`)
	}
	if _, ok := extractWidgetScopeFromBSON(raw)["btnInCell"]; !ok {
		t.Error("btnInCell missing from the widget scope")
	}
}

func TestSetInsertDropInLegacyTableCell_1453(t *testing.T) {
	m := New(makeRawPage(legacyTable(menuItem("btnInCell"))), model.ID("u"), &stubWidgetDeps{})
	if err := m.SetWidgetProperty("btnInCell", "Class", "probe"); err != nil {
		t.Fatalf("set Class on btnInCell: %v", err)
	}
	if err := m.InsertWidget("btnInCell", "", backend.InsertPosition("after"), []pages.Widget{newCellWidget("btnNew")}); err != nil {
		t.Fatalf("insert after btnInCell: %v", err)
	}
	if got := cellWidgetNames(t, m.rawData); len(got) != 2 || got[1] != "btnNew" {
		t.Fatalf("cell after insert = %v, want [btnInCell btnNew]", got)
	}
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "btnInCell"}}); err != nil {
		t.Fatalf("drop btnInCell: %v", err)
	}
	if got := cellWidgetNames(t, m.rawData); len(got) != 1 || got[0] != "btnNew" {
		t.Errorf("cell after drop = %v, want [btnNew]", got)
	}
}

func newCellWidget(name string) pages.Widget {
	w := &pages.ActionButton{}
	w.Name = name
	return w
}
