// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#1804: a process that reads the whole project (a catalog refresh)
// opened every .mxunit, after every save; on Windows each open is scanned.
// The unit store keeps the bytes in one file and serves a unit from it while
// the unit's file is unchanged.

// readEvery reads every unit of the project in one pass, the way a catalog
// build does, and returns the bytes by ID.
func readEvery(t *testing.T, r *Reader, ids []string) map[string][]byte {
	t.Helper()
	release := r.CacheUnitReads()
	defer release()
	if err := r.buildUnitCache(); err != nil {
		t.Fatalf("build unit cache: %v", err)
	}
	out := map[string][]byte{}
	for _, id := range ids {
		b, err := r.readMprContents(blobToUUID(uuidToBlob(id)))
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		out[id] = b
	}
	return out
}

func storeProject(t *testing.T, n int) (*v2Project, []string) {
	t.Helper()
	p := newV2Project(t)
	ids := []string{p.add(t, 0, "Projects$ModuleImpl")}
	for i := 1; i <= n; i++ {
		ids = append(ids, p.add(t, i, "Microflows$Microflow"))
	}
	return p, ids
}

// warm runs one process over the project and closes it, leaving the store.
func warm(t *testing.T, p *v2Project, ids []string) {
	t.Helper()
	r := p.reader()
	readEvery(t, r, ids)
	if got := r.UnitFileReads(); got < int64(len(ids)) {
		t.Fatalf("control: the cold pass read %d files, want >= %d", got, len(ids))
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUnitStore_SecondProcessReadsNoUnitFile(t *testing.T) {
	const n = 40
	p, ids := storeProject(t, n)
	warm(t, p, ids)

	second := p.reader()
	got := readEvery(t, second, ids)
	if reads := second.UnitFileReads(); reads != 0 {
		t.Errorf("a second pass over an unchanged project read %d unit files, want 0", reads)
	}
	if hits := second.UnitStoreHits(); hits < int64(len(ids)) {
		t.Errorf("the store served %d reads, want >= %d", hits, len(ids))
	}
	for _, id := range ids {
		want, err := os.ReadFile(p.unitPath(id))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[id], want) {
			t.Errorf("unit %s: the store served bytes that differ from its file", id)
		}
	}
}

// A save changes the file and its row: the next pass reads that one file.
func TestUnitStore_ChangedUnitIsReadFromItsFile(t *testing.T) {
	p, ids := storeProject(t, 10)
	warm(t, p, ids)

	changed := typedDoc(t, "Microflows$Microflow", "Renamed")
	p.rewrite(t, ids[3], changed)
	bumpMtime(t, p.unitPath(ids[3]))

	r := p.reader()
	got := readEvery(t, r, ids)
	if !bytes.Equal(got[ids[3]], changed) {
		t.Fatalf("the changed unit was served from the store, not its file")
	}
	if reads := r.UnitFileReads(); reads != 1 {
		t.Errorf("a pass after one save read %d unit files, want 1", reads)
	}
}

// A plain git checkout rewrites the file and can leave the Unit table's hash
// as it was (`mxcli git doctor` exists for that drift). The file's stat still
// changes, and that is enough.
func TestUnitStore_FileChangedBehindTheHashIsReadFromTheFile(t *testing.T) {
	p, ids := storeProject(t, 10)
	warm(t, p, ids)

	changed := typedDoc(t, "Microflows$Microflow", "UnitX") // the length of "Unit2": only the stat tells them apart
	if err := os.WriteFile(p.unitPath(ids[2]), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMtime(t, p.unitPath(ids[2]))

	got := readEvery(t, p.reader(), ids)
	if !bytes.Equal(got[ids[2]], changed) {
		t.Fatalf("a file changed behind its row's hash was served from the store")
	}
}

// A project copied over another with its timestamps kept has the same stat
// and another hash.
func TestUnitStore_RowHashChangeIsReadFromTheFile(t *testing.T) {
	p, ids := storeProject(t, 10)
	warm(t, p, ids)

	if _, err := p.db.Exec(`UPDATE Unit SET ContentsHash = 'other' WHERE UnitID = ?`, uuidToBlob(ids[4])); err != nil {
		t.Fatal(err)
	}
	r := p.reader()
	readEvery(t, r, ids)
	if reads := r.UnitFileReads(); reads != 1 {
		t.Errorf("a pass after one row's hash changed read %d unit files, want 1", reads)
	}
}

// A deleted unit's copy does not outlive it.
func TestUnitStore_DeletedUnitIsDropped(t *testing.T) {
	p, ids := storeProject(t, 10)
	warm(t, p, ids)

	gone := ids[5]
	if _, err := p.db.Exec(`DELETE FROM Unit WHERE UnitID = ?`, uuidToBlob(gone)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.unitPath(gone)); err != nil {
		t.Fatal(err)
	}
	rest := append(append([]string{}, ids[:5]...), ids[6:]...)
	warmAgain := p.reader()
	readEvery(t, warmAgain, rest)
	if err := warmAgain.Close(); err != nil {
		t.Fatal(err)
	}

	if n := storedRows(t, p, blobToUUID(uuidToBlob(gone))); n != 0 {
		t.Errorf("the store still holds %d copies of a deleted unit", n)
	}
}

// A store an older or newer mxcli wrote is replaced, not misread.
func TestUnitStore_AnotherFormatIsReplaced(t *testing.T) {
	p, ids := storeProject(t, 5)
	path := storePath(p)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT); INSERT INTO meta VALUES ('version', '0')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	warm(t, p, ids)
	second := p.reader()
	readEvery(t, second, ids)
	if reads := second.UnitFileReads(); reads != 0 {
		t.Errorf("after replacing another format's store a second pass read %d files, want 0", reads)
	}
}

// The store is a copy of the whole model: it must never be committed, and
// .mxcli/ is not ignored in every project.
func TestUnitStore_IsGitIgnored(t *testing.T) {
	p, ids := storeProject(t, 3)
	warm(t, p, ids)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(storePath(p)), ".gitignore"))
	if err != nil {
		t.Fatalf("the store's directory has no .gitignore: %v", err)
	}
	if !strings.Contains(string(b), "\n*\n") {
		t.Errorf("the store's .gitignore does not ignore everything:\n%s", b)
	}
}

func storePath(p *v2Project) string {
	return filepath.Join(p.root, ".mxcli", unitStoreDir, unitStoreName)
}

func storedRows(t *testing.T, p *v2Project, id string) int {
	t.Helper()
	db, err := sql.Open("sqlite", storePath(p))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM units WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// bumpMtime moves a file's modification time on, so a test does not depend
// on the file system's timestamp resolution to see a rewrite.
func bumpMtime(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	later := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}
