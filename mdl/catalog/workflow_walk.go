// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// walkWorkflowActivities visits every activity of a workflow, depth-first: the
// main flow, every sub-flow an activity carries (outcome flows and
// boundary-event flows, per wfnames.SubFlows), and every event sub-process.
//
// It is the one traversal the catalog's workflow passes share. The refs walk and
// the activity counts each had their own recursion over outcomes only, so a
// microflow called from a boundary-event path or an event sub-process had no
// inbound edge and was reported dead by graph_dead_assets (mendixlabs/mxcli#1269,
// workflows row). A new place that holds a flow is added to wfnames.SubFlows or
// here, and both passes see it.
func walkWorkflowActivities(wf *workflows.Workflow, visit func(workflows.WorkflowActivity)) {
	if wf == nil {
		return
	}
	walkWorkflowFlow(wf.Flow, visit)
	for _, esp := range wf.EventSubProcesses {
		if esp != nil {
			walkWorkflowFlow(esp.Flow, visit)
		}
	}
}

func walkWorkflowFlow(flow *workflows.Flow, visit func(workflows.WorkflowActivity)) {
	if flow == nil {
		return
	}
	for _, act := range flow.Activities {
		if act == nil {
			continue
		}
		visit(act)
		for _, sub := range wfnames.SubFlows(act) {
			walkWorkflowFlow(sub, visit)
		}
	}
}

// workflowDocRef is one reference a workflow makes to another document.
type workflowDocRef struct {
	TargetType, TargetName, RefKind string
}

// workflowDocRefs returns every document reference a workflow makes, in walk
// order, one per occurrence. Every microflow the runtime runs on the workflow's
// behalf — a call activity, an event handler, an on-created or completion
// microflow, a targeting microflow — is a `call` edge, the kind the activity
// calls always had, so consumers filtering on `call` see them all.
func workflowDocRefs(wf *workflows.Workflow) []workflowDocRef {
	if wf == nil {
		return nil
	}
	var out []workflowDocRef
	add := func(targetType, targetName, refKind string) {
		if targetName != "" {
			out = append(out, workflowDocRef{targetType, targetName, refKind})
		}
	}

	if wf.Parameter != nil {
		add(RefObjectEntity, wf.Parameter.EntityRef, RefKindParameter)
	}
	add(RefObjectPage, wf.OverviewPage, RefKindShowPage)
	for _, h := range wf.EventHandlers {
		if h != nil {
			add(RefObjectMicroflow, h.Microflow, RefKindCall)
		}
	}

	walkWorkflowActivities(wf, func(act workflows.WorkflowActivity) {
		switch a := act.(type) {
		case *workflows.UserTask:
			add(RefObjectPage, a.Page, RefKindShowPage)
			add(RefObjectEntity, a.UserTaskEntity, RefKindDatasource)
			switch us := a.UserSource.(type) {
			case *workflows.MicroflowBasedUserSource:
				add(RefObjectMicroflow, us.Microflow, RefKindCall)
			case *workflows.MicroflowGroupSource:
				add(RefObjectMicroflow, us.Microflow, RefKindCall)
			}
			add(RefObjectMicroflow, a.OnCreated, RefKindCall)
			if a.CompletionCriteria != nil {
				add(RefObjectMicroflow, a.CompletionCriteria.Microflow, RefKindCall)
			}
		case *workflows.CallMicroflowTask:
			add(RefObjectMicroflow, a.Microflow, RefKindCall)
		case *workflows.SystemTask:
			add(RefObjectMicroflow, a.Microflow, RefKindCall)
		case *workflows.CallWorkflowActivity:
			add(RefObjectWorkflow, a.Workflow, RefKindCall)
		}
	})
	return out
}
