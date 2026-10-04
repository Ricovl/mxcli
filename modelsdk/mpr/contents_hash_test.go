// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#972: restoring a .mxunit with `git checkout` leaves the .mpr's
// ContentsHash describing bytes that are no longer on disk.

const hashTestUnit = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestVerifyContentsHashesCleanControl(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.Units != 1 || rep.Files != 1 {
		t.Fatalf("untouched project not clean: %+v", rep)
	}
}

func TestVerifyContentsHashesDetectsRevertedFile(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	reverted := unitDoc(t, "RevertedByGit")
	if err := os.WriteFile(unitPath, reverted, 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Mismatches) != 1 {
		t.Fatalf("want 1 mismatch, got %+v", rep)
	}
	m := rep.Mismatches[0]
	if m.UnitID != hashTestUnit || m.Path != unitPath || m.Actual != hashOf(reverted) || m.Stored == m.Actual {
		t.Fatalf("wrong mismatch: %+v", m)
	}
}

func TestVerifyContentsHashesMissingAndOrphanFiles(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	if err := os.Remove(unitPath); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(w.ConcreteReader().ContentsDir(), "12", "34", "12345678-0000-0000-0000-000000000000.mxunit")
	if err := os.MkdirAll(filepath.Dir(orphan), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.MissingFiles) != 1 || rep.MissingFiles[0].UnitID != hashTestUnit {
		t.Fatalf("missing file not reported: %+v", rep)
	}
	if len(rep.OrphanFiles) != 1 || rep.OrphanFiles[0] != orphan {
		t.Fatalf("orphan file not reported: %+v", rep)
	}
	if len(rep.Mismatches) != 0 {
		t.Fatalf("unexpected mismatches: %+v", rep.Mismatches)
	}
}

func TestRepairContentsHashesRewritesIndex(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	reverted := unitDoc(t, "RevertedByGit")
	if err := os.WriteFile(unitPath, reverted, 0644); err != nil {
		t.Fatal(err)
	}
	rep, n, err := w.RepairContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(rep.Mismatches) != 1 {
		t.Fatalf("want 1 row repaired, got %d (%+v)", n, rep)
	}
	if got := storedHash(t, w, hashTestUnit); got != hashOf(reverted) {
		t.Fatalf("ContentsHash = %q, want %q", got, hashOf(reverted))
	}
	after, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if !after.Clean() {
		t.Fatalf("not clean after repair: %+v", after)
	}
	// The file itself is the source of truth and must be left alone.
	if b, _ := os.ReadFile(unitPath); string(b) != string(reverted) {
		t.Fatal("repair modified the unit file")
	}
}

func TestRepairContentsHashesRefusedWhileStudioProOpen(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	stored := storedHash(t, w, hashTestUnit)
	if err := os.WriteFile(unitPath, unitDoc(t, "RevertedByGit"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.ConcreteReader().Path()+".lock", nil, 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := w.RepairContentsHashes()
	var open *StudioProOpenError
	if !errors.As(err, &open) {
		t.Fatalf("want StudioProOpenError, got %v", err)
	}
	if got := storedHash(t, w, hashTestUnit); got != stored {
		t.Fatal("hash rewritten despite the guard")
	}
}

func TestVerifyContentsHashesV1NotApplicable(t *testing.T) {
	r := &Reader{version: MPRVersionV1}
	if _, err := r.VerifyContentsHashes(); !errors.Is(err, ErrNotMPRv2) {
		t.Fatalf("want ErrNotMPRv2, got %v", err)
	}
}
