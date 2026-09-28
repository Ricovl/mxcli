// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#791. An activity's Outcomes list holds a different element type per
// activity kind, and the path ops addressed it without looking at the kind:
// `drop userTask1 path 1` removed the user task's first outcome ('Good') and
// reported "Altered workflow", because "Path N" was matched against the index
// of any Outcomes list. The member ops must refuse an activity whose list does
// not hold what they address — never reinterpret the address against it.

func makeWfOutcome(typeName, value string) bson.D {
	d := bson.D{
		{Key: "$ID", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}},
		{Key: "$Type", Value: typeName},
	}
	if value != "" {
		d = append(d, bson.E{Key: "Value", Value: value})
	}
	return d
}

func makeWfKindActivity(typeName, name string, outcomes ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, o := range outcomes {
		arr = append(arr, o)
	}
	act := makeWfActivity(typeName, name, name)
	return append(act, bson.E{Key: "Outcomes", Value: arr})
}

// memberFixture is one workflow holding a user task with three outcomes (the
// shape of TestApp's userTask1), a multi user task, a decision and a parallel
// split with two paths.
func memberFixture() *Mutator {
	return newMutator(makeWorkflowDoc(
		makeWfKindActivity("Workflows$SingleUserTaskActivity", "userTask1",
			makeWfOutcome("Workflows$UserTaskOutcome", "Good"),
			makeWfOutcome("Workflows$UserTaskOutcome", "Bad"),
			makeWfOutcome("Workflows$UserTaskOutcome", "Ugly")),
		makeWfKindActivity("Workflows$MultiUserTaskActivity", "userTask2",
			makeWfOutcome("Workflows$UserTaskOutcome", "Fast")),
		makeWfKindActivity("Workflows$ExclusiveSplitActivity", "decision1",
			makeWfOutcome("Workflows$VoidConditionOutcome", "")),
		makeWfKindActivity("Workflows$ParallelSplitActivity", "split1",
			makeWfOutcome("Workflows$ParallelSplitOutcome", ""),
			makeWfOutcome("Workflows$ParallelSplitOutcome", "")),
		makeWfActivity("Workflows$WaitForTimerActivity", "timer1", "timer1"),
	))
}

func outcomeValues(t *testing.T, m *Mutator, ref string) []string {
	t.Helper()
	act, err := m.findActivityByCaption(ref, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(act, "Outcomes")) {
		d := e.(bson.D)
		v := bsonnav.DGetString(d, "Value")
		if v == "" {
			v = bsonnav.DGetString(d, "$Type")
		}
		out = append(out, v)
	}
	return out
}

func TestDropPath_OnUserTaskIsRefused(t *testing.T) {
	for _, ref := range []string{"userTask1", "userTask2", "decision1"} {
		m := memberFixture()
		before := strings.Join(outcomeValues(t, m, ref), ",")
		err := m.DropPath(ref, 0, "Path 1")
		if err == nil || !strings.Contains(err.Error(), "not a parallel split") {
			t.Errorf("drop %s path 1: want a refusal naming the activity kind, got %v", ref, err)
		}
		if after := strings.Join(outcomeValues(t, m, ref), ","); after != before {
			t.Errorf("drop %s path 1 changed the outcomes: %s -> %s", ref, before, after)
		}
	}
}

// Control: on a parallel split the same address drops that path.
func TestDropPath_OnParallelSplitDropsThePath(t *testing.T) {
	m := memberFixture()
	if err := m.DropPath("split1", 0, "Path 2"); err != nil {
		t.Fatal(err)
	}
	if got := outcomeValues(t, m, "split1"); len(got) != 1 {
		t.Errorf("want 1 path left, got %v", got)
	}
}

// The old `drop path ” on X` dropped the LAST path: an empty caption names no
// path, so which one went was a guess.
func TestDropPath_EmptyCaptionIsRefused(t *testing.T) {
	m := memberFixture()
	err := m.DropPath("split1", 0, "")
	if err == nil || !strings.Contains(err.Error(), "names no path") {
		t.Errorf("want a refusal of the empty caption, got %v", err)
	}
	if got := outcomeValues(t, m, "split1"); len(got) != 2 {
		t.Errorf("paths changed: %v", got)
	}
}

func TestDropPath_OutOfRangeNamesTheCount(t *testing.T) {
	m := memberFixture()
	err := m.DropPath("split1", 0, "Path 3")
	if err == nil || !strings.Contains(err.Error(), "has 2 paths") {
		t.Errorf("want the path count in the error, got %v", err)
	}
}

// The inserting ops are refused at the mutator too: the executor's check reads
// the STORED workflow, so it cannot see an activity inserted earlier in the
// same statement, and a wrong outcome type leaves a project Mendix cannot load
// (ako/mxcli#415).
func TestInsertMember_OnWrongKindIsRefused(t *testing.T) {
	cases := []struct {
		name string
		op   func(m *Mutator) error
		ref  string
	}{
		{"insert path on a user task", func(m *Mutator) error { return m.InsertPath("userTask1", 0, "", nil) }, "userTask1"},
		{"insert path on a decision", func(m *Mutator) error { return m.InsertPath("decision1", 0, "", nil) }, "decision1"},
		{"insert outcome on a split", func(m *Mutator) error { return m.InsertOutcome("split1", 0, "X", nil) }, "split1"},
		{"insert outcome on a decision", func(m *Mutator) error { return m.InsertOutcome("decision1", 0, "X", nil) }, "decision1"},
		{"insert branch on a user task", func(m *Mutator) error { return m.InsertBranch("userTask1", 0, "true", nil) }, "userTask1"},
		{"insert branch on a split", func(m *Mutator) error { return m.InsertBranch("split1", 0, "true", nil) }, "split1"},
		{"insert boundary event on a split", func(m *Mutator) error {
			return m.InsertBoundaryEvent("split1", 0, "InterruptingTimer", "", nil)
		}, "split1"},
		{"insert boundary event on a timer", func(m *Mutator) error {
			return m.InsertBoundaryEvent("timer1", 0, "InterruptingTimer", "", nil)
		}, "timer1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := memberFixture()
			before, _ := bson.Marshal(m.rawData)
			err := tc.op(m)
			if err == nil || !(strings.Contains(err.Error(), "cannot hold") || strings.Contains(err.Error(), "not a parallel split")) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if after, _ := bson.Marshal(m.rawData); string(after) != string(before) {
				t.Error("the refused op changed the workflow")
			}
		})
	}
}

// Control: each inserting op still lands on the kind that holds it.
func TestInsertMember_OnRightKindLands(t *testing.T) {
	m := memberFixture()
	if err := m.InsertPath("split1", 0, "", nil); err != nil {
		t.Errorf("insert path on a split: %v", err)
	}
	if err := m.InsertOutcome("userTask1", 0, "New", nil); err != nil {
		t.Errorf("insert outcome on a user task: %v", err)
	}
	if err := m.InsertOutcome("userTask2", 0, "New", nil); err != nil {
		t.Errorf("insert outcome on a multi user task: %v", err)
	}
	if err := m.InsertBranch("decision1", 0, "true", nil); err != nil {
		t.Errorf("insert branch on a decision: %v", err)
	}
	if err := m.InsertBoundaryEvent("userTask1", 0, "InterruptingTimer", "", nil); err != nil {
		t.Errorf("insert boundary event on a user task: %v", err)
	}
}
