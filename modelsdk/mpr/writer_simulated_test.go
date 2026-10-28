// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"os"
	"testing"
)

// mendixlabs/mxcli#1517: check applies the flow changes it predicts to a
// simulation, so a later statement on the same flow is predicted against them.
// A simulation holds updates and serves them back, and nothing it is given
// reaches storage — neither the update, nor any write it cannot hold.

func TestSimulatedWritesAreReadBackAndNeverWritten(t *testing.T) {
	stored := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://a.test")
	w, unitPath := newV2WriterForCommitTest(t, deferredUnitID, stored)
	seedTransactionTable(t, w, "before-the-run")
	seedUnitRow(t, w, deferredUnitID, "22222222-2222-2222-2222-222222222222", "Documents")

	discard, ok := w.SimulateWrites()
	if !ok {
		t.Fatal("no simulation opened")
	}
	if _, again := w.SimulateWrites(); again {
		t.Error("a second simulation opened inside the first")
	}
	changed := ruleDoc(t, true, "33333333-3333-3333-3333-333333333333", "https://b.test")
	if err := w.UpdateRawUnit(deferredUnitID, changed); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := w.reader.GetRawUnitBytes(deferredUnitID); string(got) != string(changed) {
		t.Error("the simulated update is not read back")
	}
	// What a simulation cannot hold is refused, before it touches anything.
	if err := w.DeleteUnit(deferredUnitID); !errors.Is(err, ErrSimulatedWrite) {
		t.Errorf("delete during a simulation: %v, want ErrSimulatedWrite", err)
	}
	if err := w.FlushDeferredWrites(); !errors.Is(err, ErrSimulatedWrite) {
		t.Errorf("flush during a simulation: %v, want ErrSimulatedWrite", err)
	}
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(stored) {
		t.Error("a simulated write reached disk")
	}

	discard()
	if got, _ := w.reader.GetRawUnitBytes(deferredUnitID); string(got) != string(stored) {
		t.Error("the stored unit is not read back once the simulation is discarded")
	}
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(stored) {
		t.Error("discarding the simulation wrote")
	}
	if got := transactionID(t, w); got != "before-the-run" {
		t.Errorf("LastTransactionID = %q after a simulation", got)
	}
	// And the writer writes again once it is over.
	if err := w.UpdateRawUnit(deferredUnitID, changed); err != nil {
		t.Fatalf("update after the simulation: %v", err)
	}
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(changed) {
		t.Error("a write after the simulation did not land")
	}
}
