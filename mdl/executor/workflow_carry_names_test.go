// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// describe prints no name for a workflow's implicit activities — the main
// flow's start and end, the end of an event sub-process, an `end workflow` in a
// branch — so `create or modify` rebuilt them as Start, End, End2, ... Studio
// Pro names them start1, end1, end2 (ako/TestApp workflow.Workflow1, #743), and
// every rewrite of its own describe output renamed them. The rewrite carries the
// stored name of each activity the statement cannot name.

func storedNamedWorkflow(mod *model.Module) *workflows.Workflow {
	act := func(a workflows.WorkflowActivity) workflows.WorkflowActivity { return a }
	base := func(name, caption string) workflows.BaseWorkflowActivity {
		return workflows.BaseWorkflowActivity{Name: name, Caption: caption}
	}
	return &workflows.Workflow{
		BaseElement: model.BaseElement{ID: nextID("wf")},
		ContainerID: mod.ID,
		Name:        "Approve",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			act(&workflows.StartWorkflowActivity{BaseWorkflowActivity: base("start1", "Start")}),
			act(&workflows.UserTask{
				BaseWorkflowActivity: base("userTask1", "Review"),
				Outcomes: []*workflows.UserTaskOutcome{
					{Value: "Reject", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
						act(&workflows.EndWorkflowActivity{BaseWorkflowActivity: base("end3", "End")}),
					}}},
					{Value: "Approve", Flow: &workflows.Flow{}},
				},
			}),
			act(&workflows.EndWorkflowActivity{BaseWorkflowActivity: base("end1", "End")}),
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{
			Name: "eventSubProcess1",
			Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				act(&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: base("espStart", "Wait"), Interrupting: true}),
				act(&workflows.EndWorkflowActivity{BaseWorkflowActivity: base("end2", "End")}),
			}},
		}},
	}
}

const namedWorkflowRewrite = `create or modify workflow MyModule.Approve
begin
  user task userTask1 'Review'
    page MyModule.P
    outcomes
      'Reject' { end workflow; }
      'Approve' { };
  event subprocess eventSubProcess1 on interrupting notification espStart 'Wait' {
  };
end workflow;`

func rewriteStoredWorkflow(t *testing.T, stored *workflows.Workflow, mod *model.Module, script string) *workflows.Workflow {
	t.Helper()
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)
	var written *workflows.Workflow
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{stored}, nil },
		ListPagesFunc:     func() ([]*pages.Page, error) { return []*pages.Page{mkPage(mod.ID, "P")}, nil },
		UpdateWorkflowFunc: func(wf *workflows.Workflow) error {
			written = wf
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog := parseMDL(t, script)
	if err := execCreateWorkflow(ctx, prog.Statements[0].(*ast.CreateWorkflowStmt)); err != nil {
		t.Fatalf("create or modify: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

func TestCreateOrModifyWorkflow_CarriesNamesDescribeCannotPrint(t *testing.T) {
	mod := mkModule("MyModule")
	got := rewriteStoredWorkflow(t, storedNamedWorkflow(mod), mod, namedWorkflowRewrite)

	acts := got.Flow.Activities
	if n := acts[0].GetName(); n != "start1" {
		t.Errorf("main start = %q, want the stored start1", n)
	}
	if n := acts[len(acts)-1].GetName(); n != "end1" {
		t.Errorf("main end = %q, want the stored end1", n)
	}
	task, ok := acts[1].(*workflows.UserTask)
	if !ok {
		t.Fatalf("activity 1 is %T, want the user task", acts[1])
	}
	if f := task.Outcomes[0].Flow; f == nil || len(f.Activities) != 1 || f.Activities[0].GetName() != "end3" {
		t.Errorf("the end in outcome 'Reject' was not named end3: %+v", f)
	}
	if len(got.EventSubProcesses) != 1 {
		t.Fatalf("event sub-processes = %d, want 1", len(got.EventSubProcesses))
	}
	espActs := got.EventSubProcesses[0].Flow.Activities
	if n := espActs[len(espActs)-1].GetName(); n != "end2" {
		t.Errorf("event sub-process end = %q, want the stored end2", n)
	}
}

// Control: with nothing stored under those names (a workflow mxcli wrote), the
// rebuild names the implicit activities as it always did.
func TestCreateOrModifyWorkflow_ImplicitNamesWithoutStoredNames(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedNamedWorkflow(mod)
	stored.Flow = nil
	stored.EventSubProcesses = nil
	got := rewriteStoredWorkflow(t, stored, mod, namedWorkflowRewrite)
	acts := got.Flow.Activities
	if acts[0].GetName() != "Start" || acts[len(acts)-1].GetName() != "End" {
		t.Errorf("main flow names = %q … %q, want Start … End", acts[0].GetName(), acts[len(acts)-1].GetName())
	}
}
