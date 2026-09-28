// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// `insert before X { a; b; }` over MCP stores the activities together, in
// order, right before X: every add at X's index, which a batch counts against
// the list as it was (the rule InsertAfterActivity was measured against).
func TestWFInsertBeforeActivity_KeepsOrder(t *testing.T) {
	sim, m := listMutator(t, named("Start", "R", "P", "End"))
	if err := m.InsertBeforeActivity("P", 0, []workflows.WorkflowActivity{waitWith("I1"), waitWith("I2")}); err != nil {
		t.Fatal(err)
	}
	if got, want := sim.field("/flow/activities", "name"), []any{"Start", "R", "I1", "I2", "P", "End"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stored %v, want %v", got, want)
	}
}

// The MCP resolver answers as the modelsdk one does (the shared
// backend.ResolveWorkflowActivityTarget): a name, @n, an ambiguity that lists
// its matches.
func TestWFResolveAlterTarget(t *testing.T) {
	flow := named("Start", "R", "P", "End")
	flow[1]["caption"] = "Same"
	flow[2]["caption"] = "Same"
	_, m := listMutator(t, flow)

	got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"P"}})
	if err != nil || got != (backend.AlterTargetMatch{Kind: "wait for notification", Name: "P"}) {
		t.Errorf("P: got %+v, %v", got, err)
	}
	got, err = m.ResolveAlterTarget(backend.AlterTarget{Caption: "Same", Ordinal: 2})
	if err != nil || got.Name != "P" {
		t.Errorf("'Same'@2: got %+v, %v", got, err)
	}
	if _, err := m.ResolveAlterTarget(backend.AlterTarget{Caption: "Same"}); err == nil ||
		!strings.Contains(err.Error(), "@1 wait for notification R, @2 wait for notification P") {
		t.Errorf("'Same': err = %v, want the two matches listed", err)
	}
	if _, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"Nope"}}); err == nil {
		t.Error("Nope resolved")
	}
}
