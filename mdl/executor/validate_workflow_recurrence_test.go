// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// MDL-WF22 (ako/mxcli#1716): the limits Studio Pro 11.15's model validation
// puts on a recurrence (MW0004, MW0005), which mx check does not enforce.
func TestWorkflowRecurrence_Limits(t *testing.T) {
	task := func(timer string) string {
		return wfTemplateHead + "begin user task R 'R' outcomes 'Done' { } boundary event " + timer + " { }; end workflow;"
	}
	cases := []struct{ name, src, want string }{
		{"interrupting", task("interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 3 times"), "only a non-interrupting timer"},
		{"no kind named", task("timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 3 times"), "only a non-interrupting timer"},
		{"seconds", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 30 seconds at most 3 times"), "minute, hour, day, week or month"},
		{"years", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 year at most 3 times"), "MW0005"},
		{"an unknown unit", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 fortnight at most 3 times"), "fortnight"},
		{"interval 0", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 0 days at most 3 times"), "at least 1"},
		{"once in all", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 1 times"), "MW0004"},
		{"interrupting event sub-process", wfTemplateHead + "begin user task R 'R' outcomes 'Done' { }; event subprocess E 'E' on interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 3 times { }; end workflow;", "only a non-interrupting timer"},
		// --- clean: the bounds themselves ---
		{"every minute, twice", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 minute at most 2 times"), ""},
		{"months, plural", task("non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 3 Months at most 12 times"), ""},
		{"event sub-process", wfTemplateHead + "begin user task R 'R' outcomes 'Done' { }; event subprocess E 'E' on non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 week at most 4 times { }; end workflow;", ""},
		{"no recurrence", task("interrupting timer addDays([%CurrentDateTime%], 1)"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := wfRuleViolations(ValidateWorkflow(parseWorkflowStmt(t, c.src)), wfRecurrenceRule)
			switch {
			case c.want == "" && len(vs) > 0:
				t.Errorf("reported a valid recurrence: %v", vs)
			case c.want != "" && (len(vs) == 0 || !strings.Contains(vs[0].Message, c.want)):
				t.Errorf("want a violation naming %q, got %v", c.want, vs)
			}
		})
	}
	prog, errs := visitor.Build(`alter workflow M.W { insert into Review { boundary event interrupting timer addDays([%CurrentDateTime%], 7) repeat every 1 week at most 4 times { } } };`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if vs := wfRuleViolations(ValidateAlterWorkflow(prog.Statements[0].(*ast.AlterWorkflowStmt)), wfRecurrenceRule); len(vs) != 1 {
		t.Errorf("alter insert of a repeating interrupting timer: %v", vs)
	}
}

// The builder stores the unit as the IntervalType the runtime reads.
func TestWorkflowRecurrence_Builder(t *testing.T) {
	for word, want := range map[string]string{"minute": "Minute", "Hours": "Hour", "day": "Day", "WEEKS": "Week", "month": "Month"} {
		got := recurrenceFromNode(&ast.WorkflowRecurrenceNode{Interval: 2, Unit: word, MaxExecutions: 3})
		if got.IntervalType != want || got.Interval != 2 || got.MaxExecutions != 3 {
			t.Errorf("%s -> %+v, want %s", word, got, want)
		}
	}
	if recurrenceFromNode(nil) != nil {
		t.Error("no recurrence must build none")
	}
}

// Describe prints a recurrence on a boundary event and on an event
// sub-process start, and the output re-parses into the same values.
func TestWorkflowDescribe_RecurrenceReparses(t *testing.T) {
	task := &workflows.UserTask{Page: "M.P", Outcomes: []*workflows.UserTaskOutcome{{Value: "Ok"}}}
	task.Name, task.Caption = "T", "Task"
	task.BoundaryEvents = []*workflows.BoundaryEvent{
		{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 1)", Caption: "Remind",
			Recurrence: &workflows.Recurrence{IntervalType: "Day", Interval: 1, MaxExecutions: 5}},
		{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 2)",
			Recurrence: &workflows.Recurrence{IntervalType: "Hour", Interval: 6, MaxExecutions: 3}},
	}
	body := strings.Join(formatSingleActivity(task, "  "), "\n")
	if !strings.Contains(body, "repeat every 1 day at most 5 times caption 'Remind'") || !strings.Contains(body, "repeat every 6 hours at most 3 times") {
		t.Errorf("describe:\n%s", body)
	}
	prog, errs := visitor.Build("create workflow M.W parameter $C: M.E\nbegin\n" + body + "\nend workflow;")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, body)
	}
	got := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowUserTaskNode).BoundaryEvents
	if r := recurrenceFromNode(got[1].Recurrence); r == nil || *r != (workflows.Recurrence{IntervalType: "Hour", Interval: 6, MaxExecutions: 3}) {
		t.Errorf("re-parsed recurrence = %+v", r)
	}
}
