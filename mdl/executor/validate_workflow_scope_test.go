// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// wfScopeViolations returns the MDL-WF19 violations ValidateWorkflow reports for
// a workflow statement.
func wfScopeViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	var out []linter.Violation
	for _, v := range ValidateWorkflow(parseWorkflowStmt(t, src)) {
		if v.RuleID == "MDL-WF19" {
			out = append(out, v)
		}
	}
	return out
}

// MDL-WF19 (mendixlabs/mxcli#1529, #1523). A workflow expression can name only
// the context and the workflow instance. Every row below was measured with
// `mx check` on 11.6.6 and on 11.15.0 (identical verdicts): the "flagged" rows
// are CE0117 "Error(s) in expression.", the "clean" rows build with 0 errors.
// `check --references` and `exec` passed all of them before this rule.
func TestWorkflowExpressionScope(t *testing.T) {
	const head = "create workflow M.W parameter $WorkflowContext: M.Item"
	cases := []struct {
		name string
		src  string
		want string // substring of the message (or suggestion); "" = no violation
	}{
		// --- flagged: CE0117 measured ---
		{"call microflow argument $Workflow (#1529)",
			head + " begin call microflow M.ACT_Bind(Workflow = $Workflow, Item = $WorkflowContext); end workflow;",
			"$WorkflowInstance"},
		{"deprecated with() argument $Workflow (#1523)",
			head + " begin call microflow M.ACT_Bind with (Workflow = '$Workflow'); end workflow;",
			"$WorkflowInstance"},
		{"call workflow argument",
			head + " begin call workflow M.Sub(WorkflowContext = $Workflow); end workflow;",
			"$WorkflowInstance"},
		{"decision on $currentUser",
			head + " begin decision d1 $currentUser != empty outcomes true -> { } false -> { }; end workflow;",
			"$currentUser"},
		{"decision on $currentSession",
			head + " begin decision d1 $currentSession != empty outcomes true -> { } false -> { }; end workflow;",
			"$currentSession"},
		{"user task due date on $WorkflowUserTask",
			head + " begin user task u1 'U' page M.P due date addDays($WorkflowUserTask/StartTime, 1) outcomes 'Done' { }; end workflow;",
			"$WorkflowUserTask"},
		{"call microflow argument $WorkflowUserTask",
			head + " begin call microflow M.ACT_Task(Task = $WorkflowUserTask); end workflow;",
			"$WorkflowUserTask"},
		{"boundary timer on $WorkflowUserTask",
			head + " begin user task u1 'U' page M.P outcomes 'Done' { } boundary event interrupting timer addDays($WorkflowUserTask/StartTime, 3) { }; end workflow;",
			"$WorkflowUserTask"},
		{"wait for timer on $Workflow",
			head + " begin wait for timer t1 addDays($Workflow/StartTime, 1); end workflow;",
			"$WorkflowInstance"},
		{"event sub-process timer start on $Workflow",
			head + " begin call microflow M.A(Item = $WorkflowContext); event subprocess E on non interrupting timer addDays($Workflow/StartTime, 1) as eStart { call microflow M.A(Item = $WorkflowContext); }; end workflow;",
			"$WorkflowInstance"},
		{"nested in an outcome",
			head + " begin decision d1 $WorkflowContext/Ok outcomes true -> { call microflow M.ACT_Inst(Workflow = $Workflow); } false -> { }; end workflow;",
			"$WorkflowInstance"},
		{"workflow due date on $WorkflowInstance",
			head + " due date addDays($WorkflowInstance/StartTime, 2) begin call microflow M.A(Item = $WorkflowContext); end workflow;",
			"due date"},
		{"an undeclared name",
			head + " begin call microflow M.A(Item = $Request); end workflow;",
			"$Request"},

		// --- clean: 0 errors measured ---
		{"$WorkflowInstance as a call argument",
			head + " begin call microflow M.ACT_Bind(Workflow = $WorkflowInstance, Item = $WorkflowContext); end workflow;", ""},
		{"$WorkflowInstance in a decision",
			head + " begin decision d1 $WorkflowInstance/Name != empty outcomes true -> { } false -> { }; end workflow;", ""},
		{"$WorkflowInstance in a wait for timer",
			head + " begin wait for timer t1 addDays($WorkflowInstance/StartTime, 1); end workflow;", ""},
		{"$WorkflowInstance in a user task due date",
			head + " begin user task u1 'U' page M.P due date addDays($WorkflowInstance/StartTime, 1) outcomes 'Done' { }; end workflow;", ""},
		{"$WorkflowInstance in a boundary timer",
			head + " begin user task u1 'U' page M.P outcomes 'Done' { } boundary event interrupting timer addDays($WorkflowInstance/StartTime, 3) { }; end workflow;", ""},
		{"$WorkflowInstance in an event sub-process",
			head + " begin call microflow M.A(Item = $WorkflowContext); event subprocess E on non interrupting timer addDays($WorkflowInstance/StartTime, 1) as eStart { call microflow M.ACT_Inst(Workflow = $WorkflowInstance); }; end workflow;", ""},
		{"workflow due date on the context",
			head + " due date addDays([%CurrentDateTime%], length($WorkflowContext/Title)) begin call microflow M.A(Item = $WorkflowContext); end workflow;", ""},
		// Rewritten to the exact spelling on write, so not an error.
		{"$workflowContext lower case",
			head + " begin call microflow M.A(Item = $workflowContext); end workflow;", ""},
		{"$workflowInstance lower case",
			head + " begin call microflow M.ACT_Inst(Workflow = $workflowInstance); end workflow;", ""},
		{"declared header alias",
			"create workflow M.W parameter $Ctx: M.Item begin decision d1 $Ctx/Title != empty outcomes true -> { } false -> { }; end workflow;", ""},
		{"header alias named Workflow",
			"create workflow M.W parameter $Workflow: M.Item begin call microflow M.A(Item = $Workflow); end workflow;", ""},
		{"a $ inside a string literal",
			head + " begin decision d1 $WorkflowContext/Title = '$Workflow isn''t $x' outcomes true -> { } false -> { }; end workflow;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := wfScopeViolations(t, c.src)
			if c.want == "" {
				if len(vs) != 0 {
					t.Fatalf("want no MDL-WF19, got %d: %s", len(vs), vs[0].Message)
				}
				return
			}
			if len(vs) != 1 {
				t.Fatalf("want one MDL-WF19 (CE0117), got %d", len(vs))
			}
			if vs[0].Severity != linter.SeverityError {
				t.Errorf("severity = %v, want error", vs[0].Severity)
			}
			text := vs[0].Message + " " + vs[0].Suggestion
			if !strings.Contains(text, c.want) {
				t.Errorf("violation %q / %q does not mention %q", vs[0].Message, vs[0].Suggestion, c.want)
			}
			if !strings.Contains(vs[0].Message, "CE0117") {
				t.Errorf("message %q does not name CE0117", vs[0].Message)
			}
		})
	}
}

// ALTER WORKFLOW writes the same expressions — an inserted activity, a boundary
// event's timer, a due date — so the rule follows it there.
func TestWorkflowExpressionScope_Alter(t *testing.T) {
	count := func(src string) int {
		t.Helper()
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse errors: %v", errs)
		}
		stmt, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
		if !ok {
			t.Fatalf("statement is %T", prog.Statements[0])
		}
		n := 0
		for _, v := range ValidateAlterWorkflow(stmt) {
			if v.RuleID == "MDL-WF19" {
				n++
			}
		}
		return n
	}
	cases := []struct {
		src  string
		want int
	}{
		{"alter workflow M.W { insert after Review { call microflow M.ACT_Inst(Workflow = $Workflow); } };", 1},
		{"alter workflow M.W { insert after Review { call microflow M.ACT_Inst(Workflow = $WorkflowInstance); } };", 0},
		{"alter workflow M.W { set (DueDate: addDays($WorkflowInstance/StartTime, 7)); };", 1},
		{"alter workflow M.W { set (DueDate: addDays([%CurrentDateTime%], 7)); };", 0},
	}
	for _, c := range cases {
		if got := count(c.src); got != c.want {
			t.Errorf("%s: %d MDL-WF19, want %d", c.src, got, c.want)
		}
	}
}
