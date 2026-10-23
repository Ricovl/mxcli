// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// A legacy table cell stores no name, so an empty one could not be filled at
// all (ako/mxcli#1326, mendixlabs/mxcli#1453). It is addressed by position,
// `table1 cell(row, column)`, 1-based. The fixture is the shape Mendix 11.13's
// `mx convert` writes for a table (Forms$DbTableCell cells keyed by
// TopRowIndex/LeftColumnIndex with Width/Height spans, ColumnWidths, Weight):
//
//	row 1: [ tInCell ] [ empty ]
//	row 2: [ one cell spanning both columns ]

func dbTableCell(row, col, width int32, widgets ...string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$DbTableCell"},
		{Key: "Height", Value: int32(1)},
		{Key: "IsHeader", Value: false},
		{Key: "LeftColumnIndex", Value: col},
		{Key: "TopRowIndex", Value: row},
		{Key: "Widgets", Value: widgetList(widgets...)},
		{Key: "Width", Value: width},
	}
}

func legacyTable2x2() bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$Table"},
		{Key: "Cells", Value: bson.A{int32(2),
			dbTableCell(0, 0, 1, "tInCell"),
			dbTableCell(0, 1, 1),
			dbTableCell(1, 0, 2, "tWide"),
		}},
		{Key: "ColumnWidths", Value: bson.A{int32(2),
			bson.D{{Key: "$Type", Value: "Forms$TableColumn"}, {Key: "Value", Value: int32(1)}},
			bson.D{{Key: "$Type", Value: "Forms$TableColumn"}, {Key: "Value", Value: int32(1)}},
		}},
		{Key: "Name", Value: "table1"},
		{Key: "Rows", Value: bson.A{int32(2), bson.D{{Key: "$Type", Value: "Forms$TableRow"}}, bson.D{{Key: "$Type", Value: "Forms$TableRow"}}}},
		{Key: "WidthUnit", Value: "Weight"},
	}
}

func tableMutator() *Mutator {
	return New(makeRawPage(legacyTable2x2()), model.ID("u"), &stubWidgetDeps{})
}

// cellNames returns the widget names in the cell at stored index i, checking
// the cell count and the Widgets marker are intact.
func cellNames(t *testing.T, m *Mutator, i int) []string {
	t.Helper()
	table := findWidgetForTest(t, m.rawData, "table1")
	cells := bsonnav.DGetArrayElements(bsonnav.DGet(table, "Cells"))
	if len(cells) != 3 {
		t.Fatalf("the table has %d cells, want 3 — a cell was added or removed", len(cells))
	}
	raw := bsonnav.ToBsonA(bsonnav.DGet(cells[i].(bson.D), "Widgets"))
	if len(raw) == 0 || raw[0] != int32(2) {
		t.Fatalf("cell %d lost its Widgets [2] marker: %v", i, raw)
	}
	var out []string
	for _, w := range raw[1:] {
		out = append(out, bsonnav.DGetString(w.(bson.D), "Name"))
	}
	return out
}

func TestInsertIntoEmptyTableCell(t *testing.T) {
	m := tableMutator()
	if err := m.InsertWidget("table1 cell(1,2)", "", into, itemButton("btnNew")); err != nil {
		t.Fatalf("insert into table1 cell(1,2): %v", err)
	}
	if got := strings.Join(cellNames(t, m, 1), ","); got != "btnNew" {
		t.Errorf("cell(1,2) = %s, want btnNew", got)
	}
}

func TestInsertIntoTableCellAppends(t *testing.T) {
	m := tableMutator()
	if err := m.InsertWidget("table1 cell(1,1)", "", into, itemButton("btnNew")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cellNames(t, m, 0), ","); got != "tInCell,btnNew" {
		t.Errorf("cell(1,1) = %s, want tInCell,btnNew", got)
	}
}

func TestReplaceAndDropTableCell(t *testing.T) {
	m := tableMutator()
	if err := m.ReplaceWidget("table1 cell(1,1)", "", itemButton("btnRep")); err != nil {
		t.Fatalf("replace table1 cell(1,1): %v", err)
	}
	if got := strings.Join(cellNames(t, m, 0), ","); got != "btnRep" {
		t.Errorf("after replace cell(1,1) = %s, want btnRep", got)
	}
	// DROP empties the cell; removing the cell itself would break the grid.
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "table1 cell(1,1)"}}); err != nil {
		t.Fatalf("drop table1 cell(1,1): %v", err)
	}
	if got := cellNames(t, m, 0); len(got) != 0 {
		t.Errorf("after drop cell(1,1) = %v, want empty", got)
	}
}

func TestSpannedTableCell(t *testing.T) {
	m := tableMutator()
	if err := m.InsertWidget("table1 cell(2,1)", "", into, itemButton("btnNew")); err != nil {
		t.Fatalf("the spanning cell's own top-left: %v", err)
	}
	err := m.InsertWidget("table1 cell(2,2)", "", into, itemButton("btnX"))
	if err == nil || !strings.Contains(err.Error(), "cell(2,1)") {
		t.Errorf("cell(2,2) lies under the spanning cell(2,1); want a refusal naming it, got %v", err)
	}
}

func TestTableCellMissListsCells(t *testing.T) {
	m := tableMutator()
	err := m.InsertWidget("table1 cell(5,1)", "", into, itemButton("b"))
	if err == nil || !strings.Contains(err.Error(), "cell(1,2)") || !strings.Contains(err.Error(), "cell(2,1)") {
		t.Errorf("want a miss listing the table's cells, got %v", err)
	}
}

func TestInsertBesideTableCellIsRefused(t *testing.T) {
	m := tableMutator()
	if err := m.InsertWidget("table1 cell(1,1)", "", backend.InsertPosition("after"), itemButton("b")); err == nil {
		t.Fatal("insert after a cell must be refused: a cell is a position in the table, not a widget")
	}
	if got := strings.Join(cellNames(t, m, 0), ","); got != "tInCell" {
		t.Errorf("a refused insert changed cell(1,1): %s", got)
	}
}

func TestResolveAlterTargetTableCell(t *testing.T) {
	m := tableMutator()
	got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"table1 cell(1,2)"}})
	if err != nil {
		t.Fatalf("ResolveAlterTarget: %v", err)
	}
	if got.Kind != "cell" {
		t.Errorf("kind = %q, want cell", got.Kind)
	}
}

func TestTableCellSelectorSpelling(t *testing.T) {
	sel, ok := backend.ParseTableCellSelector("table1 cell(2,1)")
	if !ok || sel != (backend.TableCellSelector{Table: "table1", Row: 2, Column: 1}) || sel.String() != "table1 cell(2,1)" {
		t.Errorf("parse/spell round trip: %+v %v %q", sel, ok, sel.String())
	}
	for _, bad := range []string{"table1", "table1 cell(0,1)", "table1 cell(1)", "a b cell(1,1)", "table1 item@1"} {
		if _, ok := backend.ParseTableCellSelector(bad); ok {
			t.Errorf("%q must not parse as a cell", bad)
		}
	}
}

// A cell is not a data container: widgets inserted into it bind in the data
// context its table sits in, or an attribute-bound widget is refused as
// "in no data container" (measured on a table inside a data view).
func TestTableCellSeesItsTablesDataContext(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dv"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: bson.D{{Key: "$Type", Value: "DomainModels$DirectEntityRef"}, {Key: "Entity", Value: "T.Item"}}},
		}},
		{Key: "Widgets", Value: bson.A{int32(2), legacyTable2x2()}},
	}
	m := New(makeRawPage(dv), model.ID("u"), &stubWidgetDeps{})
	if got := m.EnclosingEntity("table1 cell(1,2)"); got != "T.Item" {
		t.Errorf("EnclosingEntity(table1 cell(1,2)) = %q, want T.Item", got)
	}
}
