// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

// ako/mxcli#791 over --mcp: `drop <userTask> path 1` indexed the user task's
// outcomes and removed the first one. The PED mutator resolves the same
// addresses as wfmutator, so it refuses the same way — before any update.

func TestWFDropPath_OnNonSplitIsRefused(t *testing.T) {
	for _, ref := range []string{"ReviewOrder", "Decision"} {
		f, m := wfMutatorFake(t)
		err := m.DropPath(ref, 0, "Path 1")
		if err == nil || !strings.Contains(err.Error(), "not a parallel split") {
			t.Errorf("drop %s path 1: want a refusal naming the activity kind, got %v", ref, err)
		}
		if _, sent := f.callByName("ped_update_document"); sent {
			t.Errorf("drop %s path 1 sent an update", ref)
		}
	}
}

func TestWFDropPath_EmptyCaptionIsRefused(t *testing.T) {
	f, m := wfMutatorFake(t)
	if err := m.DropPath("Parallel split", 0, ""); err == nil || !strings.Contains(err.Error(), "names no path") {
		t.Errorf("want a refusal of the empty caption, got %v", err)
	}
	if _, sent := f.callByName("ped_update_document"); sent {
		t.Error("the refused drop sent an update")
	}
}

func TestWFInsertMember_OnWrongKindIsRefused(t *testing.T) {
	cases := map[string]func(m *mcpWorkflowMutator) error{
		"path on a user task":   func(m *mcpWorkflowMutator) error { return m.InsertPath("ReviewOrder", 0, "", nil) },
		"outcome on a split":    func(m *mcpWorkflowMutator) error { return m.InsertOutcome("Parallel split", 0, "X", nil) },
		"outcome on a decision": func(m *mcpWorkflowMutator) error { return m.InsertOutcome("Decision", 0, "X", nil) },
		"branch on a user task": func(m *mcpWorkflowMutator) error { return m.InsertBranch("ReviewOrder", 0, "true", nil) },
		"boundary on a decision": func(m *mcpWorkflowMutator) error {
			return m.InsertBoundaryEvent("Decision", 0, "InterruptingTimer", "", nil)
		},
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			f, m := wfMutatorFake(t)
			err := op(m)
			if err == nil || !(strings.Contains(err.Error(), "cannot hold") || strings.Contains(err.Error(), "not a parallel split")) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if _, sent := f.callByName("ped_update_document"); sent {
				t.Error("the refused op sent an update")
			}
		})
	}
}

// Sibling of #791: a value address matched every outcome with no string value,
// so an empty value on `drop Split outcome` removed the first path and on `drop Decide outcome`
// the true branch. It matches only an outcome that stores a string value.
func TestWFDropOutcome_EmptyValueMatchesNoValuelessOutcome(t *testing.T) {
	for _, ref := range []string{"Parallel split", "Decision"} {
		for name, op := range map[string]func(m *mcpWorkflowMutator) error{
			"outcome": func(m *mcpWorkflowMutator) error { return m.DropOutcome(ref, 0, "") },
			"branch":  func(m *mcpWorkflowMutator) error { return m.DropBranch(ref, 0, "") },
		} {
			f, m := wfMutatorFake(t)
			if err := op(m); err == nil {
				t.Errorf("drop %s %s '': want not found, got success", ref, name)
			}
			if _, sent := f.callByName("ped_update_document"); sent {
				t.Errorf("drop %s %s '' sent an update", ref, name)
			}
		}
	}
}
