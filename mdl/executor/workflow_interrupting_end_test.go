// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// An interrupting boundary path written from MDL that runs to its last activity
// ended in the non-interrupting end-of-path marker; Studio Pro ends it with an
// End, and an evaluator read the two as mxcli "mixing" them (ako/mxcli#1712).

func lastActivity(acts []workflows.WorkflowActivity) workflows.WorkflowActivity {
	if len(acts) == 0 {
		return nil
	}
	return acts[len(acts)-1]
}

// CREATE: the whole workflow is known, so the main flow, outcomes and event
// sub-process bodies get an End and a parallel split keeps the marker.
func TestCreateWorkflow_InterruptingPathsEndLikeStudioPro(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W
  parameter $Context: M.E
begin
  user task T1 'T1' page M.P outcomes 'Done' { }
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call microflow M.Act; }
    boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { call microflow M.Act; };
  parallel split s1
    path 1 {
      user task T2 'T2' page M.P outcomes 'Done' { }
        boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call microflow M.Act; };
    }
    path 2 { call microflow M.Act; };

  event subprocess ESP 'ESP' on non interrupting notification espStart 'Start' {
    user task T3 'T3' page M.P outcomes 'Done' { }
      boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call microflow M.Act; };
  };
end workflow;`)
	acts := buildWorkflowActivities(stmt.Activities)
	esps := buildEventSubProcesses(stmt.EventSubProcesses)
	endBoundaryPathsLikeStudioPro(acts, esps)

	t1 := acts[0].(*workflows.UserTask)
	if _, ok := lastActivity(t1.BoundaryEvents[0].Flow.Activities).(*workflows.EndWorkflowActivity); !ok {
		t.Errorf("main-flow interrupting path ends with %T, want *workflows.EndWorkflowActivity",
			lastActivity(t1.BoundaryEvents[0].Flow.Activities))
	}
	if _, ok := lastActivity(t1.BoundaryEvents[1].Flow.Activities).(*workflows.EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("non-interrupting path ends with %T, want the marker", lastActivity(t1.BoundaryEvents[1].Flow.Activities))
	}
	t2 := acts[1].(*workflows.ParallelSplitActivity).Outcomes[0].Flow.Activities[0].(*workflows.UserTask)
	if _, ok := lastActivity(t2.BoundaryEvents[0].Flow.Activities).(*workflows.EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("interrupting path in a parallel split ends with %T, want the marker (an End is CE1844 there)",
			lastActivity(t2.BoundaryEvents[0].Flow.Activities))
	}
	var t3 *workflows.UserTask
	for _, a := range esps[0].Flow.Activities {
		if ut, ok := a.(*workflows.UserTask); ok {
			t3 = ut
		}
	}
	if t3 == nil {
		t.Fatal("event sub-process has no user task")
	}
	if _, ok := lastActivity(t3.BoundaryEvents[0].Flow.Activities).(*workflows.EndWorkflowActivity); !ok {
		t.Errorf("interrupting path in an event sub-process ends with %T, want *workflows.EndWorkflowActivity",
			lastActivity(t3.BoundaryEvents[0].Flow.Activities))
	}
}

// interruptingEndCtx is genericWfCtx's workflow with a user task inside the
// parallel split's first path.
func interruptingEndCtx(t *testing.T, mut *mock.MockWorkflowMutator) *ExecContext {
	t.Helper()
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "a1"}, Name: "task1"}},
		&workflows.ParallelSplitActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "a3"}, Name: "split1"},
			Outcomes: []*workflows.ParallelSplitOutcome{
				{Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
					&workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
						BaseElement: model.BaseElement{ID: "a4"}, Name: "taskInSplit"}},
				}}},
				{},
			}},
	}}
	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListWorkflowsFunc:           func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
		OpenWorkflowForMutationFunc: func(model.ID) (backend.WorkflowMutator, error) { return mut, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// ALTER … insert into X { boundary event interrupting … }: the path handed to
// the mutator ends in an End where the stored workflow allows one; the mutator
// then adds no marker (EndBoundaryEventPath leaves an ended path alone).
func TestAlterInsertBoundaryEvent_InterruptingPathEndsLikeStudioPro(t *testing.T) {
	var got map[string][]workflows.WorkflowActivity
	mut := &mock.MockWorkflowMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			return backend.AlterTargetMatch{Kind: "user task", Name: backend.WorkflowTargetText(tg)}, nil
		},
		InsertBoundaryEventFunc: func(ref string, _ int, eventType, _ string, acts []workflows.WorkflowActivity) error {
			got[ref+" "+eventType] = acts
			return nil
		},
	}
	for _, tc := range []struct {
		src, key string
		wantEnd  bool
	}{
		{`alter workflow Sales.WF { insert into task1 { boundary event interrupting timer addDays([%CurrentDateTime%], 1) { wait for notification wn1; } } };`,
			"task1 InterruptingTimer", true},
		// CONTROL: a non-interrupting path keeps the marker the mutator appends.
		{`alter workflow Sales.WF { insert into task1 { boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { wait for notification wn1; } } };`,
			"task1 NonInterruptingTimer", false},
		// CONTROL: under a parallel split an End is CE1844, so the marker stays.
		{`alter workflow Sales.WF { insert into taskInSplit { boundary event interrupting timer addDays([%CurrentDateTime%], 1) { wait for notification wn1; } } };`,
			"taskInSplit InterruptingTimer", false},
	} {
		got = map[string][]workflows.WorkflowActivity{}
		if err := execAlterWf(t, interruptingEndCtx(t, mut), tc.src); err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
		acts, ok := got[tc.key]
		if !ok {
			t.Fatalf("%s: InsertBoundaryEvent was not called (calls: %v)", tc.key, got)
		}
		_, isEnd := lastActivity(acts).(*workflows.EndWorkflowActivity)
		if isEnd != tc.wantEnd {
			t.Errorf("%s: path ends with %T, want End = %v", tc.key, lastActivity(acts), tc.wantEnd)
		}
	}
}

// ALTER … insert after X { user task … boundary event interrupting … }: a
// boundary event nested in inserted activities follows the same rule.
func TestAlterInsertAfter_NestedInterruptingPathEndsLikeStudioPro(t *testing.T) {
	var acts []workflows.WorkflowActivity
	mut := &mock.MockWorkflowMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			return backend.AlterTargetMatch{Kind: "user task", Name: backend.WorkflowTargetText(tg)}, nil
		},
		InsertAfterActivityFunc: func(_ string, _ int, a []workflows.WorkflowActivity) error { acts = a; return nil },
	}
	for _, tc := range []struct {
		target  string
		wantEnd bool
	}{{"task1", true}, {"taskInSplit", false}} {
		acts = nil
		src := `alter workflow Sales.WF { insert after ` + tc.target + ` {
  user task Escalate 'Escalate' outcomes 'Done' { }
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { wait for notification wn1; };
} };`
		if err := execAlterWf(t, interruptingEndCtx(t, mut), src); err != nil {
			t.Fatalf("%s: %v", tc.target, err)
		}
		if len(acts) != 1 {
			t.Fatalf("%s: inserted %d activities, want 1", tc.target, len(acts))
		}
		path := acts[0].(*workflows.UserTask).BoundaryEvents[0].Flow.Activities
		if _, isEnd := lastActivity(path).(*workflows.EndWorkflowActivity); isEnd != tc.wantEnd {
			t.Errorf("after %s: path ends with %T, want End = %v", tc.target, lastActivity(path), tc.wantEnd)
		}
	}
}
