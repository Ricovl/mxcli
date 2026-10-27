// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#1714: a workflow text's parameters are written as
// Microflows$TemplateParameter children and read back in order — the display
// and description, and every user task's task name and description.
func TestWorkflowTemplates_RoundTrip(t *testing.T) {
	single := &workflows.UserTask{Page: "M.P",
		TaskName: "Review {1}", TaskNameParams: []string{"$WorkflowContext/Title"},
		TaskDescription: "{1} by {2}", TaskDescriptionParams: []string{"$WorkflowContext/Title", "$WorkflowInstance/Name"}}
	single.Name, single.Caption = "Review", "Review"
	single.Outcomes = []*workflows.UserTaskOutcome{{Value: "Done"}}
	multi := &workflows.UserTask{IsMulti: true, Page: "M.P",
		TaskName: "Vote on {1}", TaskNameParams: []string{"$WorkflowContext/Title"}}
	multi.Name, multi.Caption = "Board", "Board"
	multi.Outcomes = []*workflows.UserTaskOutcome{{Value: "Yes"}, {Value: "No"}}
	plain := &workflows.UserTask{Page: "M.P"} // control: no parameters
	plain.Name, plain.Caption = "Plain", "Plain"
	plain.Outcomes = []*workflows.UserTaskOutcome{{Value: "Done"}}

	wf := &workflows.Workflow{
		Name:                "WF",
		Parameter:           &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		WorkflowName:        "Approval for {1}",
		WorkflowNameParams:  []string{"$WorkflowContext/Title"},
		WorkflowDescription: "Plain",
		Flow:                &workflows.Flow{Activities: []workflows.WorkflowActivity{single, multi, plain}},
	}
	raw, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	back := workflowFromGen(el.(*genWf.Workflow), "")

	check := func(what string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", what, got, want)
		}
	}
	check("display params", back.WorkflowNameParams, []string{"$WorkflowContext/Title"})
	check("description params", back.WorkflowDescriptionParams, []string(nil))
	acts := back.Flow.Activities
	var tasks []*workflows.UserTask
	for _, a := range acts {
		if ut, ok := a.(*workflows.UserTask); ok {
			tasks = append(tasks, ut)
		}
	}
	if len(tasks) != 3 {
		t.Fatalf("read %d user tasks, want 3", len(tasks))
	}
	check("task name", tasks[0].TaskName, "Review {1}")
	check("task name params", tasks[0].TaskNameParams, []string{"$WorkflowContext/Title"})
	check("task description params", tasks[0].TaskDescriptionParams, []string{"$WorkflowContext/Title", "$WorkflowInstance/Name"})
	check("multi task name params", tasks[1].TaskNameParams, []string{"$WorkflowContext/Title"})
	check("plain task name", tasks[2].TaskName, "Plain") // the caption, as the writer falls back to
	check("plain task name params", tasks[2].TaskNameParams, []string(nil))
}
