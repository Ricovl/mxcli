// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#1716: a recurrence is written as a Workflows$LinearRecurrence on a
// non-interrupting timer boundary event and on a non-interrupting timer event
// sub-process start, and read back.
func TestWorkflowRecurrence_RoundTrip(t *testing.T) {
	daily := &workflows.Recurrence{IntervalType: "Day", Interval: 1, MaxExecutions: 5}
	hourly := &workflows.Recurrence{IntervalType: "Hour", Interval: 2, MaxExecutions: 3}
	task := &workflows.UserTask{Page: "M.P", Outcomes: []*workflows.UserTaskOutcome{{Value: "Done"}}}
	task.Name, task.Caption = "Review", "Review"
	task.BoundaryEvents = []*workflows.BoundaryEvent{
		{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 1)", Recurrence: daily},
		{EventType: "NonInterruptingTimer", TimerDelay: "addDays([%CurrentDateTime%], 2)"}, // control: fires once
	}
	start := &workflows.EventSubProcessStartActivity{Timer: true, FirstExecutionTime: "addHours([%CurrentDateTime%], 2)", Recurrence: hourly}
	start.Name, start.Caption = "NudgeStart", "NudgeStart"
	esp := &workflows.EventSubProcess{Name: "Nudge", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{start}}}
	wf := &workflows.Workflow{Name: "WF", Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{task}}, EventSubProcesses: []*workflows.EventSubProcess{esp}}

	raw, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	back := workflowFromGen(el.(*genWf.Workflow), "")
	var rt *workflows.UserTask
	for _, a := range back.Flow.Activities {
		if ut, ok := a.(*workflows.UserTask); ok {
			rt = ut
		}
	}
	if rt == nil || len(rt.BoundaryEvents) != 2 {
		t.Fatalf("read back %+v", rt)
	}
	if r := rt.BoundaryEvents[0].Recurrence; r == nil || *r != *daily {
		t.Errorf("boundary recurrence = %+v, want %+v", r, daily)
	}
	if r := rt.BoundaryEvents[1].Recurrence; r != nil {
		t.Errorf("control fires once, read %+v", r)
	}
	if len(back.EventSubProcesses) != 1 || back.EventSubProcesses[0].Start() == nil {
		t.Fatalf("event sub-process not read back: %+v", back.EventSubProcesses)
	}
	if r := back.EventSubProcesses[0].Start().Recurrence; r == nil || *r != *hourly {
		t.Errorf("event sub-process start recurrence = %+v, want %+v", r, hourly)
	}
}
