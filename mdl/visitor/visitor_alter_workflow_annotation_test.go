// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1717: the notes CREATE writes — the header's `annotation '…'` and
// an activity's `@annotation '…'` — are set in ALTER as `Annotation`, on the
// workflow and with `on <activity>`; the empty string removes one.
func TestAlterWorkflow_Annotation(t *testing.T) {
	prog := mustBuild(t, `alter workflow M.W {
  set (Annotation: 'Started from the request form');
  set (Annotation: 'Logs it') on ACT_Log;
  set (Annotation: '') on 'Review'@2;
};`)
	ops := prog.Statements[0].(*ast.AlterWorkflowStmt).Operations
	if len(ops) != 3 {
		t.Fatalf("ops = %d, want 3", len(ops))
	}
	wf, ok := ops[0].(*ast.SetWorkflowPropertyOp)
	if !ok || wf.Property != "annotation" || wf.Value != "Started from the request form" {
		t.Errorf("workflow op = %#v", ops[0])
	}
	act, ok := ops[1].(*ast.SetActivityPropertyOp)
	if !ok || act.Property != "annotation" || act.Value != "Logs it" || act.ActivityRef != "ACT_Log" {
		t.Errorf("activity op = %#v", ops[1])
	}
	clear, ok := ops[2].(*ast.SetActivityPropertyOp)
	if !ok || clear.Property != "annotation" || clear.Value != "" || clear.ActivityRef != "Review" || clear.AtPosition != 2 {
		t.Errorf("removing op = %#v", ops[2])
	}
}

// A note is a plain string: Workflows$Annotation stores a Description and no
// template, so parameters, a page or an expression are refused with the form
// that works.
func TestAlterWorkflow_AnnotationTakesAString(t *testing.T) {
	for _, src := range []string{
		`alter workflow M.W { set (Annotation: 'For {1}' with ({1} = $WorkflowContext/Name)); };`,
		`alter workflow M.W { set (Annotation: M.Page) on Review; };`,
		`alter workflow M.W { set (Annotation: addDays([%CurrentDateTime%], 1)); };`,
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Errorf("%s: accepted", src)
			continue
		}
		if !strings.Contains(errs[0].Error(), "Annotation takes a string") {
			t.Errorf("%s: error %q", src, errs[0])
		}
	}
}
