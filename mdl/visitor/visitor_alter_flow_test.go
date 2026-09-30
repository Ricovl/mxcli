// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// alter microflow / nanoflow (ADR-0012 decision 3, ako/mxcli#736): targets are
// content addresses kept as written, fragments are microflow statements.
func TestAlterFlow_OperationsAndTargets(t *testing.T) {
	prog, errs := Build(`alter microflow FeedbackModule.VAL_Feedback {
		insert after $IsValidEmail { log info node 'Feedback' 'Email checked'; }
		insert before 'Email is Valid?' @2 { declare $n Integer = 1; set $n = 2; }
		replace log  *  node 'Debug' * with { log warning node 'Debug' 'x'; }
		drop set $ValidFeedback = false @3;
	};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterFlowStmt)
	if !ok {
		t.Fatalf("want *ast.AlterFlowStmt, got %T", prog.Statements[0])
	}
	if stmt.Nanoflow || stmt.Name.String() != "FeedbackModule.VAL_Feedback" {
		t.Errorf("header: nanoflow=%v name=%s", stmt.Nanoflow, stmt.Name)
	}
	want := []struct {
		op     ast.AlterFlowOpKind
		target string
		body   int
	}{
		{ast.AlterFlowInsertAfter, "$IsValidEmail", 1},
		{ast.AlterFlowInsertBefore, "'Email is Valid?' @2", 2},
		{ast.AlterFlowReplace, "log  *  node 'Debug' *", 1},
		{ast.AlterFlowDrop, "set $ValidFeedback = false @3", 0},
	}
	if len(stmt.Operations) != len(want) {
		t.Fatalf("want %d operations, got %d", len(want), len(stmt.Operations))
	}
	for i, w := range want {
		got := stmt.Operations[i]
		if got.Op != w.op || got.Target != w.target || len(got.Body) != w.body {
			t.Errorf("op %d: got %q %q body=%d, want %q %q body=%d", i, got.Op, got.Target, len(got.Body), w.op, w.target, w.body)
		}
	}
}

func TestAlterFlow_Nanoflow(t *testing.T) {
	prog, errs := Build(`alter nanoflow M.NF { drop $X; }`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.AlterFlowStmt)
	if !stmt.Nanoflow || stmt.Operations[0].Op != ast.AlterFlowDrop || stmt.Operations[0].Target != "$X" {
		t.Errorf("got %+v %+v", stmt, stmt.Operations[0])
	}
}
