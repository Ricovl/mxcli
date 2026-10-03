// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A workflow names microflows and pages in more places than its activities'
// call slots: workflow event handlers, a user task's on-created microflow, a
// multi-user task's completion microflow, a group-targeting microflow, and every
// activity inside a boundary-event path or an event sub-process. The refs walk
// covered only the main flow's call slots and MicroflowBasedUserSource, so a
// microflow used only from one of the others had no inbound edge: listed by
// graph_dead_assets, "(no callers found)" from `show callers` (TestApp:
// workflow.WorkflowEventHandle, workflow.UserTaskEventHandle). Related:
// mendixlabs/mxcli#1269 (workflows row).

func wfMicroflowTask(mf string) *workflows.CallMicroflowTask {
	return &workflows.CallMicroflowTask{Microflow: mf}
}

// everySlotWorkflow is a workflow with one distinct target per slot, so a
// missing edge names the slot it came from.
func everySlotWorkflow() *workflows.Workflow {
	return &workflows.Workflow{
		Name:         "WF",
		OverviewPage: "M.OverviewPage",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "M.Context"},
		EventHandlers: []*workflows.WorkflowEventHandler{
			{Microflow: "M.WorkflowEventHandler"},
		},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.UserTask{
				Page:       "M.TaskPage",
				OnCreated:  "M.OnCreated",
				UserSource: &workflows.MicroflowBasedUserSource{Microflow: "M.TargetUsers"},
				BoundaryEvents: []*workflows.BoundaryEvent{{
					EventType: "NonInterruptingTimer",
					Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{wfMicroflowTask("M.InUserTaskBoundary")}},
				}},
			},
			&workflows.UserTask{
				IsMulti:            true,
				UserSource:         &workflows.MicroflowGroupSource{Microflow: "M.TargetGroups"},
				CompletionCriteria: &workflows.CompletionCriteria{Kind: "Microflow", Microflow: "M.Completion"},
			},
			&workflows.CallMicroflowTask{
				Microflow: "M.MainCall",
				BoundaryEvents: []*workflows.BoundaryEvent{{
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.SystemTask{Microflow: "M.InCallBoundary"}}},
				}},
			},
			&workflows.CallWorkflowActivity{
				Workflow: "M.SubWorkflow",
				BoundaryEvents: []*workflows.BoundaryEvent{{
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{wfMicroflowTask("M.InCallWorkflowBoundary")}},
				}},
			},
			&workflows.WaitForNotificationActivity{
				BoundaryEvents: []*workflows.BoundaryEvent{{
					Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{wfMicroflowTask("M.InWaitBoundary")}},
				}},
			},
		}},
		EventSubProcesses: []*workflows.EventSubProcess{{
			Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
				&workflows.EventSubProcessStartActivity{Interrupting: true},
				wfMicroflowTask("M.InEventSubProcess"),
				&workflows.UserTask{Page: "M.SubProcessTaskPage"},
			}},
		}},
	}
}

func TestWorkflowDocRefs_ReachEverySlot(t *testing.T) {
	got := map[workflowDocRef]bool{}
	for _, r := range workflowDocRefs(everySlotWorkflow()) {
		got[r] = true
	}
	want := []workflowDocRef{
		// Edges the walk already emitted — kinds unchanged.
		{RefObjectEntity, "M.Context", RefKindParameter},
		{RefObjectPage, "M.OverviewPage", RefKindShowPage},
		{RefObjectPage, "M.TaskPage", RefKindShowPage},
		{RefObjectMicroflow, "M.TargetUsers", RefKindCall},
		{RefObjectMicroflow, "M.MainCall", RefKindCall},
		{RefObjectWorkflow, "M.SubWorkflow", RefKindCall},
		// Edges the walk missed.
		{RefObjectMicroflow, "M.WorkflowEventHandler", RefKindCall},
		{RefObjectMicroflow, "M.OnCreated", RefKindCall},
		{RefObjectMicroflow, "M.TargetGroups", RefKindCall},
		{RefObjectMicroflow, "M.Completion", RefKindCall},
		{RefObjectMicroflow, "M.InUserTaskBoundary", RefKindCall},
		{RefObjectMicroflow, "M.InCallBoundary", RefKindCall},
		{RefObjectMicroflow, "M.InCallWorkflowBoundary", RefKindCall},
		{RefObjectMicroflow, "M.InWaitBoundary", RefKindCall},
		{RefObjectMicroflow, "M.InEventSubProcess", RefKindCall},
		{RefObjectPage, "M.SubProcessTaskPage", RefKindShowPage},
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing workflow ref %+v", w)
		}
	}
}

// The activity counts in workflows_data walk the same tree: activities inside a
// boundary-event path or an event sub-process are activities of the workflow.
func TestCountWorkflowActivityTypes_BoundaryAndEventSubProcess(t *testing.T) {
	total, ut, mf, dec := countWorkflowActivityTypes(everySlotWorkflow())
	// main: 2 user tasks, call microflow, call workflow, wait = 5
	// boundary flows: call, system, call, call = 4
	// event sub-process: start, call microflow, user task = 3
	if total != 12 || ut != 3 || mf != 6 || dec != 0 {
		t.Errorf("counts = (total %d, userTasks %d, microflowCalls %d, decisions %d), want (12, 3, 6, 0)", total, ut, mf, dec)
	}
}

// copyTestApp copies the TestApp submodule project so the build never touches
// the checked-in fixture; it skips when the submodule is not initialised.
func copyTestApp(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}
	dst := t.TempDir()
	if err := os.CopyFS(filepath.Join(dst, "mprcontents"), os.DirFS(filepath.Join(src, "mprcontents"))); err != nil {
		t.Fatalf("copy mprcontents: %v", err)
	}
	mpr, err := os.ReadFile(filepath.Join(src, "TestApp.mpr"))
	if err != nil {
		t.Fatalf("read TestApp.mpr: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "TestApp.mpr"), mpr, 0o644); err != nil {
		t.Fatalf("write TestApp.mpr: %v", err)
	}
	return filepath.Join(dst, "TestApp.mpr")
}

// fullTestAppCatalog runs the real full catalog build over a TestApp copy.
func fullTestAppCatalog(t *testing.T) *Catalog {
	t.Helper()
	be := modelsdkbackend.New()
	if err := be.Connect(copyTestApp(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { be.Disconnect() })
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}
	return cat
}

// On TestApp, workflow.WorkflowEventHandle is the workflows' OnAnyEvent handler
// and workflow.UserTaskEventHandle a user task's on-created microflow. Neither
// is called from anywhere else, so before the fix both were listed as dead.
// Microflows.SplitMerge is genuinely unused and is the control: it must stay
// dead, or the assertion would pass against a view that lists nothing.
func TestTestApp_WorkflowHandlersAreNotDead(t *testing.T) {
	cat := fullTestAppCatalog(t)
	dead := map[string]bool{}
	for _, row := range queryRows(t, cat, `SELECT QualifiedName FROM graph_dead_assets WHERE ObjectType = 'MICROFLOW'`) {
		dead[row[0].(string)] = true
	}
	if !dead["Microflows.SplitMerge"] {
		t.Fatalf("control: Microflows.SplitMerge is unused and must be listed dead; dead set = %v", dead)
	}
	for _, mf := range []string{"workflow.WorkflowEventHandle", "workflow.UserTaskEventHandle"} {
		if dead[mf] {
			t.Errorf("%s is used by a workflow but graph_dead_assets lists it as dead", mf)
		}
	}
}
