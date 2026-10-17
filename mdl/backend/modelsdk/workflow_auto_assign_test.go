// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #1445: userTaskToGen wrote AutoAssignSingleTargetUser = false on every
// user task whatever the model said, and the reader never read it back. Both
// task kinds, both values, survive a write and a fresh read — checked on the
// stored key as well as through the reader, so the reader cannot mask the writer.
func TestCreateWorkflow_AutoAssignSingleTargetUserRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	want := map[string]bool{"singleOn": true, "singleOff": false, "multiOn": true, "multiOff": false}
	acts := []workflows.WorkflowActivity{&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Start"}}}
	for _, name := range []string{"singleOn", "singleOff", "multiOn", "multiOff"} {
		acts = append(acts, &workflows.UserTask{
			BaseWorkflowActivity:       workflows.BaseWorkflowActivity{Name: name, Caption: name},
			IsMulti:                    name == "multiOn" || name == "multiOff",
			Outcomes:                   []*workflows.UserTaskOutcome{{Value: "Approve"}},
			AutoAssignSingleTargetUser: want[name],
		})
	}
	acts = append(acts, &workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End"}})
	wf := &workflows.Workflow{ContainerID: mod.ID, Name: "ZzAutoAssign", Parameter: &workflows.WorkflowParameter{EntityRef: "MyFirstModule.Ctx"}, Flow: &workflows.Flow{Activities: acts}}
	if err := b.CreateWorkflow(wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	var got *workflows.Workflow
	for _, w := range all {
		if w.Name == "ZzAutoAssign" {
			got = w
		}
	}
	if got == nil {
		t.Fatal("workflow not found after reconnect")
	}

	raw, err := b2.GetRawUnit(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored := map[string]any{}
	for _, a := range raw["Flow"].(map[string]any)["Activities"].([]any) {
		if m, ok := a.(map[string]any); ok {
			if n, _ := m["Name"].(string); n != "" {
				stored[n] = m["AutoAssignSingleTargetUser"]
			}
		}
	}
	read := map[string]bool{}
	for _, a := range got.Flow.Activities {
		if ut, ok := a.(*workflows.UserTask); ok {
			read[ut.Name] = ut.AutoAssignSingleTargetUser
		}
	}
	for name, w := range want {
		if stored[name] != w {
			t.Errorf("%s: stored AutoAssignSingleTargetUser = %v, want %v", name, stored[name], w)
		}
		if read[name] != w {
			t.Errorf("%s: read back AutoAssignSingleTargetUser = %v, want %v", name, read[name], w)
		}
	}
}
