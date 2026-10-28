// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A legacy table cell stores no name, so it is addressed by its position,
// `table1 cell(row, column)`, 1-based (PROPOSAL_alter_object_list_item_address.md).
// The address travels as the target's Widget spelling, like a navigation list
// item's, so every operation that takes a widget receives it unchanged.
func TestAlterTarget_TableCell(t *testing.T) {
	stmt := buildAlterPage(t, `alter page M.P {
		insert into table1 cell(1, 2) { dynamictext t1 (Content: 'x') };
		replace table1 cell(2,1) with { dynamictext t2 (Content: 'y') };
		drop table1 cell(3, 3);
	};`)
	check := func(label string, got ast.WidgetRef, want ast.TableCellRef, spelled string) {
		t.Helper()
		if got.Cell == nil || *got.Cell != want {
			t.Errorf("%s: Cell = %+v, want %+v", label, got.Cell, want)
		}
		if got.Widget != spelled || got.Column != "" || got.Ordinal != 0 {
			t.Errorf("%s: Widget %q Column %q Ordinal %d, want %q, \"\", 0", label, got.Widget, got.Column, got.Ordinal, spelled)
		}
	}
	check("insert", stmt.Operations[0].(*ast.InsertWidgetOp).Target, ast.TableCellRef{Table: "table1", Row: 1, Column: 2}, "table1 cell(1,2)")
	check("replace", stmt.Operations[1].(*ast.ReplaceWidgetOp).Target, ast.TableCellRef{Table: "table1", Row: 2, Column: 1}, "table1 cell(2,1)")
	check("drop", stmt.Operations[2].(*ast.DropWidgetOp).Targets[0], ast.TableCellRef{Table: "table1", Row: 3, Column: 3}, "table1 cell(3,3)")
}

// `cell` stays an ordinary name everywhere else.
func TestCellIsStillAnIdentifier(t *testing.T) {
	stmt := buildAlterPage(t, `alter page M.P { set (Caption: 'x') on cell; };`)
	if got := stmt.Operations[0].(*ast.SetPropertyOp).Target.Widget; got != "cell" {
		t.Errorf("a widget named cell: Widget = %q", got)
	}
}

// Positions count from 1, as describe prints them.
func TestAlterTarget_TableCellZeroIsRefused(t *testing.T) {
	_, errs := Build(`alter page M.P { drop table1 cell(0, 1); };`)
	if len(errs) == 0 {
		t.Error("cell(0, 1) must be refused: rows and columns count from 1")
	}
}
