// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1713: settings a Studio Pro author gives a workflow and MDL cannot
// state were reset by a describe -> create or modify round trip without a word.

// unstatableWorkflow is a stored workflow with one user task, Review, and two
// timer boundary events, holding only what a rebuild writes anyway — the
// shape the probe app held before its settings were given (11.15.0).
func unstatableWorkflow() (wf, task, timer, reminder map[string]any) {
	template := func(text string) map[string]any {
		return map[string]any{"$Type": "Microflows$StringTemplate", "Text": text, "Parameters": []any{2}}
	}
	timer = map[string]any{"$Type": "Workflows$InterruptingTimerBoundaryEvent", "Caption": "", "FirstExecutionTime": "addDays([%CurrentDateTime%], 3)"}
	reminder = map[string]any{"$Type": "Workflows$NonInterruptingTimerBoundaryEvent", "Caption": "", "Recurrence": nil, "FirstExecutionTime": "addDays([%CurrentDateTime%], 1)"}
	task = map[string]any{
		"$Type": "Workflows$SingleUserTaskActivity", "Name": "Review", "Caption": "Review",
		"TaskName": template("Review"), "TaskDescription": template(""),
		"OnCreatedEvent": map[string]any{"$Type": "Workflows$NoEvent"},
		"BoundaryEvents": []any{2, timer, reminder},
	}
	wf = map[string]any{
		"$Type":               "Workflows$Workflow",
		"WorkflowName":        template("W"),
		"WorkflowDescription": template(""),
		"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{3,
			map[string]any{"$Type": "Workflows$StartWorkflowActivity", "Name": "start1"},
			task,
			map[string]any{"$Type": "Workflows$EndWorkflowActivity", "Name": "end1"},
		}},
		"OnWorkflowEvent": []any{2},
	}
	return wf, task, timer, reminder
}

func templateParam(expr string) []any {
	return []any{2, map[string]any{"$Type": "Microflows$TemplateParameter", "Expression": expr}}
}

const wfUnstatableRestated = `create or modify workflow M.W parameter $WorkflowContext: M.Ctx
  display 'W'
begin
  user task Review 'Review' page M.P outcomes 'Done' { }
    boundary event interrupting timer addDays([%CurrentDateTime%], 3) { }
    boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { };
end workflow;`

func TestWorkflowRewrite_RefusesResettingUnstatableSettings(t *testing.T) {
	cases := []struct {
		name string
		set  func(wf, task, timer, reminder map[string]any)
		want string
	}{
		{"task name parameter", func(_, task, _, _ map[string]any) {
			task["TaskName"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Name")
		}, "'Review' with ({1} = $WorkflowContext/Name)"},
		{"task name other than the caption", func(_, task, _, _ map[string]any) {
			task["TaskName"].(map[string]any)["Text"] = "Review the request"
		}, "task name 'Review the request' is not its caption 'Review'"},
		{"task description parameter", func(_, task, _, _ map[string]any) {
			task["TaskDescription"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Title")
		}, "description '' with ({1} = $WorkflowContext/Title)"},
		{"workflow display parameter", func(wf, _, _, _ map[string]any) {
			wf["WorkflowName"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Title")
		}, "display 'W' with ({1} = $WorkflowContext/Title)"},
		{"timer caption", func(_, _, timer, _ map[string]any) { timer["Caption"] = "Escalate" },
			"interrupting timer boundary event has caption 'Escalate'"},
		{"recurrence", func(_, _, _, reminder map[string]any) {
			reminder["Recurrence"] = map[string]any{"$Type": "Workflows$LinearRecurrence", "Interval": int32(1), "IntervalType": "Day", "MaxExecutions": int32(5)}
		}, "repeats (every 1 Day, at most 5 times)"},
		{"boundary event annotation", func(_, _, _, reminder map[string]any) {
			reminder["Annotation"] = map[string]any{"$Type": "Workflows$Annotation", "Description": "nudge"}
		}, "annotation 'nudge'"},
	}
	stmt := parseWorkflowStmt(t, wfUnstatableRestated)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wf, task, timer, reminder := unstatableWorkflow()
			c.set(wf, task, timer, reminder)
			err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", stmt)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("rewrite must be refused naming %q, got %v", c.want, err)
			}
		})
	}
}

// CONTROLS: what a rebuild reproduces is not refused — the stored workflow
// holding only that; a task name the statement's caption restates; and a task
// the statement drops, whose name goes with it on purpose.
func TestWorkflowRewrite_AllowsWhatARebuildRestates(t *testing.T) {
	wf, _, _, _ := unstatableWorkflow()
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, wfUnstatableRestated)); err != nil {
		t.Errorf("nothing to lose, got %v", err)
	}

	wf, task, _, _ := unstatableWorkflow()
	task["TaskName"].(map[string]any)["Text"] = "Review the request"
	task["Caption"] = "Review the request"
	restated := strings.Replace(wfUnstatableRestated, "user task Review 'Review'", "user task Review 'Review the request'", 1)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, restated)); err != nil {
		t.Errorf("the caption restates the task name, got %v", err)
	}

	wf, task, _, _ = unstatableWorkflow()
	task["TaskName"].(map[string]any)["Text"] = "Review the request"
	task["BoundaryEvents"] = []any{2}
	dropped := `create or modify workflow M.W parameter $WorkflowContext: M.Ctx
  display 'W'
begin
  user task Approve 'Approve' page M.P outcomes 'Done' { };
end workflow;`
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, dropped)); err != nil {
		t.Errorf("a dropped task's name is dropped on purpose, got %v", err)
	}
}

// REPLACE ACTIVITY rebuilds the activity from the statement, so it resets the
// same settings on the activity it replaces.
func TestAlterWorkflow_ReplaceRefusesToResetUnstatableSettings(t *testing.T) {
	replace := func() *ast.AlterWorkflowStmt {
		prog, errs := visitor.Build(`alter workflow M.W replace activity userTask1 with user task userTask1 'Review' page M.P outcomes 'Done' { }
  boundary event interrupting timer addDays([%CurrentDateTime%], 3) { };`)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	wf, task, timer, _ := unstatableWorkflow()
	task["Name"] = "userTask1"
	if got := validateAlterReplaceKeepsStudioProState(storedReplaceCtx(t, wf), replace()); len(got) != 0 {
		t.Fatalf("control: nothing to lose, got %v", got)
	}
	timer["Caption"] = "Escalate"
	got := validateAlterReplaceKeepsStudioProState(storedReplaceCtx(t, wf), replace())
	if len(got) != 1 || !strings.Contains(got[0], "caption 'Escalate'") {
		t.Errorf("must be refused naming the timer's caption, got %v", got)
	}
}
