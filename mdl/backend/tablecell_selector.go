// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"strconv"
	"strings"
)

// TableCellSelector is the address of a legacy table cell: the cell whose
// top-left position is Row, Column, both counted from 1 —
//
//	table1 cell(2,1)
//
// A cell stores no name (Forms$DbTableCell carries only TopRowIndex and
// LeftColumnIndex, 0-based), so it is addressed by position. Like
// NavItemSelector it travels through the PageMutator interface as the
// widgetRef string, spelled by String, which no widget name can collide with:
// a widget name contains no space.
type TableCellSelector struct {
	Table  string
	Row    int
	Column int
}

const tableCellSelectorInfix = " cell("

// String spells the selector as MDL writes it (ast.TableCellRef.Spell).
func (s TableCellSelector) String() string {
	return s.Table + tableCellSelectorInfix + strconv.Itoa(s.Row) + "," + strconv.Itoa(s.Column) + ")"
}

// ParseTableCellSelector reads a widgetRef spelled by String. ok is false for
// anything else, including every widget name and a position below 1.
func ParseTableCellSelector(ref string) (TableCellSelector, bool) {
	table, rest, found := strings.Cut(ref, tableCellSelectorInfix)
	if !found || table == "" || strings.ContainsAny(table, " ()'") {
		return TableCellSelector{}, false
	}
	pos, ok := strings.CutSuffix(rest, ")")
	if !ok {
		return TableCellSelector{}, false
	}
	r, c, ok := strings.Cut(pos, ",")
	if !ok {
		return TableCellSelector{}, false
	}
	row, err1 := strconv.Atoi(strings.TrimSpace(r))
	col, err2 := strconv.Atoi(strings.TrimSpace(c))
	if err1 != nil || err2 != nil || row < 1 || col < 1 {
		return TableCellSelector{}, false
	}
	return TableCellSelector{Table: table, Row: row, Column: col}, true
}
