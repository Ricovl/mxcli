// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1714: a workflow's display and description, and a user task's task
// name and description, take `with ({1} = <expression>, …)`. Before, `with`
// after `display '…'` was "extraneous input 'WITH'".
func TestWorkflowVisitor_TextTemplates(t *testing.T) {
	src := `create workflow M.WF
  parameter $WorkflowContext: M.E
  display 'Approval for {1} by {2}' with ({1} = $WorkflowContext/Title, {2} = $WorkflowContext/SubmittedBy)
  description 'Worth {1}' with ({1} = toString($WorkflowContext/Value))
begin
  user task Review 'Review'
    task name 'Review {1}' with ({1} = $WorkflowContext/Title)
    description 'Started {1}' with ({1} = $WorkflowInstance/Name)
    outcomes 'Done' { };
  multi user task Board 'Board'
    task name 'Board'
    outcomes 'Done' { };
end workflow;`
	wf := mustBuild(t, src).Statements[0].(*ast.CreateWorkflowStmt)
	p := func(pairs ...string) []ast.WorkflowTemplateParam {
		var out []ast.WorkflowTemplateParam
		for i, e := range pairs {
			out = append(out, ast.WorkflowTemplateParam{Index: i + 1, Expression: e})
		}
		return out
	}
	check := func(what string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", what, got, want)
		}
	}
	check("display", wf.DisplayName, "Approval for {1} by {2}")
	check("display params", wf.DisplayParams, p("$WorkflowContext/Title", "$WorkflowContext/SubmittedBy"))
	check("description params", wf.DescriptionParams, p("toString($WorkflowContext/Value)"))
	review := wf.Activities[0].(*ast.WorkflowUserTaskNode)
	check("task name", review.TaskName, "Review {1}")
	check("task name params", review.TaskNameParams, p("$WorkflowContext/Title"))
	check("task description params", review.TaskDescriptionParams, p("$WorkflowInstance/Name"))
	board := wf.Activities[1].(*ast.WorkflowUserTaskNode)
	check("multi task name", board.TaskName, "Board")
	check("multi task name params", len(board.TaskNameParams), 0)
}

// CONTROL: a workflow without the new clauses builds as before.
func TestWorkflowVisitor_TextTemplatesAbsent(t *testing.T) {
	wf := mustBuild(t, "create workflow M.WF parameter $WorkflowContext: M.E display 'Plain' begin user task R 'R' outcomes 'Done' { }; end workflow;").Statements[0].(*ast.CreateWorkflowStmt)
	r := wf.Activities[0].(*ast.WorkflowUserTaskNode)
	if wf.DisplayName != "Plain" || wf.DisplayParams != nil || r.TaskName != "" || r.TaskNameParams != nil {
		t.Errorf("built %+v / %+v", wf, r)
	}
}

// `task <word>` parses for any identifier; only `name` is a clause.
func TestWorkflowVisitor_TaskNameWordIsChecked(t *testing.T) {
	_, errs := Build("create workflow M.WF parameter $WorkflowContext: M.E begin user task R 'R' task nam 'X' outcomes 'Done' { }; end workflow;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "task name '…'") {
		t.Errorf("want an error pointing at `task name`, got %v", errs)
	}
	if _, errs := Build("create workflow M.WF parameter $WorkflowContext: M.E begin user task R 'R' TASK NAME 'X' outcomes 'Done' { }; end workflow;"); len(errs) != 0 {
		t.Errorf("`TASK NAME` in upper case must parse: %v", errs)
	}
	_, errs = Build("create workflow M.WF parameter $WorkflowContext: M.E begin user task R 'R' task name 'X' task name 'Y' outcomes 'Done' { }; end workflow;")
	if len(errs) == 0 {
		t.Error("a repeated `task name` must be refused like every other repeated clause")
	}
}

func TestAlterWorkflow_SetTextTemplates(t *testing.T) {
	stmt := mustBuild(t, `alter workflow M.WF {
  set (Display: 'Approve {1}' with ({1} = $WorkflowContext/Title), Description: 'plain');
  set (TaskName: 'Check {1}' with ({1} = $WorkflowContext/Title), Description: 'Due {1}' with ({1} = $WorkflowInstance/Name)) on Review;
};`).Statements[0].(*ast.AlterWorkflowStmt)
	var got []string
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.SetWorkflowPropertyOp:
			got = append(got, o.Property+"="+o.Value+paramsText(o.Params))
		case *ast.SetActivityPropertyOp:
			got = append(got, o.ActivityRef+"."+o.Property+"="+o.Value+paramsText(o.Params))
		}
	}
	want := []string{
		"display=Approve {1}[$WorkflowContext/Title]",
		"description=plain",
		"Review.task_name=Check {1}[$WorkflowContext/Title]",
		"Review.description=Due {1}[$WorkflowInstance/Name]",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ops\n got %q\nwant %q", got, want)
	}
}

func paramsText(ps []ast.WorkflowTemplateParam) string {
	if len(ps) == 0 {
		return ""
	}
	var parts []string
	for _, p := range ps {
		parts = append(parts, p.Expression)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
