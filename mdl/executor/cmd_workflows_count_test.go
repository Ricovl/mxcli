// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// `list workflows` and `show structure` count a workflow's activities with the
// same walk as the catalog (ako/mxcli#963): activities on a boundary-event path
// and in an event sub-process are activities of the workflow.
func boundaryAndESPWorkflow() *workflows.Workflow {
	call := func(mf string) *workflows.CallMicroflowTask { return &workflows.CallMicroflowTask{Microflow: mf} }
	return &workflows.Workflow{
		Name: "WF",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.UserTask{
				BoundaryEvents: []*workflows.BoundaryEvent{{
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{call("M.InBoundary")}},
				}},
			},
			&workflows.ExclusiveSplitActivity{},
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{
			Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				&workflows.EventSubProcessStartActivity{},
				&workflows.UserTask{},
			}},
		}},
	}
}

// outcomeOnlyWorkflow is the control: no boundary events, no event
// sub-process, so the old outcome-only recursion and the shared walk agree.
func outcomeOnlyWorkflow() *workflows.Workflow {
	return &workflows.Workflow{
		Name: "WF",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.UserTask{Outcomes: []*workflows.UserTaskOutcome{{
				Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.CallMicroflowTask{}}},
			}}},
			&workflows.ExclusiveSplitActivity{},
		}},
	}
}

func TestCountWorkflowActivities_BoundaryAndEventSubProcess(t *testing.T) {
	cases := []struct {
		name                   string
		wf                     *workflows.Workflow
		total, ut, calls, decs int
	}{
		// main: user task, decision = 2; boundary: call = 1; ESP: start, user task = 2
		{"boundary+esp", boundaryAndESPWorkflow(), 5, 2, 1, 1},
		{"control: outcomes only", outcomeOnlyWorkflow(), 3, 1, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			total, ut, decs := countWorkflowActivities(c.wf)
			if total != c.total || ut != c.ut || decs != c.decs {
				t.Errorf("list workflows counts = (%d, %d, %d), want (%d, %d, %d)", total, ut, decs, c.total, c.ut, c.decs)
			}
			stotal, sut, scalls, sdecs := countStructureWorkflowActivities(c.wf)
			if stotal != c.total || sut != c.ut || scalls != c.calls || sdecs != c.decs {
				t.Errorf("show structure counts = (%d, %d, %d, %d), want (%d, %d, %d, %d)", stotal, sut, scalls, sdecs, c.total, c.ut, c.calls, c.decs)
			}
		})
	}
}

// On TestApp, Workflow1 has boundary events: the catalog has always counted 8
// activities for it, `list workflows` counted 5. Every other workflow is the
// control — it must count the same either way.
func TestCountWorkflowActivities_TestAppWorkflow1(t *testing.T) {
	exec, _ := openTestAppCopy(t)
	wfs, err := exec.backend.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, wf := range wfs {
		total, _, _ := countWorkflowActivities(wf)
		stotal, _, _, _ := countStructureWorkflowActivities(wf)
		if stotal != total {
			t.Errorf("%s: show structure %d != list workflows %d", wf.Name, stotal, total)
		}
		if wf.Name == "Workflow1" {
			found = true
			if total != 8 {
				t.Errorf("Workflow1 activities = %d, want 8 (the catalog's count)", total)
			}
		}
	}
	if !found {
		t.Fatal("TestApp has no Workflow1")
	}
}
