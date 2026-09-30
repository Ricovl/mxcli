// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Running a Studio Pro-authored workflow's own describe output as `create or
// modify` rewrote what Studio Pro stores and describe cannot print (#743,
// ako/TestApp workflow.Workflow1 and Workflow1_2, Mendix 11.14.0):
//   - every outcome stores a Flow, empty when the outcome leads nowhere
//     (Workflows$Outcome.flow is Required), and a VoidConditionOutcome stores a
//     PersistentId like every other outcome — both were dropped;
//   - every activity stores RelativeMiddlePoint and Size "0;0" — written as "";
//   - EventSubProcesses is stored empty ([2]) on a workflow that has none —
//     omitted.

func encodeWorkflowFor(t *testing.T, wf *workflows.Workflow, pv *types.ProjectVersion) bson.Raw {
	t.Helper()
	raw, err := encodeWorkflow(wf, pv)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

func v11(minor int) *types.ProjectVersion {
	return &types.ProjectVersion{MajorVersion: 11, MinorVersion: minor}
}

// activityDoc returns the i-th element of a typed array (index 0 is the marker).
func arrayDoc(t *testing.T, v bson.RawValue, i int) bson.Raw {
	t.Helper()
	vals, err := v.Array().Values()
	if err != nil || len(vals) <= i {
		t.Fatalf("array %v has no element %d", v, i)
	}
	return vals[i].Document()
}

func outcomeWorkflow() *workflows.Workflow {
	return &workflows.Workflow{
		Name:      "WF",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "start1", Caption: "Start"}},
			&workflows.CallMicroflowTask{
				BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "aiAgentTask1", Caption: "AI Agent Task"},
				IsAgent:              true,
				Microflow:            "M.InvokeAgent",
				Outcomes:             []workflows.ConditionOutcome{&workflows.VoidConditionOutcome{}},
			},
			&workflows.UserTask{
				BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "userTask1", Caption: "User Task"},
				Page:                 "M.TaskPage",
				Outcomes:             []*workflows.UserTaskOutcome{{Value: "Good"}},
			},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "end1", Caption: "End"}},
		}},
	}
}

func TestWorkflowToGen_EveryOutcomeStoresAFlow(t *testing.T) {
	raw := encodeWorkflowFor(t, outcomeWorkflow(), v11(14))
	acts := raw.Lookup("Flow", "Activities")

	void := arrayDoc(t, arrayDoc(t, acts, 2).Lookup("Outcomes"), 1)
	if _, err := void.LookupErr("Flow", "Activities"); err != nil {
		t.Errorf("a VoidConditionOutcome with no activities stores no Flow: %v", void)
	}
	if _, err := void.LookupErr("PersistentId"); err != nil {
		t.Errorf("a VoidConditionOutcome stores no PersistentId: %v", void)
	}

	user := arrayDoc(t, arrayDoc(t, acts, 3).Lookup("Outcomes"), 1)
	if _, err := user.LookupErr("Flow", "Activities"); err != nil {
		t.Errorf("a UserTaskOutcome with no activities stores no Flow: %v", user)
	}
}

func TestWorkflowToGen_ActivityLayoutIsTheStudioProDefault(t *testing.T) {
	raw := encodeWorkflowFor(t, outcomeWorkflow(), v11(14))
	acts := raw.Lookup("Flow", "Activities")
	for i := 1; i <= 4; i++ {
		act := arrayDoc(t, acts, i)
		for _, key := range []string{"RelativeMiddlePoint", "Size"} {
			if got, _ := act.Lookup(key).StringValueOK(); got != "0;0" {
				t.Errorf("activity %d %s = %q, want \"0;0\" (what Studio Pro stores)", i, key, got)
			}
		}
	}
}

func TestWorkflowEncoder_EmptyEventSubProcessesFromMendix118(t *testing.T) {
	wf := outcomeWorkflow()
	v, err := encodeWorkflowFor(t, wf, v11(14)).LookupErr("EventSubProcesses")
	if err != nil {
		t.Fatal("a workflow with no event sub-processes stores no EventSubProcesses on 11.14; Studio Pro stores [2]")
	}
	vals, _ := v.Array().Values()
	if len(vals) != 1 || vals[0].Int32() != 2 {
		t.Errorf("EventSubProcesses = %v, want [2]", v)
	}
	// Control: the property only exists from 11.8.0, and a key the project's
	// metamodel does not declare makes the document unopenable in Studio Pro.
	for _, pv := range []*types.ProjectVersion{v11(7), nil} {
		if _, err := encodeWorkflowFor(t, wf, pv).LookupErr("EventSubProcesses"); err == nil {
			t.Errorf("EventSubProcesses written for project version %v, which does not declare it", pv)
		}
	}
}
