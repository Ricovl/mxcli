// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// Settings a Studio Pro author can give a workflow that MDL has no words for, so
// a rebuild from a statement resets them without a word (ako/mxcli#1713).
// Measured on mxbuild 11.15.0: a stored workflow given each of them passed mx
// check; describe printed none of them, and replaying the description with
// `create or modify` reported "Modified workflow" and left none of them:
//
//	a user task's task name 'Review {1}' (caption 'Review')   reset to 'Review' (stated since #1714)
//	its parameter {1} = $WorkflowContext/Name                 dropped (stated since #1714)
//	a timer boundary event's caption                          blanked (stated since #1715)
//	a non-interrupting timer's recurrence                     dropped (a one-shot reminder)
//
// The rebuild writes a timer boundary event without a recurrence, and a
// boundary event without its annotation; each is refused until MDL can state it
// — the issue named after the reason adds the syntax. A user task's task name
// and the texts' parameters are stated since ako/mxcli#1714 (`task name '…'`,
// `… with ({1} = …)`), and printed by describe.

// rawBoundaryEventLosses lists what rebuilding a stored activity resets on its
// boundary events: a timer's recurrence, and an annotation. A timer's caption
// is stated with `caption '…'` and printed by describe (ako/mxcli#1715).
func rawBoundaryEventLosses(d map[string]any) []string {
	var out []string
	for _, be := range rawList(d["BoundaryEvents"]) {
		bm, ok := be.(map[string]any)
		if !ok {
			continue
		}
		kind := rawBoundaryEventKind(bm)
		if r, ok := bm["Recurrence"].(map[string]any); ok {
			out = append(out, fmt.Sprintf("its %s boundary event repeats (every %v %v, at most %v times), which MDL cannot state yet (ako/mxcli#1716)",
				kind, r["Interval"], r["IntervalType"], r["MaxExecutions"]))
		}
		if a, ok := bm["Annotation"].(map[string]any); ok {
			text, _ := a["Description"].(string)
			out = append(out, fmt.Sprintf("its %s boundary event has annotation '%s', which MDL cannot state", kind, text))
		}
	}
	return out
}

// rawBoundaryEventKind is a stored boundary event's kind as MDL writes it after
// `boundary event`: "interrupting timer", "non interrupting notification", ….
func rawBoundaryEventKind(bm map[string]any) string {
	t, _ := bm["$Type"].(string)
	t = strings.TrimSuffix(strings.TrimPrefix(t, "Workflows$"), "BoundaryEvent")
	kind := "interrupting"
	if strings.HasPrefix(t, "NonInterrupting") {
		kind = "non interrupting"
		t = strings.TrimPrefix(t, "NonInterrupting")
	} else {
		t = strings.TrimPrefix(t, "Interrupting")
	}
	return kind + " " + strings.ToLower(t)
}

// rawUnstatableWorkflowState lists, for CREATE OR MODIFY WORKFLOW, what the
// stored workflow holds that rebuilding it resets. Boundary-event state counts
// wherever it is, since a boundary event the statement does not restate is
// refused already.
func rawUnstatableWorkflowState(raw map[string]any) []string {
	var out []string
	walkRawDocs(raw, func(d map[string]any) {
		name, _ := d["Name"].(string)
		for _, r := range rawBoundaryEventLosses(d) {
			out = append(out, fmt.Sprintf("activity '%s': %s", name, r))
		}
	})
	return out
}

// rawUnstatableActivityState lists, for REPLACE ACTIVITY, what the stored
// activity holds that rebuilding it from the replacement resets.
func rawUnstatableActivityState(d map[string]any) []string {
	return rawBoundaryEventLosses(d)
}
