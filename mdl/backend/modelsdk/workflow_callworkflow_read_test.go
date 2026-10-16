// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1531. The reader converted a CallWorkflowActivity's Workflow,
// ParameterExpression and boundary events but never its ParameterMappings, so
// DESCRIBE printed `call workflow M.WF_Child;` for a call that maps an
// association path. Re-executing that output let autoBindCallWorkflow write the
// default `$WorkflowContext` mapping — silently passing a different object.
// The describe tests feed semantic structs directly, so only a test through the
// writer and the reader covers this layer.
func TestWorkflowRead_CallWorkflowParameterMappingsRoundTrip(t *testing.T) {
	call := &workflows.CallWorkflowActivity{
		Workflow:            "M.WF_Child",
		ParameterExpression: "$WorkflowContext",
	}
	call.Name = "WF_Child"
	call.ParameterMappings = []*workflows.ParameterMapping{{
		Parameter:  "M.WF_Child.WorkflowContext",
		Expression: "$WorkflowContext/M.Parent_Current/M.Item",
	}}
	call.ParameterMappings[0].ID = "11111111-2222-3333-4444-555555555555"

	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallWorkflowActivity)
	if !ok {
		t.Fatalf("round-tripped to %T, want *workflows.CallWorkflowActivity", rt)
	}
	if len(rt.ParameterMappings) != 1 {
		t.Fatalf("parameter mappings after round trip = %d, want 1 (the reader dropped it)", len(rt.ParameterMappings))
	}
	pm := rt.ParameterMappings[0]
	if pm.Parameter != "M.WF_Child.WorkflowContext" {
		t.Errorf("Parameter = %q, want M.WF_Child.WorkflowContext", pm.Parameter)
	}
	if pm.Expression != "$WorkflowContext/M.Parent_Current/M.Item" {
		t.Errorf("Expression = %q, want the association path", pm.Expression)
	}
	if pm.ID != call.ParameterMappings[0].ID {
		t.Errorf("mapping ID = %q, want the stored %q", pm.ID, call.ParameterMappings[0].ID)
	}
}

// Control: a call with no mapping must not gain one on read.
func TestWorkflowRead_CallWorkflowWithoutMappingsStaysEmpty(t *testing.T) {
	call := &workflows.CallWorkflowActivity{Workflow: "M.WF_Child"}
	call.Name = "WF_Child"
	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallWorkflowActivity)
	if !ok {
		t.Fatalf("round-tripped to %T, want *workflows.CallWorkflowActivity", rt)
	}
	if len(rt.ParameterMappings) != 0 {
		t.Errorf("parameter mappings = %d, want 0", len(rt.ParameterMappings))
	}
}
