// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
)

// An association another module owns, reaching into the module being replaced,
// is deleted by the update (mendixlabs/mxcli#1502). The module itself is
// unedited, so before the fix the gate let the update through without --force
// and without naming what it was about to remove.
func TestGateOnLocalEdits_RefusesWhenAnAssociationReachesIn(t *testing.T) {
	drift := &marketplace.Report{
		Module: "Administration",
		Findings: []marketplace.Finding{
			{Key: marketplace.ElementKey{Type: "ENTITY", Name: "Account"}, Verdict: marketplace.Unchanged},
		},
		Dependents: []marketplace.Element{{
			Key: marketplace.ElementKey{Type: "ASSOCIATION", Name: "MyFirstModule.Config_Account"},
			MDL: "create or modify association MyFirstModule.Config_Account\nfrom MyFirstModule.Config to Administration.Account;",
		}},
	}

	var out bytes.Buffer
	err := gateOnLocalEdits(&out, drift, false, "")
	if err == nil {
		t.Fatal("update allowed without --force although it removes MyFirstModule.Config_Account")
	}
	if !strings.Contains(err.Error(), "MyFirstModule.Config_Account") {
		t.Errorf("refusal does not name the association: %v", err)
	}

	out.Reset()
	if err := gateOnLocalEdits(&out, drift, true, ""); err != nil {
		t.Fatalf("--force must proceed: %v", err)
	}
	if !strings.Contains(out.String(), "MyFirstModule.Config_Account") {
		t.Errorf("--force output does not name what is removed:\n%s", out.String())
	}

	// Control: the same report without the dependent passes the gate, so the
	// refusal above is the dependent's doing and nothing else's.
	drift.Dependents = nil
	if err := gateOnLocalEdits(&out, drift, false, ""); err != nil {
		t.Fatalf("control: an unedited module with no dependents must pass the gate: %v", err)
	}
}
