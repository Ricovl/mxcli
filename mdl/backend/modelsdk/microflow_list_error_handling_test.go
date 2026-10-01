// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#591. The writer stamped a literal "Rollback" on the list
// activities and cast, which is CE6035 in a nanoflow, whose activities store
// "Abort". The builder now supplies the flow flavour's default; this pins that
// the writer emits it and the reader returns it, and that an action with none
// keeps the historical "Rollback".
func TestListActions_ErrorHandlingTypeRoundTrips(t *testing.T) {
	actions := func(eh microflows.ErrorHandlingType) []microflows.MicroflowAction {
		return []microflows.MicroflowAction{
			&microflows.CreateListAction{OutputVariable: "L", EntityQualifiedName: "M.E", ErrorHandlingType: eh},
			&microflows.ChangeListAction{ChangeVariable: "L", Type: microflows.ChangeListTypeAdd, Value: "$P", ErrorHandlingType: eh},
			&microflows.ListOperationAction{OutputVariable: "H", Operation: &microflows.HeadOperation{ListVariable: "L"}, ErrorHandlingType: eh},
			&microflows.AggregateListAction{InputVariable: "L", OutputVariable: "N", Function: microflows.AggregateFunctionCount, ErrorHandlingType: eh},
			&microflows.CastAction{OutputVariable: "S", ErrorHandlingType: eh},
		}
	}
	for _, tc := range []struct {
		set, want microflows.ErrorHandlingType
	}{
		{microflows.ErrorHandlingTypeAbort, microflows.ErrorHandlingTypeAbort},
		{"", microflows.ErrorHandlingTypeRollback},
	} {
		oc := &microflows.MicroflowObjectCollection{}
		for i, a := range actions(tc.set) {
			setActionID(a, model.ID(fmt.Sprintf("a-%d", i)))
			act := &microflows.ActionActivity{Action: a}
			act.ID = model.ID(fmt.Sprintf("act-%d", i))
			act.Position = model.Point{X: 100 * i, Y: 100}
			oc.Objects = append(oc.Objects, act)
		}
		mf := &microflows.Microflow{Name: "MF", ObjectCollection: oc}
		mf.ID = "mf-1"
		got := roundTripMicroflow(t, mf)
		n := 0
		for _, obj := range got.ObjectCollection.Objects {
			aa, ok := obj.(*microflows.ActionActivity)
			if !ok || aa.Action == nil {
				continue
			}
			var eh microflows.ErrorHandlingType
			switch a := aa.Action.(type) {
			case *microflows.CreateListAction:
				eh = a.ErrorHandlingType
			case *microflows.ChangeListAction:
				eh = a.ErrorHandlingType
			case *microflows.ListOperationAction:
				eh = a.ErrorHandlingType
			case *microflows.AggregateListAction:
				eh = a.ErrorHandlingType
			case *microflows.CastAction:
				eh = a.ErrorHandlingType
			default:
				continue
			}
			n++
			if eh != tc.want {
				t.Errorf("set %q: %T read back %q, want %q", tc.set, aa.Action, eh, tc.want)
			}
		}
		if n != 5 {
			t.Fatalf("set %q: %d actions survived the round trip, want 5", tc.set, n)
		}
	}
}

func setActionID(a microflows.MicroflowAction, id model.ID) {
	switch x := a.(type) {
	case *microflows.CreateListAction:
		x.ID = id
	case *microflows.ChangeListAction:
		x.ID = id
	case *microflows.ListOperationAction:
		x.ID = id
	case *microflows.AggregateListAction:
		x.ID = id
	case *microflows.CastAction:
		x.ID = id
	}
}

// mendixlabs/mxcli#870: the writer omitted the workflow selection whenever the
// "Pause instances" / "Unpause instances" flag was set, reading the flag as
// "all workflows". Studio Pro stores the flag WITH the selection
// (WorkflowCommons.ACT_WorkflowDefinition_Lock in ako/TestApp), and without
// one the activity is CE1825.
func TestLockWorkflow_FlagKeepsTheSelection(t *testing.T) {
	lock := &microflows.LockWorkflowAction{PauseAllWorkflows: true, WorkflowVariable: "WorkflowDefinition"}
	lock.ID = "l-1"
	unlock := &microflows.UnlockWorkflowAction{ResumeAllPausedWorkflows: true, Workflow: "M.Approve"}
	unlock.ID = "u-1"
	oc := &microflows.MicroflowObjectCollection{}
	for i, a := range []microflows.MicroflowAction{lock, unlock} {
		act := &microflows.ActionActivity{Action: a}
		act.ID = model.ID(fmt.Sprintf("act-%d", i))
		act.Position = model.Point{X: 100 * i, Y: 100}
		oc.Objects = append(oc.Objects, act)
	}
	mf := &microflows.Microflow{Name: "MF", ObjectCollection: oc}
	mf.ID = "mf-1"
	got := roundTripMicroflow(t, mf)
	n := 0
	for _, obj := range got.ObjectCollection.Objects {
		aa, _ := obj.(*microflows.ActionActivity)
		if aa == nil {
			continue
		}
		switch a := aa.Action.(type) {
		case *microflows.LockWorkflowAction:
			n++
			if !a.PauseAllWorkflows || a.WorkflowVariable != "WorkflowDefinition" {
				t.Errorf("lock read back flag=%v var=%q, want true/WorkflowDefinition", a.PauseAllWorkflows, a.WorkflowVariable)
			}
		case *microflows.UnlockWorkflowAction:
			n++
			if !a.ResumeAllPausedWorkflows || a.Workflow != "M.Approve" {
				t.Errorf("unlock read back flag=%v workflow=%q, want true/M.Approve", a.ResumeAllPausedWorkflows, a.Workflow)
			}
		}
	}
	if n != 2 {
		t.Fatalf("%d actions survived, want 2", n)
	}
}
