// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #1445: every user task was written with AutoAssignSingleTargetUser =
// false, MDL had no spelling for it and describe did not print it. The clause
// `auto assign single user` now states it on both task kinds.
func TestWorkflowAutoAssign_BuildsBothTaskKinds(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W parameter $C: M.Ctx begin
  user task singleTask 'c' page M.P targeting users xpath [System.UserRoles = '[%UserRole_User%]'] auto assign single user outcomes 'A' { };
  multi user task multiTask 'c' page M.P auto assign single user outcomes 'A' { };
  user task plainTask 'c' page M.P outcomes 'A' { };
end workflow;`)
	got := map[string]bool{}
	for _, a := range stmt.Activities {
		task := buildUserTask(a.(*ast.WorkflowUserTaskNode))
		got[task.Name] = task.AutoAssignSingleTargetUser
	}
	if !got["singleTask"] || !got["multiTask"] {
		t.Errorf("auto assign not carried: %v", got)
	}
	if got["plainTask"] {
		t.Error("a task without the clause must not auto-assign (Studio Pro's default is off)")
	}
}

// describe prints the clause when set and only then, and a re-parse of the
// output rebuilds the same value — so a round trip keeps it.
func TestWorkflowAutoAssign_DescribeRoundTrips(t *testing.T) {
	for _, multi := range []bool{false, true} {
		for _, on := range []bool{false, true} {
			task := &workflows.UserTask{
				BaseWorkflowActivity:       workflows.BaseWorkflowActivity{Name: "T", Caption: "c"},
				IsMulti:                    multi,
				Page:                       "M.P",
				Outcomes:                   []*workflows.UserTaskOutcome{{Value: "A", Caption: "A"}},
				AutoAssignSingleTargetUser: on,
			}
			out := strings.Join(formatSingleActivity(task, "  "), "\n")
			if strings.Contains(out, "auto assign single user") != on {
				t.Errorf("multi=%v on=%v: describe output\n%s", multi, on, out)
			}
			stmt := parseWorkflowStmt(t, "create workflow M.W begin\n"+out+"\nend workflow;")
			again := buildUserTask(stmt.Activities[0].(*ast.WorkflowUserTaskNode))
			if again.AutoAssignSingleTargetUser != on {
				t.Errorf("multi=%v on=%v did not round-trip:\n%s", multi, on, out)
			}
		}
	}
}

func parseClauseErrors(src string) []error {
	_, errs := visitor.Build(src)
	return errs
}

func TestWorkflowAutoAssign_AtMostOnce(t *testing.T) {
	errs := parseClauseErrors(`create workflow M.W begin user task T 'c' auto assign single user page M.P auto assign single user outcomes 'A' { }; end workflow;`)
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "duplicate AUTO ASSIGN SINGLE USER clause") {
		t.Errorf("want a duplicate-clause error, got %v", errs)
	}
}

// storedAutoAssign is the Studio Pro workflow fixture with the auto-assign box
// ticked on both its single (userTask1) and multi (userTask2) user task.
func storedAutoAssign() map[string]any {
	raw := studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true)
	for _, a := range raw["Flow"].(map[string]any)["Activities"].([]any) {
		if m, ok := a.(map[string]any); ok && (m["Name"] == "userTask1" || m["Name"] == "userTask2") {
			m["AutoAssignSingleTargetUser"] = true
		}
	}
	return raw
}

// The reported reset: `create or modify workflow` rebuilt every task with
// AutoAssignSingleTargetUser = false. A rewrite that does not restate a stored
// `true` is refused, naming the task; one that restates it goes through.
func TestWorkflowRewrite_AutoAssignMustBeRestated(t *testing.T) {
	stmt := func(single, multi string) *ast.CreateWorkflowStmt {
		return parseWorkflowStmt(t, `create or modify workflow M.W parameter $C: M.Ctx
begin
  user task userTask1 'User Task' page M.P `+single+` outcomes 'Good' { } 'Bad' { };
  multi user task userTask2 'Multi' page M.P `+multi+` outcomes 'Fast' { } 'Furious' { };
end workflow;`)
	}
	cases := []struct{ name, single, multi, want string }{
		{"nothing restated", ``, ``, "auto-assign"},
		{"single only", `auto assign single user`, ``, "userTask2"},
		{"multi only", ``, `auto assign single user`, "userTask1"},
		{"both restated", `auto assign single user`, `auto assign single user`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, storedAutoAssign()), "wf1", "M.W", stmt(c.single, c.multi))
			switch {
			case c.want == "" && err != nil:
				t.Errorf("a rewrite restating auto-assign must be allowed, got %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Errorf("must be refused naming %q, got %v", c.want, err)
			}
		})
	}
	// Control: the fixture without the flag set is rewritten freely.
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, studioProWorkflow(false, false, false, "ConsensusCompletionCriteria", true)), "wf1", "M.W", stmt(``, ``)); err != nil {
		t.Errorf("a workflow without auto-assign must not be refused: %v", err)
	}
}

// REPLACE ACTIVITY rebuilds the task from the statement too.
func TestAlterWorkflow_ReplaceRestatesAutoAssign(t *testing.T) {
	replace := func(clauses string) *ast.AlterWorkflowStmt {
		prog, errs := visitor.Build(`alter workflow M.W replace activity userTask1 with user task userTask1 'User Task' page M.P ` + clauses + ` outcomes 'Good' { } 'Bad' { };`)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	ctx := storedReplaceCtx(t, storedAutoAssign())
	if got := validateAlterReplaceKeepsStudioProState(ctx, replace(`auto assign single user`)); len(got) != 0 {
		t.Errorf("a replacement restating auto-assign must be allowed, got %v", got)
	}
	got := validateAlterReplaceKeepsStudioProState(ctx, replace(``))
	if len(got) != 1 || !strings.Contains(got[0], "auto assign single user") {
		t.Errorf("want one refusal naming the clause, got %v", got)
	}
}
