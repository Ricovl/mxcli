// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1716: a non-interrupting timer — a boundary event or an event
// sub-process's start — takes `repeat every <n> <unit> at most <m> times`.
func TestWorkflowVisitor_Recurrence(t *testing.T) {
	src := wfHead + `begin
  user task R 'R'
    outcomes 'Done' { }
    boundary event non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 5 times caption 'Remind' { }
    boundary event non interrupting timer repeat every 2 Hours at most 3 times;
  event subprocess Nudge 'Nudge'
    on non interrupting timer addHours([%CurrentDateTime%], 2) repeat every 1 week at most 4 times as NudgeStart {
  };
end workflow;`
	wf := mustBuild(t, src).Statements[0].(*ast.CreateWorkflowStmt)
	events := wf.Activities[0].(*ast.WorkflowUserTaskNode).BoundaryEvents
	if len(events) != 2 {
		t.Fatalf("built %d boundary events, want 2", len(events))
	}
	want := func(what string, got *ast.WorkflowRecurrenceNode, interval int, unit string, max int) {
		t.Helper()
		if got == nil || got.Interval != interval || got.Unit != unit || got.MaxExecutions != max {
			t.Errorf("%s recurrence = %+v, want every %d %s at most %d", what, got, interval, unit, max)
		}
	}
	want("first event", events[0].Recurrence, 1, "day", 5)
	if events[0].Caption != "Remind" {
		t.Errorf("caption after the recurrence = %q", events[0].Caption)
	}
	want("second event (no delay)", events[1].Recurrence, 2, "Hours", 3)
	if events[1].Delay != "" {
		t.Errorf("`repeat` was read as a delay: %q", events[1].Delay)
	}
	want("event sub-process", wf.EventSubProcesses[0].Recurrence, 1, "week", 4)
	if wf.EventSubProcesses[0].StartName != "NudgeStart" {
		t.Errorf("start name after the recurrence = %q", wf.EventSubProcesses[0].StartName)
	}

	op := mustBuild(t, `alter workflow M.WF { insert into Review { boundary event non interrupting timer addDays([%CurrentDateTime%], 7) repeat every 1 week at most 4 times { } } };`).
		Statements[0].(*ast.AlterWorkflowStmt).Operations[0].(*ast.InsertBoundaryEventOp)
	want("alter insert", op.Recurrence, 1, "week", 4)
}

// CONTROL: a timer without the clause has none.
func TestWorkflowVisitor_NoRecurrence(t *testing.T) {
	wf := mustBuild(t, wfHead+"begin user task R 'R' outcomes 'Done' { } boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { }; end workflow;").Statements[0].(*ast.CreateWorkflowStmt)
	if r := wf.Activities[0].(*ast.WorkflowUserTaskNode).BoundaryEvents[0].Recurrence; r != nil {
		t.Errorf("recurrence = %+v, want none", r)
	}
}

// `repeat`, `every` and `times` became keywords; they stay usable as names.
func TestWorkflowRecurrence_KeywordsAreStillNames(t *testing.T) {
	for _, src := range []string{
		"create persistent entity M.Schedule (Repeat: Boolean, Every: Integer, Times: Integer);",
		"create persistent entity M.Times (Name: String(20));",
		`create microflow M.F () returns Boolean begin
  retrieve $S from M.Schedule where [Times > 1 and Repeat = true];
  return true;
end;`,
	} {
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("%s\n%v", src, errs)
		}
	}
}
