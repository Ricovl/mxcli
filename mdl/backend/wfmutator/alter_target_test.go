// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
)

// The workflow's half of the generic ALTER (ADR-0012, ako/mxcli#712): an
// activity is addressed by name or caption, @n picks one of several, a name
// wins over a caption repeating it, and an ambiguity lists every match.
func TestResolveAlterTarget_Workflow(t *testing.T) {
	doc := makeWorkflowDoc(
		makeWfActivity("Workflows$SingleUserTaskActivity", "Review", "task1"),
		makeWfActivity("Workflows$ExclusiveSplitActivity", "Review", "decision1"),
		makeWfActivity("Workflows$SingleUserTaskActivity", "Split the jump", "bugSplitJump"),
		makeWfActivity("Workflows$JumpToActivity", "bugSplitJump", "jump1"),
	)
	m := newMutator(doc)
	for _, tc := range []struct {
		name   string
		target backend.AlterTarget
		want   backend.AlterTargetMatch
		errHas string
	}{
		{"by name", backend.AlterTarget{Path: []string{"task1"}}, backend.AlterTargetMatch{Kind: "user task", Name: "task1"}, ""},
		{"by caption", backend.AlterTarget{Caption: "Split the jump"}, backend.AlterTargetMatch{Kind: "user task", Name: "bugSplitJump"}, ""},
		{"name beats a caption repeating it", backend.AlterTarget{Path: []string{"bugSplitJump"}},
			backend.AlterTargetMatch{Kind: "user task", Name: "bugSplitJump"}, ""},
		{"@n counts every match", backend.AlterTarget{Path: []string{"bugSplitJump"}, Ordinal: 2},
			backend.AlterTargetMatch{Kind: "jump", Name: "jump1"}, ""},
		{"@2 of a shared caption", backend.AlterTarget{Caption: "Review", Ordinal: 2},
			backend.AlterTargetMatch{Kind: "decision", Name: "decision1"}, ""},
		{"ambiguous caption lists the matches", backend.AlterTarget{Caption: "Review"}, backend.AlterTargetMatch{},
			"@1 user task task1, @2 decision decision1"},
		{"miss", backend.AlterTarget{Path: []string{"nope"}}, backend.AlterTargetMatch{}, "not found"},
		{"@n past the matches", backend.AlterTarget{Caption: "Review", Ordinal: 3}, backend.AlterTargetMatch{}, "only 2 matches"},
		{"a member path is not an activity", backend.AlterTarget{Path: []string{"task1", "Caption"}}, backend.AlterTargetMatch{},
			"addressed by its name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := m.ResolveAlterTarget(tc.target)
			if tc.errHas != "" {
				var te *backend.AlterTargetError
				if err == nil || !errors.As(err, &te) || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want an AlterTargetError containing %q", err, tc.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// `insert before X { a; b; }` puts the activities, in order, right before X in
// X's own flow.
func TestInsertBeforeActivity(t *testing.T) {
	m := newMutator(makeWorkflowDoc(
		makeWfActivity("Workflows$SingleUserTaskActivity", "A", "a"),
		makeWfActivity("Workflows$SingleUserTaskActivity", "B", "b"),
	))
	newAct := func(name string) workflows.WorkflowActivity {
		u := &workflows.UserTask{}
		u.Name, u.Caption = name, name
		return u
	}
	if err := m.InsertBeforeActivity("b", 0, []workflows.WorkflowActivity{newAct("x"), newAct("y")}); err != nil {
		t.Fatal(err)
	}
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	var names []string
	for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities")) {
		names = append(names, bsonnav.DGetString(e.(bson.D), "Name"))
	}
	if want := []string{"a", "x", "y", "b"}; !reflect.DeepEqual(names, want) {
		t.Errorf("flow = %v, want %v", names, want)
	}
}
