// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// R5 (ako/mxcli#753): describe writes a workflow's expressions bare. The
// output must re-parse to the stored expression byte for byte; one that would
// not is written in the deprecated string form.

func describedWorkflowActivity(t *testing.T, act workflows.WorkflowActivity) (string, *ast.Program) {
	t.Helper()
	out := strings.Join(formatWorkflowActivities(nil, &workflows.Flow{Activities: []workflows.WorkflowActivity{act}}, "  "), "\n")
	prog, errs := visitor.Build("create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" + out + "\nend workflow;")
	if len(errs) > 0 {
		t.Fatalf("does not re-parse: %v\n%s", errs, out)
	}
	return out, prog
}

func onlyWorkflowStringExpression(prog *ast.Program) bool {
	for _, d := range prog.Deprecations {
		if d.Code != deprecation.WorkflowStringExpression {
			return false
		}
	}
	return len(prog.Deprecations) > 0
}

func TestDescribeWorkflow_DecisionExpressionIsBare(t *testing.T) {
	for _, tc := range []struct {
		name, caption, expr, want string
		quoted                    bool
	}{
		{"decision1", "Large order?", "$WorkflowContext/Total > 1000",
			"decision decision1 $WorkflowContext/Total > 1000 caption 'Large order?'", false},
		{"BigOrder", "BigOrder", "toLowerCase($WorkflowContext/Name) = 'big'",
			"decision toLowerCase($WorkflowContext/Name) = 'big'", false},
		// Written after no name, `Total` would be read as the name: the name
		// is written, which stores the same name.
		{"BigOrder", "BigOrder", "Total + 1 > 3", "decision BigOrder Total + 1 > 3", false},
		// Not an MDL expression: the string carries it as stored.
		{"decision1", "Decision", "${PT1H}", "decision decision1 '${PT1H}'", true},
	} {
		split := &workflows.ExclusiveSplitActivity{Expression: tc.expr}
		split.Name, split.Caption = tc.name, tc.caption
		out, prog := describedWorkflowActivity(t, split)
		if !strings.Contains(out, tc.want) {
			t.Errorf("describe wrote:\n%s\nwant a line containing %q", out, tc.want)
		}
		d := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowDecisionNode)
		// A name the writer derives from the caption is not written.
		if d.Expression != tc.expr || (d.Name != tc.name && !(d.Name == "" && tc.name == tc.caption)) {
			t.Errorf("%q re-parsed as name %q expression %q", tc.want, d.Name, d.Expression)
		}
		if got := onlyWorkflowStringExpression(prog); got != tc.quoted {
			t.Errorf("%q: deprecations %v", tc.want, prog.Deprecations)
		}
	}
}

func TestDescribeWorkflow_TimersAndDueDatesAreBare(t *testing.T) {
	timer := &workflows.WaitForTimerActivity{DelayExpression: "addHours([%CurrentDateTime%], 1)"}
	timer.Name, timer.Caption = "wait1", "Wait"
	out, prog := describedWorkflowActivity(t, timer)
	if !strings.Contains(out, "wait for timer wait1 addHours([%CurrentDateTime%], 1) caption 'Wait'") {
		t.Errorf("describe wrote:\n%s", out)
	}
	if len(prog.Deprecations) != 0 {
		t.Errorf("deprecations %v", prog.Deprecations)
	}

	task := &workflows.UserTask{DueDate: "addDays([%CurrentDateTime%], 3)"}
	task.Name, task.Caption = "Review", "Review"
	task.BoundaryEvents = []*workflows.BoundaryEvent{{EventType: "InterruptingTimer", TimerDelay: "addHours([%CurrentDateTime%], 8)"}}
	out, prog = describedWorkflowActivity(t, task)
	for _, want := range []string{"due date addDays([%CurrentDateTime%], 3)", "boundary event interrupting timer addHours([%CurrentDateTime%], 8)"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe wrote:\n%s\nwant %q", out, want)
		}
	}
	if len(prog.Deprecations) != 0 {
		t.Errorf("deprecations %v", prog.Deprecations)
	}
	ut := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowUserTaskNode)
	if ut.DueDate != task.DueDate || len(ut.BoundaryEvents) != 1 || ut.BoundaryEvents[0].Delay != "addHours([%CurrentDateTime%], 8)" {
		t.Errorf("re-parsed as %#v", ut)
	}
}
