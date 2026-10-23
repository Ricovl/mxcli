// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// A legacy table (Forms$Table) keeps its widgets in cells, and a cell stores
// no name — Mendix 11.13's `mx convert` writes each as a Forms$DbTableCell with
// TopRowIndex / LeftColumnIndex (0-based), Width / Height (its span) and a
// Widgets list. Widgets inside a cell are reachable by their own names
// (#1463); the cell itself, and so an EMPTY cell, is addressed by position:
// `table1 cell(row, column)`, 1-based (ako/mxcli#1326, mendixlabs/mxcli#1453).
//
// A cell is a position in the table's grid, not a widget: INSERT INTO appends
// to it, REPLACE swaps its content, DROP empties it — removing the cell element
// would leave a hole in the grid — and BEFORE/AFTER a cell are refused.

const legacyTableType = "Forms$Table"

// withTableCellAddresses extends a widget finder with table cell addresses.
// The result for a cell is the cell itself, in the table's Cells.
func withTableCellAddresses(inner widgetFinder) widgetFinder {
	return func(rawData bson.D, ref string) *bsonWidgetResult {
		if sel, ok := backend.ParseTableCellSelector(ref); ok {
			r, _ := resolveTableCell(rawData, inner, sel)
			return r
		}
		return inner(rawData, ref)
	}
}

type storedCell struct {
	index                int // position in the table's Cells, marker excluded
	row, col, rows, cols int // 0-based top-left and span
}

func tableCells(table bson.D) []storedCell {
	var out []storedCell
	for i, el := range bsonnav.DGetArrayElements(bsonnav.DGet(table, "Cells")) {
		d, ok := el.(bson.D)
		if !ok {
			continue
		}
		c := storedCell{index: i, row: intField(d, "TopRowIndex"), col: intField(d, "LeftColumnIndex"),
			rows: intField(d, "Height"), cols: intField(d, "Width")}
		if c.rows < 1 {
			c.rows = 1
		}
		if c.cols < 1 {
			c.cols = 1
		}
		out = append(out, c)
	}
	return out
}

func intField(d bson.D, key string) int {
	switch v := bsonnav.DGet(d, key).(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func (c storedCell) address(table string) string {
	return backend.TableCellSelector{Table: table, Row: c.row + 1, Column: c.col + 1}.String()
}

// resolveTableCell finds the cell whose top-left is the selector's position.
func resolveTableCell(rawData bson.D, find widgetFinder, sel backend.TableCellSelector) (*bsonWidgetResult, error) {
	table := find(rawData, sel.Table)
	if table == nil {
		return nil, fmt.Errorf("%s: table %q not found", sel, sel.Table)
	}
	if bsonnav.DGetString(table.widget, "$Type") != legacyTableType {
		return nil, fmt.Errorf("%s: %q is a %s, not a table — `cell(row, column)` addresses a legacy table's cells",
			sel, sel.Table, widgetTypeName(table.widget))
	}
	row, col := sel.Row-1, sel.Column-1
	cells := tableCells(table.widget)
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(table.widget, "Cells"))
	for _, c := range cells {
		if c.row == row && c.col == col {
			return &bsonWidgetResult{widget: elements[c.index].(bson.D), parentArr: elements, parentKey: "Cells",
				parentDoc: table.widget, index: c.index, cell: &sel}, nil
		}
	}
	for _, c := range cells {
		if row >= c.row && row < c.row+c.rows && col >= c.col && col < c.col+c.cols {
			return nil, fmt.Errorf("%s lies under %s, which spans %d row(s) and %d column(s) — address that cell",
				sel, c.address(sel.Table), c.rows, c.cols)
		}
	}
	var addrs []string
	for _, c := range cells {
		addrs = append(addrs, c.address(sel.Table))
	}
	return nil, fmt.Errorf("%s: the table has no cell there. Cells of %s: %s", sel, sel.Table, strings.Join(addrs, ", "))
}

// tableCellAddressError explains a cell address that did not resolve.
func (m *Mutator) tableCellAddressError(ref string) error {
	sel, ok := backend.ParseTableCellSelector(ref)
	if !ok {
		return nil
	}
	inner := m.widgetFinder
	if _, err := resolveTableCell(m.rawData, inner, sel); err != nil {
		return err
	}
	return nil
}

// setCellWidgets stores a cell's new content, keeping its list marker.
func setCellWidgets(cell bson.D, widgets []any) {
	out := bson.A{int32(2)}
	if raw := bsonnav.ToBsonA(bsonnav.DGet(cell, "Widgets")); len(raw) > 0 && isListMarker(raw[0]) {
		out = bson.A{raw[0]}
	}
	bsonnav.DSet(cell, "Widgets", append(out, widgets...))
}

// refuseBesideTableCell refuses INSERT BEFORE/AFTER a cell.
func refuseBesideTableCell(result *bsonWidgetResult, ref string, position backend.InsertPosition) error {
	if result.cell == nil || strings.EqualFold(string(position), "into") {
		return nil
	}
	return fmt.Errorf("cannot insert %s %q: a table cell is a position in the table, not a widget — "+
		"write `insert into %s { … }` to add to it, or name a widget inside it", strings.ToLower(string(position)), ref, ref)
}
