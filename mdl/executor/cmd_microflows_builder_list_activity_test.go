// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `filter $L where Status = 'Open'` is Studio Pro's "Filter by expression"
// even though its expression reads `Member = value`; `filter $L by …` with the
// same condition is "Filter" by member (#733). The linking word decides, not
// the shape of the condition.
func TestFilterWhereIsAlwaysByExpression(t *testing.T) {
	build := func(byExpression bool) microflows.ListOperation {
		fb := predicateBuilder(t)
		stmt := &ast.ListOperationStmt{
			Operation:      ast.ListOpFilter,
			InputVariable:  "L",
			OutputVariable: "R",
			Condition:      bare("Status", "=", "'Open'"),
			ByExpression:   byExpression,
		}
		oc := fb.buildFlowGraph([]ast.MicroflowStatement{stmt},
			&ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}})
		if errs := fb.GetErrors(); len(errs) > 0 {
			t.Fatalf("build errors: %v", errs)
		}
		for _, obj := range oc.Objects {
			if act, ok := obj.(*microflows.ActionActivity); ok {
				if lo, ok := act.Action.(*microflows.ListOperationAction); ok {
					return lo.Operation
				}
			}
		}
		t.Fatal("no list operation built")
		return nil
	}

	op, ok := build(true).(*microflows.FilterOperation)
	if !ok {
		t.Fatalf("where: built %T, want *microflows.FilterOperation", build(true))
	}
	if !strings.Contains(op.Expression, "$currentObject/Status") {
		t.Errorf("where: expression %q does not read the member off $currentObject", op.Expression)
	}
	// Control: the same condition after `by` (or in the call form) is by member.
	if got, ok := build(false).(*microflows.FilterByAttributeOperation); !ok || got.Attribute != "Shop.Order.Status" {
		t.Errorf("by: built %#v, want Filter by attribute Shop.Order.Status", build(false))
	}
}
