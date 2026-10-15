// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

func openAndList(t *testing.T, path string) (headerReads int64, stamps string) {
	t.Helper()
	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()
	if _, err := r.ListModules(); err != nil {
		t.Fatalf("list modules: %v", err)
	}
	st, err := r.UnitStamps()
	if err != nil {
		t.Fatalf("stamps: %v", err)
	}
	return r.UnitHeaderReads(), fmt.Sprint(st)
}

func TestUnitIndex_PersistsAndRereadsOnlyChangedUnits(t *testing.T) {
	path, units := copyV2Fixture(t)

	first, fp1 := openAndList(t, path)
	if first != int64(len(units)) {
		t.Fatalf("first open read %d unit files, want all %d", first, len(units))
	}
	if again, fp2 := openAndList(t, path); again != 0 || fp2 != fp1 {
		t.Errorf("unchanged reopen read %d unit files (want 0), stamps changed: %v", again, fp2 != fp1)
	}

	// Rewrite one unit: same bytes, new (old enough) mtime.
	b, _ := os.ReadFile(units[0])
	_ = os.WriteFile(units[0], b, 0o644)
	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(units[0], newer, newer)
	if reads, fp3 := openAndList(t, path); reads != 1 || fp3 == fp1 {
		t.Errorf("after one edit: read %d unit files (want 1), stamps changed %v", reads, fp3 != fp1)
	}

	// Control: without the persisted index every open reads every unit.
	t.Setenv("MXCLI_NO_PERSISTED_INDEX", "1")
	if reads, _ := openAndList(t, path); reads != int64(len(units)) {
		t.Errorf("with the index disabled: read %d unit files, want %d", reads, len(units))
	}
}

func TestUnitStamps_MarksRacilyCleanUnits(t *testing.T) {
	path, units := copyV2Fixture(t)
	now := time.Now()
	_ = os.Chtimes(units[0], now, now)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	stamps, err := r.UnitStamps()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(filepath.Base(units[0]), ".mxunit")
	var racy []string
	for _, s := range stamps {
		if s.MtimeNs == -1 {
			racy = append(racy, s.ID)
		}
	}
	if len(racy) != 1 || racy[0] != want {
		t.Fatalf("racily clean stamps = %v; want [%s]", racy, want)
	}
	d1 := r.UnitContentDigest(racy)
	b, _ := os.ReadFile(units[0])
	_ = os.WriteFile(units[0], append(b[:len(b):len(b)], 0), 0o644)
	if r.UnitContentDigest(racy) == d1 {
		t.Error("content digest unchanged after the unit's contents changed")
	}
}

// TestUnitStamps_RevalidatesALongLivedReader: a reader that outlives a change
// made by someone else (a git pull during a REPL session) sees it on the next
// UnitStamps, and keeps its caches while nothing changed.
func TestUnitStamps_RevalidatesALongLivedReader(t *testing.T) {
	path, units := copyV2Fixture(t)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	first, err := r.UnitStamps()
	if err != nil {
		t.Fatal(err)
	}
	reads := r.UnitHeaderReads()
	if again, _ := r.UnitStamps(); fmt.Sprint(again) != fmt.Sprint(first) || r.UnitHeaderReads() != reads {
		t.Fatal("an unchanged project changed the stamps or re-read units")
	}

	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(units[0], newer, newer)
	after, err := r.UnitStamps()
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(after) == fmt.Sprint(first) {
		t.Error("UnitStamps did not see a unit file changed outside the reader")
	}
}

// TestUnitStamps_OneStatPassPerScope: inside a CacheUnitReads scope (one check
// or exec pass) UnitStamps stats the unit files once; after it, a change made
// in the meantime is seen.
func TestUnitStamps_OneStatPassPerScope(t *testing.T) {
	path, units := copyV2Fixture(t)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	release := r.CacheUnitReads()
	first, _ := r.UnitStamps()
	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(units[0], newer, newer)
	if again, _ := r.UnitStamps(); fmt.Sprint(again) != fmt.Sprint(first) {
		t.Error("UnitStamps re-stat'ed inside one scope")
	}
	release()
	if after, _ := r.UnitStamps(); fmt.Sprint(after) == fmt.Sprint(first) {
		t.Error("UnitStamps missed a change once the scope closed")
	}
}

// hashMismatchesByReading is VerifyContentsHashes the way it worked before
// the unit index carried hashes: every file read.
func hashMismatchesByReading(t *testing.T, r *Reader) []string {
	t.Helper()
	rows, err := r.db.Query(`SELECT UnitID, COALESCE(ContentsHash, '') FROM Unit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var blob []byte
		var hash string
		_ = rows.Scan(&blob, &hash)
		b, err := os.ReadFile(unitFilePath(r.contentsDir, blob))
		if err == nil && contentsHashOf(b) != hash {
			out = append(out, blobToUUID(blob))
		}
	}
	sort.Strings(out)
	return out
}

func reportedMismatches(t *testing.T, r *Reader) []string {
	t.Helper()
	rep, err := r.VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range rep.Mismatches {
		out = append(out, m.UnitID)
	}
	sort.Strings(out)
	return out
}

// TestVerifyContentsHashes_FromTheUnitIndex: the hashes the unit index keeps
// give the report reading every file gives — on a first open, and on a reopen
// after a unit file changed behind the index's back.
func TestVerifyContentsHashes_FromTheUnitIndex(t *testing.T) {
	path, units := copyV2Fixture(t)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := reportedMismatches(t, r), hashMismatchesByReading(t, r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("first open: reported %d mismatches, reading says %d", len(got), len(want))
	}
	r.Close()

	b, _ := os.ReadFile(units[0])
	_ = os.WriteFile(units[0], append(b[:len(b):len(b)], 0), 0o644) // a restore git made
	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(units[0], newer, newer)

	r, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, want := reportedMismatches(t, r), hashMismatchesByReading(t, r)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("after a file changed: reported %d mismatches, reading says %d", len(got), len(want))
	}
	if !strings.Contains(fmt.Sprint(got), strings.TrimSuffix(filepath.Base(units[0]), ".mxunit")) {
		t.Error("the changed file is not reported")
	}
}
