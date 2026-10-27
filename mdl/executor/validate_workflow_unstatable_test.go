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
	timer["Annotation"] = map[string]any{"$Type": "Workflows$Annotation", "Description": "escalation"}
	got := validateAlterReplaceKeepsStudioProState(storedReplaceCtx(t, wf), replace())
	if len(got) != 1 || !strings.Contains(got[0], "annotation 'escalation'") {
		t.Errorf("must be refused naming the timer's annotation, got %v", got)
	}
}

// ako/mxcli#1715: a timer's caption is stated now, so a stored one no longer
// refuses the rewrite — describe prints it, and the statement restates it.
func TestWorkflowRewrite_AllowsATimerCaption(t *testing.T) {
	wf, _, timer, _ := unstatableWorkflow()
	timer["Caption"] = "Escalate"
	restated := strings.Replace(wfUnstatableRestated, "interrupting timer addDays([%CurrentDateTime%], 3) { }",
		"interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate' { }", 1)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, restated)); err != nil {
		t.Errorf("a restated timer caption must be allowed, got %v", err)
	}
}

// ako/mxcli#1714: a task name, and the texts' parameters, are stated now, so a
// stored one no longer refuses the rewrite — describe prints them and the
// statement restates them.
func TestWorkflowRewrite_AllowsTemplateParameters(t *testing.T) {
	wf, task, _, _ := unstatableWorkflow()
	wf["WorkflowName"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Title")
	task["TaskName"].(map[string]any)["Text"] = "Review {1}"
	task["TaskName"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Name")
	task["TaskDescription"].(map[string]any)["Parameters"] = templateParam("$WorkflowContext/Title")
	restated := strings.Replace(wfUnstatableRestated, "  display 'W'\n", "  display 'W {1}' with ({1} = $WorkflowContext/Title)\n", 1)
	restated = strings.Replace(restated, "page M.P outcomes", "page M.P task name 'Review {1}' with ({1} = $WorkflowContext/Name) "+
		"description '{1}' with ({1} = $WorkflowContext/Title) outcomes", 1)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, restated)); err != nil {
		t.Errorf("restated templates must be allowed, got %v", err)
	}
}

// ako/mxcli#1716: a recurrence is stated now, so a stored one no longer refuses
// the rewrite — describe prints it and the statement restates it.
func TestWorkflowRewrite_AllowsARecurrence(t *testing.T) {
	wf, _, _, reminder := unstatableWorkflow()
	reminder["Recurrence"] = map[string]any{"$Type": "Workflows$LinearRecurrence", "Interval": int32(1), "IntervalType": "Day", "MaxExecutions": int32(5)}
	restated := strings.Replace(wfUnstatableRestated, "non interrupting timer addDays([%CurrentDateTime%], 1) { }",
		"non interrupting timer addDays([%CurrentDateTime%], 1) repeat every 1 day at most 5 times { }", 1)
	if err := checkNoDroppedWorkflowConstructs(rawWorkflowCtx(t, wf), "wf1", "M.W", parseWorkflowStmt(t, restated)); err != nil {
		t.Errorf("a restated recurrence must be allowed, got %v", err)
	}
}
