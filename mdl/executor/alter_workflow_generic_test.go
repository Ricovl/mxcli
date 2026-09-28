// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// alter workflow on the generic ALTER (ako/mxcli#712): every operation's target
// goes through the workflow's AlterTargetResolver before anything is applied.

func genericWfCtx(t *testing.T, mut *mock.MockWorkflowMutator) *ExecContext {
	t.Helper()
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{
		&workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "a1"}, Name: "task1"}},
		&workflows.ParallelSplitActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
			BaseElement: model.BaseElement{ID: "a3"}, Name: "split1"},
			Outcomes: []*workflows.ParallelSplitOutcome{{}, {}}},
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

func execAlterWf(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	return execAlterWorkflow(ctx, prog.Statements[0].(*ast.AlterWorkflowStmt))
}

// An ambiguous target refuses the statement before the operation ahead of it
// is applied: a half-applied alter is the thing resolving first prevents.
func TestAlterWorkflowGeneric_UnresolvedTargetRefusesBeforeAnyChange(t *testing.T) {
	var applied []string
	mut := &mock.MockWorkflowMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			if backend.WorkflowTargetText(tg) == "Review" {
				return backend.AlterTargetMatch{}, &backend.AlterTargetError{Target: tg, Matches: []backend.AlterTargetMatch{
					{Kind: "user task", Name: "task1"}, {Kind: "decision", Name: "decision1"}}}
			}
			return backend.AlterTargetMatch{Kind: "user task", Name: backend.WorkflowTargetText(tg)}, nil
		},
		DropActivityFunc: func(ref string, _ int) error { applied = append(applied, "drop "+ref); return nil },
		SetActivityPropertyFunc: func(ref string, _ int, prop, _ string) error {
			applied = append(applied, "set "+prop+" on "+ref)
			return nil
		},
		SaveFunc: func() error { applied = append(applied, "save"); return nil },
	}
	ctx := genericWfCtx(t, mut)
	err := execAlterWf(t, ctx, `alter workflow Sales.WF { drop task1; set (Description: 'x') on 'Review'; };`)
	if err == nil || !strings.Contains(err.Error(), "@1 user task task1, @2 decision decision1") {
		t.Fatalf("err = %v, want the ambiguity with its matches", err)
	}
	if len(applied) != 0 {
		t.Fatalf("applied %v before refusing", applied)
	}
}

// The canonical operations reach the same mutator calls the old actions do,
// and `insert before` — which the old form could not say — reaches its own.
func TestAlterWorkflowGeneric_OperationsReachTheMutator(t *testing.T) {
	var calls []string
	rec := func(format string, a ...any) { calls = append(calls, fmt.Sprintf(format, a...)) }
	var resolved []string
	mut := &mock.MockWorkflowMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			resolved = append(resolved, tg.String())
			return backend.AlterTargetMatch{Kind: "activity", Name: backend.WorkflowTargetText(tg)}, nil
		},
		InsertBeforeActivityFunc: func(ref string, pos int, acts []workflows.WorkflowActivity) error {
			rec("before %s@%d %d", ref, pos, len(acts))
			return nil
		},
		InsertAfterActivityFunc: func(ref string, pos int, acts []workflows.WorkflowActivity) error {
			rec("after %s@%d %d", ref, pos, len(acts))
			return nil
		},
		InsertPathFunc: func(ref string, _ int, _ string, acts []workflows.WorkflowActivity) error {
			rec("path %s", ref)
			return nil
		},
		DropOutcomeFunc: func(ref string, _ int, name string) error { rec("drop outcome %s %s", ref, name); return nil },
		DropPathFunc:    func(ref string, _ int, c string) error { rec("drop path %s %s", ref, c); return nil },
		SetPropertyFunc: func(prop, v string) error { rec("set %s=%s", prop, v); return nil },
	}
	ctx := genericWfCtx(t, mut)
	err := execAlterWf(t, ctx, `alter workflow Sales.WF {
  set (Display: 'Orders');
  insert before task1 { wait for notification w1; wait for notification w2; }
  insert after 'Review'@2 { wait for notification w3; }
  insert after w3 { wait for notification w4; }
  insert into split1 { path 3 { } }
  drop task1 outcome 'Reject', split1 path 1;
};`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"set display=Orders", "before task1@0 2", "after Review@2 1", "after w3@0 1",
		"path split1", "drop outcome task1 Reject", "drop path split1 Path 1"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v\nwant    %v", calls, want)
	}
	// w3 is inserted by the statement itself, so it is resolved when its
	// operation runs, not against the workflow as stored.
	for _, r := range resolved {
		if r == "w3" {
			t.Errorf("resolved %v: w3 was looked up before the statement inserted it", resolved)
		}
	}
}

// `path n` in an insert must be the split's next path; the number is not a
// position to insert at, so any other is refused rather than silently
// appended.
func TestAlterWorkflowGeneric_PathNumberMustBeTheNextOne(t *testing.T) {
	for _, tc := range []struct {
		src     string
		refused bool
	}{
		{`alter workflow Sales.WF { insert into split1 { path 3 { } } };`, false},
		{`alter workflow Sales.WF { insert into split1 { path { } } };`, false},
		{`alter workflow Sales.WF { insert into split1 { path 3 { } path 4 { } } };`, false},
		{`alter workflow Sales.WF { insert into split1 { path 2 { } } };`, true},
		{`alter workflow Sales.WF { insert into split1 { path 3 { } path 3 { } } };`, true},
	} {
		ctx := genericWfCtx(t, &mock.MockWorkflowMutator{})
		errs := alterWfRefErrors(t, ctx, tc.src)
		got := false
		for _, e := range errs {
			if strings.Contains(e, "so a path added here is path") {
				got = true
			}
		}
		if got != tc.refused {
			t.Errorf("%s: refused = %v, want %v (%v)", tc.src, got, tc.refused, errs)
		}
	}
}
