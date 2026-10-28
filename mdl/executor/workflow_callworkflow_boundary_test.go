// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1532. A `call workflow` activity takes timer boundary events
// in Mendix (Workflows$CallWorkflowActivity.boundaryEvents, 10.14+, the same
// floor as every other boundary-event owner), and mxcli read, wrote, described
// and ALTERed them — but `create workflow … call workflow … boundary event …`
// did not parse, so describe output of such a workflow did not re-parse either.

const cwBoundaryHead = "create workflow M.W parameter $WorkflowContext: M.Item\nbegin\n"

func cwBoundaryWF(body string) string { return cwBoundaryHead + body + "\nend workflow;" }

func firstCallWorkflow(t *testing.T, src string) *ast.WorkflowCallWorkflowNode {
	t.Helper()
	stmt := parseWorkflowStmt(t, src)
	n, ok := stmt.Activities[0].(*ast.WorkflowCallWorkflowNode)
	if !ok {
		t.Fatalf("first activity is %T, want *ast.WorkflowCallWorkflowNode", stmt.Activities[0])
	}
	return n
}

// The builder writes the parsed boundary events onto the CallWorkflowActivity,
// each with its handler flow ended by Mendix's end-of-path marker.
func TestCallWorkflowBoundary_BuilderWritesBoundaryEvents(t *testing.T) {
	n := firstCallWorkflow(t, cwBoundaryWF(`call workflow M.WF_Child(WorkflowContext = $WorkflowContext) as callChild
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { end workflow; }
    boundary event non interrupting timer addHours([%CurrentDateTime%], 4) { call microflow M.ACT_Remind; };`))
	if len(n.BoundaryEvents) != 2 {
		t.Fatalf("parsed boundary events = %d, want 2", len(n.BoundaryEvents))
	}

	act := buildCallWorkflowActivity(n)
	if len(act.BoundaryEvents) != 2 {
		t.Fatalf("written boundary events = %d, want 2", len(act.BoundaryEvents))
	}
	want := []struct{ typ, delay string }{
		{"InterruptingTimer", "addDays([%CurrentDateTime%], 1)"},
		{"NonInterruptingTimer", "addHours([%CurrentDateTime%], 4)"},
	}
	for i, w := range want {
		be := act.BoundaryEvents[i]
		if be.EventType != w.typ || be.TimerDelay != w.delay {
			t.Errorf("event %d = %s %q, want %s %q", i, be.EventType, be.TimerDelay, w.typ, w.delay)
		}
		if be.ID == "" {
			t.Errorf("event %d has no ID", i)
		}
		if be.Flow == nil || len(be.Flow.Activities) == 0 {
			t.Errorf("event %d has no handler flow", i)
		}
	}
	if _, ok := act.BoundaryEvents[1].Flow.Activities[0].(*workflows.CallMicroflowTask); !ok {
		t.Errorf("non-interrupting handler's first activity = %T, want the call microflow", act.BoundaryEvents[1].Flow.Activities[0])
	}
	// The call itself is unchanged by the clause.
	if act.Name != "callChild" || act.Workflow != "M.WF_Child" {
		t.Errorf("call = %q -> %q, want callChild -> M.WF_Child", act.Name, act.Workflow)
	}
}

// Control: a call with no clause writes no boundary events.
func TestCallWorkflowBoundary_NoClauseNoEvents(t *testing.T) {
	n := firstCallWorkflow(t, cwBoundaryWF(`call workflow M.WF_Child;`))
	if act := buildCallWorkflowActivity(n); len(act.BoundaryEvents) != 0 {
		t.Errorf("boundary events = %d, want 0", len(act.BoundaryEvents))
	}
}

// Every workflow rule walks nested flows through walkWorkflowActivities and its
// siblings; a boundary body under `call workflow` must be reached like one under
// `call microflow`. Each "flagged" row would be a build error; each control row
// is the same body where Mendix accepts it.
func TestCallWorkflowBoundary_ValidatorRecursesIntoBody(t *testing.T) {
	cases := []struct {
		name, body, rule string
		want             bool
	}{
		// MDL-WF08 (CE1844): no end inside a non-interrupting boundary path.
		{"WF08 end under non-interrupting",
			`call workflow M.WF_Child boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { end workflow; };`,
			"MDL-WF08", true},
		{"WF08 control: end under interrupting",
			`call workflow M.WF_Child boundary event interrupting timer addDays([%CurrentDateTime%], 1) { end workflow; };`,
			"MDL-WF08", false},
		// MDL-WF09 (CE6671): an activity after an end in the body.
		{"WF09 activity after end in body",
			`call workflow M.WF_Child boundary event interrupting timer addDays([%CurrentDateTime%], 1) { end workflow; call microflow M.ACT_X; };`,
			"MDL-WF09", true},
		// MDL-WF18 (CE1613): a nested call workflow keyed on a non-context name.
		{"WF18 nested call workflow key",
			`call workflow M.WF_Child boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call workflow M.WF_Other(Task = $WorkflowContext); };`,
			"MDL-WF18", true},
		{"WF18 control: nested key is WorkflowContext",
			`call workflow M.WF_Child boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call workflow M.WF_Other(WorkflowContext = $WorkflowContext); };`,
			"MDL-WF18", false},
		// MDL-WF19 (CE0117): the timer expression and expressions in the body.
		{"WF19 timer on $WorkflowUserTask",
			`call workflow M.WF_Child boundary event interrupting timer addDays($WorkflowUserTask/StartTime, 1) { };`,
			"MDL-WF19", true},
		{"WF19 argument in the body",
			`call workflow M.WF_Child boundary event interrupting timer addDays([%CurrentDateTime%], 1) { call microflow M.ACT_X(Workflow = $Workflow); };`,
			"MDL-WF19", true},
		{"WF19 control: timer on the workflow instance",
			`call workflow M.WF_Child boundary event interrupting timer addDays($WorkflowInstance/StartTime, 1) { };`,
			"MDL-WF19", false},
		// MDL-WF05: an activity inside the body is a jump target like any other.
		{"WF05 control: jump to an activity in the body",
			`call workflow M.WF_Child boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { user task Escalate 'Escalate' page M.P outcomes 'a' { } 'b' { }; };
  user task Next 'Next' page M.P outcomes 'a' { } 'b' { jump to Escalate; };`,
			"MDL-WF05", false},
		// MDL-WF15: at most one interrupting boundary event per activity.
		{"WF15 two interrupting",
			`call workflow M.WF_Child
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { }
    boundary event interrupting timer addDays([%CurrentDateTime%], 2) { };`,
			"MDL-WF15", true},
		{"WF15 control: one of each",
			`call workflow M.WF_Child
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { }
    boundary event non interrupting timer addDays([%CurrentDateTime%], 2) { };`,
			"MDL-WF15", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []linter.Violation
			for _, v := range ValidateWorkflow(parseWorkflowStmt(t, cwBoundaryWF(c.body))) {
				if v.RuleID == c.rule {
					got = append(got, v)
				}
			}
			if (len(got) > 0) != c.want {
				t.Errorf("%s fired = %v, want %v: %v", c.rule, len(got) > 0, c.want, got)
			}
		})
	}
}

// The helpers that count or classify boundary events feed the rewrite guard
// (#948), the bare-timer refusal (MDL-WF07) and the notification version gate.
func TestCallWorkflowBoundary_CountedAndClassified(t *testing.T) {
	stmt := parseWorkflowStmt(t, cwBoundaryWF(`call workflow M.WF_Child
    boundary event interrupting notification Withdrawn { }
    boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { };`))
	if n := countAuthoredBoundaryEvents(stmt.Activities); n != 2 {
		t.Errorf("countAuthoredBoundaryEvents = %d, want 2", n)
	}
	if !workflowUsesNotificationEvents(stmt.Activities) {
		t.Error("a notification boundary event on call workflow must trip the 11.11 notification gate")
	}
}

// Describe output of a call workflow with boundary events re-parses with both
// events intact — the round trip the missing grammar clause broke.
func TestCallWorkflowBoundary_DescribeReparses(t *testing.T) {
	call := &workflows.CallWorkflowActivity{Workflow: "M.WF_Child", ParameterExpression: "$WorkflowContext"}
	call.Name, call.Caption = "callChild", "Call child"
	call.BoundaryEvents = []*workflows.BoundaryEvent{
		{EventType: "InterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 3)"},
		{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 1)"},
	}
	body := strings.Join(formatSingleActivity(call, "  "), "\n")
	src := "create workflow M.W parameter $C: M.E\nbegin\n" + body + "\nend workflow;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, src)
	}
	n, ok := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowCallWorkflowNode)
	if !ok {
		t.Fatalf("re-parsed activity is not a call workflow\n%s", src)
	}
	if len(n.BoundaryEvents) != 2 {
		t.Errorf("boundary events after re-parse = %d, want 2\n%s", len(n.BoundaryEvents), src)
	}
}

// ALTER's end-ancestry check reads the STORED workflow: a task inside a
// non-interrupting boundary path of a call workflow is "under a non-interrupting
// boundary" (CE1844 for an end there) exactly as under a user task's.
func TestCallWorkflowBoundary_StoredEndAncestry(t *testing.T) {
	task := &workflows.UserTask{Outcomes: []*workflows.UserTaskOutcome{{Value: "a"}, {Value: "b"}}}
	task.Name = "InCallBoundary"
	call := &workflows.CallWorkflowActivity{Workflow: "WF.Child"}
	call.Name = "callChild"
	call.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType: "NonInterruptingTimer",
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{task}},
	}}
	wf := &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call}}}
	wf.ContainerID = "mod1"
	mod := &model.Module{Name: "WF"}
	mod.ID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	for src, refused := range map[string]bool{
		`alter workflow WF.W insert outcome 'Stop' on InCallBoundary { end workflow; };`: true,
		// Control: same target, nothing that ends.
		`alter workflow WF.W insert outcome 'Stop' on InCallBoundary { };`: false,
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		got := validateAlterWorkflowEndAncestry(ctx, prog.Statements[0].(*ast.AlterWorkflowStmt))
		if (len(got) > 0) != refused {
			t.Errorf("%s: refused = %v, want %v: %v", src, len(got) > 0, refused, got)
		}
	}
}
