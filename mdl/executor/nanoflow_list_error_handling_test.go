// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#591. The list activities — create list, add to / remove from
// list, list operations, aggregates and cast — carried no error-handling type
// at all, and both writers stamped a literal "Rollback" on them. In a NANOFLOW
// that is CE6035 "Error handling type is not supported" on every one of them,
// measured on 11.14.0 (ako/TestApp copy, one nanoflow per activity): Create list,
// Change list (add and remove), Aggregate list and List operation (head, filter,
// sort) all failed. Studio Pro stores "Abort" on every action of every nanoflow
// in ako/TestApp (130 actions over 11 nanoflows), and "Rollback" on the same
// actions in a microflow — so the value is the flow flavour's default, which is
// what the builder now supplies.
func TestListActions_ErrorHandlingFollowsTheFlowFlavour(t *testing.T) {
	src := `mdl 1;
create microflow M.F ($Products: List of M.Product, $P: M.Product)
begin
  $L = create list of M.Product;
  add $P to $L;
  remove $P from $L;
  $H = head $Products;
  $N = count $Products;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	body := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt).Body

	for _, tc := range []struct {
		nanoflow bool
		want     microflows.ErrorHandlingType
	}{
		{true, microflows.ErrorHandlingTypeAbort},
		{false, microflows.ErrorHandlingTypeRollback},
	} {
		fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, isNanoflow: tc.nanoflow}
		oc := fb.buildFlowGraph(body, nil)
		seen := 0
		for _, obj := range oc.Objects {
			aa, ok := obj.(*microflows.ActionActivity)
			if !ok {
				continue
			}
			var got microflows.ErrorHandlingType
			switch a := aa.Action.(type) {
			case *microflows.CreateListAction:
				got = a.ErrorHandlingType
			case *microflows.ChangeListAction:
				got = a.ErrorHandlingType
			case *microflows.ListOperationAction:
				got = a.ErrorHandlingType
			case *microflows.AggregateListAction:
				got = a.ErrorHandlingType
			default:
				continue
			}
			seen++
			if got != tc.want {
				t.Errorf("nanoflow=%v: %T has error handling %q, want %q", tc.nanoflow, aa.Action, got, tc.want)
			}
		}
		if seen != 5 {
			t.Fatalf("nanoflow=%v: found %d list activities, want 5", tc.nanoflow, seen)
		}
	}
}

// The cast activity is built by a different path from the list ones (an
// inheritance split's case body), so it is pinned on its own.
func TestCastAction_ErrorHandlingFollowsTheFlowFlavour(t *testing.T) {
	for _, tc := range []struct {
		nanoflow bool
		want     microflows.ErrorHandlingType
	}{
		{true, microflows.ErrorHandlingTypeAbort},
		{false, microflows.ErrorHandlingTypeRollback},
	} {
		fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, isNanoflow: tc.nanoflow}
		oc := fb.buildFlowGraph([]ast.MicroflowStatement{&ast.CastObjectStmt{ObjectVariable: "Obj", OutputVariable: "Sub"}}, nil)
		found := false
		for _, obj := range oc.Objects {
			if aa, ok := obj.(*microflows.ActionActivity); ok {
				if c, ok := aa.Action.(*microflows.CastAction); ok {
					found = true
					if c.ErrorHandlingType != tc.want {
						t.Errorf("nanoflow=%v: cast has error handling %q, want %q", tc.nanoflow, c.ErrorHandlingType, tc.want)
					}
				}
			}
		}
		if !found {
			t.Fatal("no cast action built")
		}
	}
}
