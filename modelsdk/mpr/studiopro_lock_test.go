// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mendixlabs/mxcli#849: a file-based write while Studio Pro has the project open
// is silently discarded by Studio Pro's next save. The writer refuses it when
// Studio Pro's lock file is beside the .mpr.

const lockUnitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// studioProLock creates the lock file the way Studio Pro names it: the project
// name lower-cased (Studio Pro's own .gitignore lists `testapp.mpr.lock` beside
// `TestApp.mpr`), here against the fixture's `app.mpr`, so it is spelled in
// upper case to prove the match ignores case.
func studioProLock(t *testing.T, w *Writer) string {
	t.Helper()
	p := filepath.Join(filepath.Dir(w.reader.path), "APP.MPR.LOCK")
	if err := os.WriteFile(p, nil, 0644); err != nil {
		t.Fatalf("create lock: %v", err)
	}
	return p
}

func TestStudioProOpen_WriteRefused(t *testing.T) {
	t.Setenv(AllowStudioProOpenEnv, "")
	stored := unitDoc(t, "Before")
	w, unitPath := newV2WriterForCommitTest(t, lockUnitID, stored)
	studioProLock(t, w)

	err := w.UpdateRawUnit(lockUnitID, unitDoc(t, "After"))
	var spErr *StudioProOpenError
	if !errors.As(err, &spErr) {
		t.Fatalf("UpdateRawUnit with Studio Pro open: err = %v, want *StudioProOpenError", err)
	}
	if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(stored) {
		t.Error("a refused write reached disk")
	}
	if err := w.DeleteUnit(lockUnitID); !errors.As(err, &spErr) {
		t.Errorf("DeleteUnit with Studio Pro open: err = %v, want *StudioProOpenError", err)
	}
	if err := w.InsertUnit("bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee", lockUnitID, "Documents", "Projects$Folder", unitDoc(t, "New")); !errors.As(err, &spErr) {
		t.Errorf("InsertUnit with Studio Pro open: err = %v, want *StudioProOpenError", err)
	}

	// Reads are never refused.
	if got, err := w.reader.GetRawUnitBytes(lockUnitID); err != nil || string(got) != string(stored) {
		t.Errorf("read with Studio Pro open: err = %v", err)
	}
}

// A write that reconciliation elides never reaches storage, so it is not
// refused: re-running an applied script with Studio Pro open stays a no-op
// rather than an error.
func TestStudioProOpen_NoOpWriteNotRefused(t *testing.T) {
	t.Setenv(AllowStudioProOpenEnv, "")
	stored := unitDoc(t, "Same")
	w, _ := newV2WriterForCommitTest(t, lockUnitID, stored)
	studioProLock(t, w)
	if err := w.UpdateRawUnit(lockUnitID, unitDoc(t, "Same")); err != nil {
		t.Fatalf("no-op write with Studio Pro open refused: %v", err)
	}
}

// Controls: the same write lands without a lock file, and with the override.
func TestStudioProOpen_Controls(t *testing.T) {
	t.Setenv(AllowStudioProOpenEnv, "")
	t.Run("NoLock", func(t *testing.T) {
		w, unitPath := newV2WriterForCommitTest(t, lockUnitID, unitDoc(t, "Before"))
		after := unitDoc(t, "After")
		if err := w.UpdateRawUnit(lockUnitID, after); err != nil {
			t.Fatalf("UpdateRawUnit without a lock: %v", err)
		}
		if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(after) {
			t.Error("write did not land")
		}
	})
	t.Run("Env", func(t *testing.T) {
		t.Setenv(AllowStudioProOpenEnv, "1")
		w, unitPath := newV2WriterForCommitTest(t, lockUnitID, unitDoc(t, "Before"))
		studioProLock(t, w)
		after := unitDoc(t, "After")
		if err := w.UpdateRawUnit(lockUnitID, after); err != nil {
			t.Fatalf("UpdateRawUnit with %s=1: %v", AllowStudioProOpenEnv, err)
		}
		if onDisk, _ := os.ReadFile(unitPath); string(onDisk) != string(after) {
			t.Error("write did not land")
		}
	})
	t.Run("Force", func(t *testing.T) {
		AllowWritesWhileStudioProOpen = true
		t.Cleanup(func() { AllowWritesWhileStudioProOpen = false })
		w, _ := newV2WriterForCommitTest(t, lockUnitID, unitDoc(t, "Before"))
		studioProLock(t, w)
		if err := w.UpdateRawUnit(lockUnitID, unitDoc(t, "After")); err != nil {
			t.Fatalf("UpdateRawUnit with --force: %v", err)
		}
	})
}

func TestStudioProLockFile_MatchesOnlyTheProjectsLock(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "TestApp.mpr")
	for _, n := range []string{"Other.mpr.lock", "TestApp.mpr.bak", "testapp.mpr.lock.old"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := StudioProLockFile(mpr); got != "" {
		t.Fatalf("StudioProLockFile = %q with no lock for this project", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "testapp.mpr.lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got, _ := StudioProLockFile(mpr); filepath.Base(got) != "testapp.mpr.lock" {
		t.Fatalf("StudioProLockFile = %q, want the lower-cased lock Studio Pro writes", got)
	}
}
