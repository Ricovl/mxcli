// SPDX-License-Identifier: Apache-2.0

package workflows

import (
	"testing"
)

// An interrupting boundary path that runs to its last activity was ended with
// the end-of-path marker, the terminator Studio Pro writes for a
// NON-interrupting path; Studio Pro ends an interrupting one with an End
// (ako/mxcli#1712). Both build, and on the 11.13.0 runtime both complete the
// instance without running what follows the task, so this is the shape Studio
// Pro shows, not a runtime change.

// boundaryPath is a boundary event whose path is a call followed by the marker,
// as buildBoundaryEvents leaves every path that does not end on its own.
func boundaryPath(eventType string, nested ...*BoundaryEvent) *BoundaryEvent {
	task := &UserTask{BoundaryEvents: nested}
	return &BoundaryEvent{EventType: eventType, Flow: &Flow{Activities: []WorkflowActivity{
		&CallMicroflowTask{}, task, &EndOfBoundaryEventPathActivity{},
	}}}
}

func lastOf(t *testing.T, be *BoundaryEvent) WorkflowActivity {
	t.Helper()
	if be.Flow == nil || len(be.Flow.Activities) == 0 {
		t.Fatal("boundary path is empty")
	}
	return be.Flow.Activities[len(be.Flow.Activities)-1]
}

func TestEndInterruptingBoundaryPaths_EndsAnInterruptingPathWithAnEnd(t *testing.T) {
	for _, kind := range []string{"InterruptingTimer", "InterruptingNotification"} {
		be := boundaryPath(kind)
		EndInterruptingBoundaryPaths([]WorkflowActivity{&UserTask{BoundaryEvents: []*BoundaryEvent{be}}}, true, counterIDs())
		end, ok := lastOf(t, be).(*EndWorkflowActivity)
		if !ok {
			t.Fatalf("%s: path ends with %T, want *EndWorkflowActivity", kind, lastOf(t, be))
		}
		if end.ID == "" || end.Name != "End" || end.Caption != "End" {
			t.Errorf("%s: the End is %+v, want a fresh ID named and captioned End", kind, end.BaseWorkflowActivity)
		}
		if n := len(be.Flow.Activities); n != 3 {
			t.Errorf("%s: %d activities, want the marker REPLACED, not followed", kind, n)
		}
	}
}

// CONTROL: the marker stays wherever an End is not Studio Pro's terminator or
// cannot stand — a non-interrupting path (an End there is CE1844), and
// anything under a parallel split or a non-interrupting path (CE1844 at any
// depth), where Studio Pro would end it with a jump only the author can aim.
func TestEndInterruptingBoundaryPaths_KeepsTheMarkerWhereAnEndCannotStand(t *testing.T) {
	ids := counterIDs()

	nonInt := boundaryPath("NonInterruptingTimer")
	EndInterruptingBoundaryPaths([]WorkflowActivity{&CallMicroflowTask{BoundaryEvents: []*BoundaryEvent{nonInt}}}, true, ids)
	if _, ok := lastOf(t, nonInt).(*EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("non-interrupting path ends with %T, want the marker", lastOf(t, nonInt))
	}

	inSplit := boundaryPath("InterruptingTimer")
	split := &ParallelSplitActivity{Outcomes: []*ParallelSplitOutcome{{Flow: &Flow{Activities: []WorkflowActivity{
		&UserTask{BoundaryEvents: []*BoundaryEvent{inSplit}},
	}}}}}
	EndInterruptingBoundaryPaths([]WorkflowActivity{split}, true, ids)
	if _, ok := lastOf(t, inSplit).(*EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("interrupting path under a parallel split ends with %T, want the marker", lastOf(t, inSplit))
	}

	underNonInt := boundaryPath("InterruptingTimer")
	outer := boundaryPath("NonInterruptingTimer", underNonInt)
	EndInterruptingBoundaryPaths([]WorkflowActivity{&UserTask{BoundaryEvents: []*BoundaryEvent{outer}}}, true, ids)
	if _, ok := lastOf(t, underNonInt).(*EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("interrupting path under a non-interrupting one ends with %T, want the marker", lastOf(t, underNonInt))
	}

	// The caller says the place itself cannot hold an End.
	notAllowed := boundaryPath("InterruptingTimer")
	EndInterruptingBoundaryPaths([]WorkflowActivity{&UserTask{BoundaryEvents: []*BoundaryEvent{notAllowed}}}, false, ids)
	if _, ok := lastOf(t, notAllowed).(*EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("with endAllowed false the path ends with %T, want the marker", lastOf(t, notAllowed))
	}
}

// An End is reachable through outcomes and through interrupting paths, at any
// depth (measured clean: a decision outcome, a user-task outcome).
func TestEndInterruptingBoundaryPaths_ReachesNestedPaths(t *testing.T) {
	inner := boundaryPath("InterruptingTimer")
	outer := boundaryPath("InterruptingTimer", inner)
	viaOutcome := boundaryPath("InterruptingTimer")
	decision := &ExclusiveSplitActivity{Outcomes: []ConditionOutcome{&BooleanConditionOutcome{
		Value: true, Flow: &Flow{Activities: []WorkflowActivity{&UserTask{BoundaryEvents: []*BoundaryEvent{viaOutcome}}}},
	}}}
	task := &UserTask{BoundaryEvents: []*BoundaryEvent{outer}}
	EndInterruptingBoundaryPaths([]WorkflowActivity{task, decision}, true, counterIDs())
	for name, be := range map[string]*BoundaryEvent{"outer": outer, "nested": inner, "in a decision outcome": viaOutcome} {
		if _, ok := lastOf(t, be).(*EndWorkflowActivity); !ok {
			t.Errorf("%s path ends with %T, want *EndWorkflowActivity", name, lastOf(t, be))
		}
	}
}

// A path that already ends — in a jump, or an End the author wrote — is left
// alone, and a bare `timer` (no kind, Mendix 10) keeps the marker.
func TestEndInterruptingBoundaryPaths_LeavesEndedAndUnkindedPaths(t *testing.T) {
	jump := &BoundaryEvent{EventType: "InterruptingTimer", Flow: &Flow{Activities: []WorkflowActivity{&JumpToActivity{}}}}
	bare := boundaryPath("Timer")
	EndInterruptingBoundaryPaths([]WorkflowActivity{&UserTask{BoundaryEvents: []*BoundaryEvent{jump, bare}}}, true, counterIDs())
	if len(jump.Flow.Activities) != 1 {
		t.Errorf("a path ending in a jump got %d activities, want it untouched", len(jump.Flow.Activities))
	}
	if _, ok := lastOf(t, bare).(*EndOfBoundaryEventPathActivity); !ok {
		t.Errorf("a bare timer's path ends with %T, want the marker", lastOf(t, bare))
	}
}

func TestEndInterruptingPath(t *testing.T) {
	ids := counterIDs()
	got := EndInterruptingPath([]WorkflowActivity{&CallMicroflowTask{}}, ids)
	if _, ok := got[len(got)-1].(*EndWorkflowActivity); !ok || len(got) != 2 {
		t.Errorf("a path running to its end got %d activities ending %T, want an End appended", len(got), got[len(got)-1])
	}
	if got := EndInterruptingPath(nil, ids); len(got) != 1 {
		t.Errorf("an empty path got %d activities, want just the End", len(got))
	}
	for _, last := range []WorkflowActivity{&JumpToActivity{}, &EndWorkflowActivity{}} {
		if got := EndInterruptingPath([]WorkflowActivity{&CallMicroflowTask{}, last}, ids); len(got) != 2 {
			t.Errorf("a path ending in %T got %d activities, want it untouched", last, len(got))
		}
	}
}
