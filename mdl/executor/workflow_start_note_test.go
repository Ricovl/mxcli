// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#1717: a note a Studio Pro author puts on the workflow's start is
// stored on the start activity. describe prints it as the header's `annotation
// '…'` — it printed only the workflow's own Annotation, which Studio Pro shows
// nowhere, so a replay dropped the start's note — and CREATE writes the header
// clause back onto the start activity, not onto the workflow.
func TestWorkflowStartNote_SurvivesDescribeAndCreate(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "Approve")
	start := &workflows.StartWorkflowActivity{}
	start.Name, start.Caption, start.Annotation = "Start", "Start", "Started from the request form"
	end := &workflows.EndWorkflowActivity{}
	end.Name, end.Caption = "End", "End"
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{start, end}}

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	var written *workflows.Workflow
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc:  func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		CreateWorkflowFunc: func(w *workflows.Workflow) error { written = w; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeWorkflow(ctx, ast.QualifiedName{Module: "Sales", Name: "Approve"}))
	out := buf.String()
	if !strings.Contains(out, "annotation 'Started from the request form'") {
		t.Fatalf("describe does not print the start's note:\n%s", out)
	}

	stmt := findStmt[*ast.CreateWorkflowStmt](t, reparse(t, out), out)
	stmt.Name.Name = "ApproveCopy"
	stmt.CreateOrModify = false
	assertNoError(t, execCreateWorkflow(ctx, stmt))
	if written == nil || written.Flow == nil || len(written.Flow.Activities) == 0 {
		t.Fatalf("nothing written: %+v", written)
	}
	s, ok := written.Flow.Activities[0].(*workflows.StartWorkflowActivity)
	if !ok || s.Annotation != "Started from the request form" {
		t.Errorf("start activity = %+v, want the note on it", written.Flow.Activities[0])
	}
	if written.Annotation != "" {
		t.Errorf("the note was also written to the workflow's own Annotation: %q", written.Annotation)
	}
}
