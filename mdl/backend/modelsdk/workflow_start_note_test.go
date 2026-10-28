// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// roundTripWorkflow writes wf as the unit stores it and reads it back.
func roundTripWorkflow(t *testing.T, wf *workflows.Workflow) (*genWf.Workflow, *workflows.Workflow) {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	g := el.(*genWf.Workflow)
	return g, workflowFromGen(g, "")
}

// ako/mxcli#1717: the note on a workflow's start is the start activity's —
// Studio Pro 11.15's workflow editor draws the start's note from
// StartWorkflowActivity.Annotation and the workflow's own Annotation nowhere —
// so it is written there and read back from there.
func TestWorkflowStartNote_IsTheStartActivitys(t *testing.T) {
	start := &workflows.StartWorkflowActivity{}
	start.Name, start.Caption, start.Annotation = "Start", "Start", "Started from the request form"
	wf := &workflows.Workflow{Name: "WF", Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{start, &workflows.EndWorkflowActivity{}}}}

	g, back := roundTripWorkflow(t, wf)
	if g.Annotation() != nil {
		t.Errorf("the workflow's own Annotation was written: %v", g.Annotation())
	}
	if got := back.StartNote(); got != "Started from the request form" {
		t.Errorf("StartNote() = %q, want the start activity's note", got)
	}
	if back.Annotation != "" {
		t.Errorf("read the start's note into the workflow's own slot: %q", back.Annotation)
	}
}

// A workflow mxcli wrote before #1717 keeps its note on the workflow; it still
// reads as the start's note until a rewrite moves it.
func TestWorkflowStartNote_FallsBackToTheWorkflows(t *testing.T) {
	start := &workflows.StartWorkflowActivity{}
	start.Name, start.Caption = "Start", "Start"
	wf := &workflows.Workflow{Name: "WF", Annotation: "Written before #1717",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{start, &workflows.EndWorkflowActivity{}}}}

	_, back := roundTripWorkflow(t, wf)
	if got := back.StartNote(); got != "Written before #1717" {
		t.Errorf("StartNote() = %q, want the workflow's own note", got)
	}
}
