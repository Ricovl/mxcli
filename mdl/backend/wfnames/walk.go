// SPDX-License-Identifier: Apache-2.0

package wfnames

import "github.com/mendixlabs/mxcli/sdk/workflows"

// WalkActivities visits every activity of a workflow, depth-first: the main
// flow, every sub-flow an activity carries (outcome flows and boundary-event
// flows, per SubFlows), and every event sub-process.
//
// It is the one traversal every reader of a stored workflow shares — the
// catalog's refs and activity counts, `list workflows` and `show structure`.
// Each used to carry its own recursion over outcomes only, so they disagreed:
// a microflow called from a boundary-event path had no inbound edge
// (mendixlabs/mxcli#1269), and `list workflows` counted 5 activities where the
// catalog counted 8 (ako/mxcli#963). A new place that holds a flow is added to
// SubFlows or here, and every caller sees it.
func WalkActivities(wf *workflows.Workflow, visit func(workflows.WorkflowActivity)) {
	if wf == nil {
		return
	}
	walkFlow(wf.Flow, visit)
	for _, esp := range wf.EventSubProcesses {
		if esp != nil {
			walkFlow(esp.Flow, visit)
		}
	}
}

func walkFlow(flow *workflows.Flow, visit func(workflows.WorkflowActivity)) {
	if flow == nil {
		return
	}
	for _, act := range flow.Activities {
		if act == nil {
			continue
		}
		visit(act)
		for _, sub := range SubFlows(act) {
			walkFlow(sub, visit)
		}
	}
}

// ActivityCounts is the per-type tally of a workflow's activities over every
// flow WalkActivities reaches.
type ActivityCounts struct {
	Total          int
	UserTasks      int
	MicroflowCalls int // call-microflow and system tasks
	Decisions      int
}

// CountActivities tallies a workflow's activities with WalkActivities.
func CountActivities(wf *workflows.Workflow) ActivityCounts {
	var c ActivityCounts
	WalkActivities(wf, func(act workflows.WorkflowActivity) {
		c.Total++
		switch act.(type) {
		case *workflows.UserTask:
			c.UserTasks++
		case *workflows.CallMicroflowTask, *workflows.SystemTask:
			c.MicroflowCalls++
		case *workflows.ExclusiveSplitActivity:
			c.Decisions++
		}
	})
	return c
}
