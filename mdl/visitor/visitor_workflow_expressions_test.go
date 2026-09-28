// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): a workflow's expressions — a decision's condition, a
// timer's delay or first execution time, a due date — are written bare. The
// string form is the deprecated alias MDL-DEPR080 and builds exactly what the
// bare form builds: its content is the expression.

const wfHead = "create workflow M.WF\n  parameter $WorkflowContext: M.E\n"

func TestWorkflowExpression_BareFormIsCanonical(t *testing.T) {
	for _, tc := range []struct{ name, quoted, bare string }{
		{"decision",
			wfHead + "begin\n  decision '$WorkflowContext/Total > 1000' comment 'Large?'\n    outcomes true -> { } false -> { };\nend workflow;",
			wfHead + "begin\n  decision $WorkflowContext/Total > 1000 comment 'Large?'\n    outcomes true -> { } false -> { };\nend workflow;"},
		{"named decision",
			wfHead + "begin\n  decision Big 'toLowerCase($WorkflowContext/Name) = ''big'''\n    outcomes true -> { } false -> { };\nend workflow;",
			wfHead + "begin\n  decision Big toLowerCase($WorkflowContext/Name) = 'big'\n    outcomes true -> { } false -> { };\nend workflow;"},
		{"decision starting with a function call",
			wfHead + "begin\n  decision 'toLowerCase($WorkflowContext/Name) = ''big'''\n    outcomes true -> { } false -> { };\nend workflow;",
			wfHead + "begin\n  decision toLowerCase($WorkflowContext/Name) = 'big'\n    outcomes true -> { } false -> { };\nend workflow;"},
		{"wait for timer",
			wfHead + "begin\n  wait for timer 'addHours([%CurrentDateTime%], 1)' comment 'Wait';\nend workflow;",
			wfHead + "begin\n  wait for timer addHours([%CurrentDateTime%], 1) comment 'Wait';\nend workflow;"},
		{"named wait for timer",
			wfHead + "begin\n  wait for timer T1 'addHours([%CurrentDateTime%], 1)';\nend workflow;",
			wfHead + "begin\n  wait for timer T1 addHours([%CurrentDateTime%], 1);\nend workflow;"},
		{"workflow and user task due dates, boundary timers",
			wfHead + "  due date 'addDays([%CurrentDateTime%], 2)'\nbegin\n  user task Review 'Review'\n    due date 'addHours([%CurrentDateTime%], 4)'\n" +
				"    outcomes 'Done' { }\n    boundary event interrupting timer 'addHours([%CurrentDateTime%], 8)' { }\n" +
				"    boundary event non interrupting timer 'addHours([%CurrentDateTime%], 9)'\n    boundary event timer 'addHours([%CurrentDateTime%], 10)';\nend workflow;",
			wfHead + "  due date addDays([%CurrentDateTime%], 2)\nbegin\n  user task Review 'Review'\n    due date addHours([%CurrentDateTime%], 4)\n" +
				"    outcomes 'Done' { }\n    boundary event interrupting timer addHours([%CurrentDateTime%], 8) { }\n" +
				"    boundary event non interrupting timer addHours([%CurrentDateTime%], 9)\n    boundary event timer addHours([%CurrentDateTime%], 10);\nend workflow;"},
		{"timer event sub-process",
			wfHead + "begin\n  event subprocess Esp on interrupting timer 'addDays([%CurrentDateTime%], 30)' as Start comment 'After 30 days' { };\nend workflow;",
			wfHead + "begin\n  event subprocess Esp on interrupting timer addDays([%CurrentDateTime%], 30) as Start comment 'After 30 days' { };\nend workflow;"},
		{"alter workflow due date",
			"alter workflow M.WF set due date 'addDays([%CurrentDateTime%], 1)';",
			"alter workflow M.WF set due date addDays([%CurrentDateTime%], 1);"},
		{"alter activity due date",
			"alter workflow M.WF set activity Review due date 'addDays([%CurrentDateTime%], 1)';",
			"alter workflow M.WF set activity Review due date addDays([%CurrentDateTime%], 1);"},
		{"inserted wait for timer",
			"alter workflow M.WF insert after Review wait for timer 'addHours([%CurrentDateTime%], 1)' comment 'W';",
			"alter workflow M.WF insert after Review wait for timer addHours([%CurrentDateTime%], 1) comment 'W';"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, canon := mustBuild(t, tc.quoted), mustBuild(t, tc.bare)
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("string form built %#v\nbare form    %#v", old.Statements, canon.Statements)
			}
			for _, d := range old.Deprecations {
				if d.Code != deprecation.WorkflowStringExpression {
					t.Errorf("string form recorded %s", d.Code)
				}
			}
			if len(old.Deprecations) == 0 {
				t.Errorf("string form recorded no %s", deprecation.WorkflowStringExpression)
			}
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("bare form recorded %v, want none", got)
			}
		})
	}
}

func decisionOf(t *testing.T, body string) *ast.WorkflowDecisionNode {
	t.Helper()
	prog := mustBuild(t, wfHead+"begin\n  "+body+"\n    outcomes true -> { } false -> { };\nend workflow;")
	wf := prog.Statements[0].(*ast.CreateWorkflowStmt)
	return wf.Activities[0].(*ast.WorkflowDecisionNode)
}

// The name slot before a decision's expression: a name followed by a space is
// a name; a word directly followed by `(` is a function call.
func TestWorkflowExpression_DecisionNameOrCall(t *testing.T) {
	for _, tc := range []struct{ body, name, expr string }{
		{"decision toLowerCase($WorkflowContext/Name) = 'big'", "", "toLowerCase($WorkflowContext/Name) = 'big'"},
		{"decision BigOrder toLowerCase($WorkflowContext/Name) = 'big'", "BigOrder", "toLowerCase($WorkflowContext/Name) = 'big'"},
		{"decision BigOrder $WorkflowContext/Big", "BigOrder", "$WorkflowContext/Big"},
		{"decision BigOrder comment 'Is it big?'", "BigOrder", ""},
		{"decision $WorkflowContext/Big", "", "$WorkflowContext/Big"},
	} {
		d := decisionOf(t, tc.body)
		if d.Name != tc.name || d.Expression != tc.expr {
			t.Errorf("%q: name %q expression %q, want %q %q", tc.body, d.Name, d.Expression, tc.name, tc.expr)
		}
	}
}

// `set due date ”` clears the due date: it is no expression at all, so it is
// not an expression written in a string.
func TestWorkflowExpression_EmptyStringIsNotDeprecated(t *testing.T) {
	prog := mustBuild(t, "alter workflow M.WF set due date '';")
	if got := deprecationCodes(prog); len(got) != 0 {
		t.Errorf("recorded %v, want none", got)
	}
}

func TestWorkflowDecisionReadsBack(t *testing.T) {
	for _, tc := range []struct {
		nameClause, expr string
		want             bool
	}{
		{"", "$WorkflowContext/Total > 1000", true},
		{"", "toLowerCase($WorkflowContext/Name) = 'big'", true},
		{"", "Total + 1 > 3", false}, // `Total` would be the name
		{" decision1", "Total + 1 > 3", true},
	} {
		if got := WorkflowDecisionReadsBack(tc.nameClause, tc.expr); got != tc.want {
			t.Errorf("WorkflowDecisionReadsBack(%q, %q) = %v, want %v", tc.nameClause, tc.expr, got, tc.want)
		}
	}
}
