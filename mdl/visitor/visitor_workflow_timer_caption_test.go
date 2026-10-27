// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1715: a timer boundary event takes the `caption '…'` clause every
// activity takes, after its delay. Before, `caption` there was a parse error.
func TestWorkflowVisitor_TimerBoundaryEventCaption(t *testing.T) {
	for _, tc := range []struct {
		name, events string
		want         []ast.WorkflowBoundaryEventNode
	}{
		{"after the delay",
			"boundary event interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate'",
			[]ast.WorkflowBoundaryEventNode{{EventType: "InterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)", Caption: "Escalate"}}},
		{"non-interrupting, then a body",
			"boundary event non interrupting timer addDays([%CurrentDateTime%], 1) caption 'Remind' { }",
			[]ast.WorkflowBoundaryEventNode{{EventType: "NonInterruptingTimer", Delay: "addDays([%CurrentDateTime%], 1)", Caption: "Remind"}}},
		{"with no delay — `caption` is not taken for the delay",
			"boundary event interrupting timer caption 'Escalate'",
			[]ast.WorkflowBoundaryEventNode{{EventType: "InterruptingTimer", Caption: "Escalate"}}},
		{"two events, the first captioned",
			"boundary event interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate' non interrupting timer addDays([%CurrentDateTime%], 1)",
			[]ast.WorkflowBoundaryEventNode{
				{EventType: "InterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)", Caption: "Escalate"},
				{EventType: "NonInterruptingTimer", Delay: "addDays([%CurrentDateTime%], 1)"},
			}},
		// CONTROL: no caption stays empty.
		{"no caption", "boundary event interrupting timer addDays([%CurrentDateTime%], 3)",
			[]ast.WorkflowBoundaryEventNode{{EventType: "InterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := wfHead + "begin\n  user task R 'R'\n    outcomes 'Done' { }\n    " + tc.events + ";\nend workflow;"
			prog := mustBuild(t, src)
			got := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowUserTaskNode).BoundaryEvents
			if len(got) != len(tc.want) {
				t.Fatalf("built %d events, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				g := got[i]
				if g.EventType != w.EventType || g.Caption != w.Caption || strings.ReplaceAll(g.Delay, " ", "") != strings.ReplaceAll(w.Delay, " ", "") {
					t.Errorf("event %d = {%s %q %q}, want {%s %q %q}", i, g.EventType, g.Delay, g.Caption, w.EventType, w.Delay, w.Caption)
				}
			}
		})
	}
}

// ALTER WORKFLOW's insert carries the caption to the mutator.
func TestAlterWorkflow_InsertTimerBoundaryEventCaption(t *testing.T) {
	prog := mustBuild(t, `alter workflow M.WF { insert into Review { boundary event interrupting timer addDays([%CurrentDateTime%], 3) caption 'Escalate' { } } };`)
	op, ok := prog.Statements[0].(*ast.AlterWorkflowStmt).Operations[0].(*ast.InsertBoundaryEventOp)
	if !ok {
		t.Fatalf("got %T, want *ast.InsertBoundaryEventOp", prog.Statements[0].(*ast.AlterWorkflowStmt).Operations[0])
	}
	if op.Caption != "Escalate" {
		t.Errorf("Caption = %q, want Escalate", op.Caption)
	}
}
