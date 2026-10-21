// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1532: `call workflow` takes the same boundary-event clause as
// `call microflow` and `wait for notification`. It was a parse error
// ("mismatched input 'boundary' expecting ';'").
func TestWorkflowVisitor_CallWorkflowBoundaryEvents(t *testing.T) {
	input := `create workflow Tmp.WF_Parent
  parameter $WorkflowContext: Tmp.Item
begin
  call workflow Tmp.WF_Child(WorkflowContext = $WorkflowContext) as callChild
    boundary event interrupting timer addDays([%CurrentDateTime%], 1) { end workflow; }
    non interrupting timer addHours([%CurrentDateTime%], 4) { call microflow Tmp.ACT_Remind; };
end workflow;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateWorkflowStmt)
	n, ok := stmt.Activities[0].(*ast.WorkflowCallWorkflowNode)
	if !ok {
		t.Fatalf("activity is %T, want *ast.WorkflowCallWorkflowNode", stmt.Activities[0])
	}
	if n.Name != "callChild" || n.Workflow.String() != "Tmp.WF_Child" || len(n.ParameterMappings) != 1 {
		t.Errorf("call = %q %s %d mappings, want callChild Tmp.WF_Child 1", n.Name, n.Workflow, len(n.ParameterMappings))
	}
	if len(n.BoundaryEvents) != 2 {
		t.Fatalf("boundary events = %d, want 2", len(n.BoundaryEvents))
	}
	if be := n.BoundaryEvents[0]; be.EventType != "InterruptingTimer" || be.Delay != "addDays([%CurrentDateTime%], 1)" || len(be.Activities) != 1 {
		t.Errorf("event 0 = %+v", be)
	}
	if be := n.BoundaryEvents[1]; be.EventType != "NonInterruptingTimer" || len(be.Activities) != 1 {
		t.Errorf("event 1 = %+v", be)
	}
	if _, ok := n.BoundaryEvents[1].Activities[0].(*ast.WorkflowCallMicroflowNode); !ok {
		t.Errorf("event 1 body = %T, want call microflow", n.BoundaryEvents[1].Activities[0])
	}
}

// The deprecated `with (…)` mapping form still parses with a boundary clause
// after it, as it does on call microflow.
func TestWorkflowVisitor_CallWorkflowBoundaryAfterWith(t *testing.T) {
	input := `create workflow Tmp.WF_Parent
begin
  call workflow Tmp.WF_Child with (WorkflowContext = '$WorkflowContext')
    boundary event non interrupting timer '${PT1H}';
end workflow;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	n := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowCallWorkflowNode)
	if len(n.BoundaryEvents) != 1 || len(n.ParameterMappings) != 1 {
		t.Errorf("boundary events = %d, mappings = %d, want 1 and 1", len(n.BoundaryEvents), len(n.ParameterMappings))
	}
}
