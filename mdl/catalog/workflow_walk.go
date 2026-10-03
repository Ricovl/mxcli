// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

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

	wfnames.WalkActivities(wf, func(act workflows.WorkflowActivity) {
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
