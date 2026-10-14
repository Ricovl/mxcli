// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// copyV2Fixture copies the expr-checker project with every file's mtime set
// an hour back, so nothing is racily clean, and an empty .mxcli/ to persist in.
func copyV2Fixture(t *testing.T) (mprPath string, units []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	_ = os.RemoveAll(filepath.Join(dir, ".mxcli"))
	if err := os.Mkdir(filepath.Join(dir, ".mxcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".mxunit") {
			units = append(units, p)
			_ = os.Chtimes(p, old, old)
		}
		return nil
	})
	return filepath.Join(dir, "minimal.mpr"), units
}

func openAndList(t *testing.T, path string) (headerReads int64, fp string) {
	t.Helper()
	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()
	if _, err := r.ListModules(); err != nil {
		t.Fatalf("list modules: %v", err)
	}
	fp, _, _, err = r.UnitStateFingerprint()
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return r.UnitHeaderReads(), fp
}

func TestUnitIndex_PersistsAndRereadsOnlyChangedUnits(t *testing.T) {
	path, units := copyV2Fixture(t)

	first, fp1 := openAndList(t, path)
	if first != int64(len(units)) {
		t.Fatalf("first open read %d unit files, want all %d", first, len(units))
	}
	if again, fp2 := openAndList(t, path); again != 0 || fp2 != fp1 {
		t.Errorf("unchanged reopen read %d unit files (want 0), fingerprint changed: %v", again, fp2 != fp1)
	}

	// Rewrite one unit: same bytes, new (old enough) mtime.
	b, _ := os.ReadFile(units[0])
	_ = os.WriteFile(units[0], b, 0o644)
	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(units[0], newer, newer)
	if reads, fp3 := openAndList(t, path); reads != 1 || fp3 == fp1 || fp3 == "" {
		t.Errorf("after one edit: read %d unit files (want 1), fingerprint changed %v, empty %v", reads, fp3 != fp1, fp3 == "")
	}

	// Control: without the persisted index every open reads every unit.
	t.Setenv("MXCLI_NO_PERSISTED_INDEX", "1")
	if reads, _ := openAndList(t, path); reads != int64(len(units)) {
		t.Errorf("with the index disabled: read %d unit files, want %d", reads, len(units))
	}
}

func TestUnitStateFingerprint_ListsRacilyCleanUnits(t *testing.T) {
	path, units := copyV2Fixture(t)
	now := time.Now()
	_ = os.Chtimes(units[0], now, now)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, racy, _, err := r.UnitStateFingerprint()
	want := strings.TrimSuffix(filepath.Base(units[0]), ".mxunit")
	if err != nil || len(racy) != 1 || racy[0] != want {
		t.Fatalf("racy = %v, %v; want [%s]", racy, err, want)
	}
	d1 := r.UnitContentDigest(racy)
	b, _ := os.ReadFile(units[0])
	_ = os.WriteFile(units[0], append(b[:len(b):len(b)], 0), 0o644)
	if r.UnitContentDigest(racy) == d1 {
		t.Error("content digest unchanged after the unit's contents changed")
	}
}
