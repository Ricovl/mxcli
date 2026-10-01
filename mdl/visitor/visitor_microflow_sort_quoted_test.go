// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestQuotedReservedSortAttribute covers the #619 grammar-widen slice for list
// SORT: an attribute named after a reserved keyword (e.g. "Status") sorted via
// sort($list, attr). The sortSpec position previously accepted only a bare
// IDENTIFIER; it now accepts QUOTED_IDENTIFIER and the visitor unquotes it.
func TestQuotedReservedSortAttribute(t *testing.T) {
	input := `create microflow M.MF_Sort (
  $ProductList: list of M.Product
)
returns list of M.Product as $Sorted
begin
  $Sorted = sort($ProductList, "Status" asc, Name desc);
  return $Sorted;
end;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	var sortOp *ast.ListOperationStmt
	for _, s := range mf.Body {
		if op, ok := s.(*ast.ListOperationStmt); ok && len(op.SortSpecs) > 0 {
			sortOp = op
			break
		}
	}
	if sortOp == nil {
		t.Fatal("no list SORT operation with sort specs found in microflow body")
	}

	if len(sortOp.SortSpecs) != 2 {
		t.Fatalf("expected 2 sort specs, got %d: %+v", len(sortOp.SortSpecs), sortOp.SortSpecs)
	}
	// Reserved-word attribute must unquote back to "Status".
	if sortOp.SortSpecs[0].Attribute != "Status" || !sortOp.SortSpecs[0].Ascending {
		t.Errorf(`first sort spec: expected {"Status" asc}, got %+v`, sortOp.SortSpecs[0])
	}
	// Non-reserved attribute is unaffected.
	if sortOp.SortSpecs[1].Attribute != "Name" || sortOp.SortSpecs[1].Ascending {
		t.Errorf(`second sort spec: expected {"Name" desc}, got %+v`, sortOp.SortSpecs[1])
	}
}

// A keyword attribute needs no quotes in the call form either, as in the
// statement form's listSortItem: `sort($L, Position, Status desc)` sorts by
// the attributes Position and Status (ako/mxcli#889).
func TestUnquotedKeywordSortAttribute(t *testing.T) {
	prog, errs := Build(`create microflow M.MF_Sort ($L: list of M.Lap) returns list of M.Lap as $Sorted
begin
  $Sorted = sort($L, Position, Status desc);
  return $Sorted;
end;`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	op, ok := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ListOperationStmt)
	if !ok {
		t.Fatalf("expected a list operation, got %T", prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0])
	}
	want := []ast.SortSpec{{Attribute: "Position", Ascending: true}, {Attribute: "Status", Ascending: false}}
	if len(op.SortSpecs) != 2 || op.SortSpecs[0] != want[0] || op.SortSpecs[1] != want[1] {
		t.Errorf("sort specs: got %+v, want %+v", op.SortSpecs, want)
	}
}
