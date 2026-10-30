// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1804: a catalog refresh opened every unit file of the project,
// after every save, and on Windows each open is scanned. The reader keeps a
// copy of the units in one store file and serves an unchanged unit from it,
// so the next process's refresh opens only what changed — and builds the
// same catalog.

// forcedFullBuild builds the catalog from scratch and returns every row of
// every table, by table.
func forcedFullBuild(t *testing.T, exec *Executor) map[string][]string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	ctx.catalogForce = true
	if err := buildCatalog(ctx, true, false, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	defer func() { ctx.Catalog.Close(); ctx.Catalog = nil }()
	tables, err := ctx.Catalog.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for _, tr := range tables.Rows {
		name := fmt.Sprint(tr[0])
		// What records when the build ran, not what it found.
		if name == "catalog_meta" || name == "snapshots" || name == "projects" || strings.Contains(name, "_fts") {
			continue
		}
		res, err := ctx.Catalog.Query("SELECT * FROM " + name)
		if err != nil {
			continue // a virtual table's shadow tables
		}
		for _, r := range res.Rows {
			rows[name] = append(rows[name], fmt.Sprint(r...))
		}
		sort.Strings(rows[name])
	}
	return rows
}

func TestCatalogRefresh_NextProcessOpensOnlyChangedUnits(t *testing.T) {
	first, out, dir := openPedAppCopy(t)
	before := unitFileReads(t, first)
	want := forcedFullBuild(t, first)
	cold := unitFileReads(t, first) - before
	if cold < 100 {
		t.Fatalf("control: the first refresh read %d unit files; the fixture does not exercise the store", cold)
	}
	if err := first.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatalf("disconnect: %v\n%s", err, out.String())
	}

	second, _ := openPedAppAgain(t, dir)
	before = unitFileReads(t, second)
	got := forcedFullBuild(t, second)
	if reads := unitFileReads(t, second) - before; reads != 0 {
		t.Errorf("a refresh in the next process read %d unit files, want 0 (the first read %d)", reads, cold)
	}
	diffCatalogs(t, "unchanged project", got, want)

	// A save through mxcli: the writing process reads the unit back and
	// keeps the new copy, so the next refresh opens nothing.
	if err := agreeExec(t, second, "alter entity FeedbackModule.Feedback add attribute StoreProbe: String;"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := second.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatal(err)
	}
	third, _ := openPedAppAgain(t, dir)
	before = unitFileReads(t, third)
	afterSave := forcedFullBuild(t, third)
	if reads := unitFileReads(t, third) - before; reads != 0 {
		t.Errorf("a refresh after a save through mxcli read %d unit files, want 0", reads)
	}
	if err := third.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatal(err)
	}

	// A file changed outside mxcli (Studio Pro, a checkout): the next
	// refresh opens that file, and only it.
	touchOneUnit(t, dir)
	fourth, _ := openPedAppAgain(t, dir)
	before = unitFileReads(t, fourth)
	forcedFullBuild(t, fourth)
	if reads := unitFileReads(t, fourth) - before; reads != 1 {
		t.Errorf("a refresh after one file changed read %d unit files, want 1", reads)
	}
	if err := fourth.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatal(err)
	}

	removeUnitStore(t, dir)
	fresh, _ := openPedAppAgain(t, dir)
	diffCatalogs(t, "after a save", afterSave, forcedFullBuild(t, fresh))
}

// touchOneUnit moves one unit file's modification time on, as a save that
// rewrote it would.
func touchOneUnit(t *testing.T, dir string) {
	t.Helper()
	var path string
	_ = filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(p string, d os.DirEntry, err error) error {
		if err == nil && path == "" && strings.HasSuffix(p, ".mxunit") {
			path = p
		}
		return nil
	})
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	later := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func diffCatalogs(t *testing.T, step string, got, want map[string][]string) {
	t.Helper()
	for name, w := range want {
		g := got[name]
		if strings.Join(g, "\n") != strings.Join(w, "\n") {
			t.Errorf("%s: table %s differs from a build that read every file (%d rows, want %d)", step, name, len(g), len(w))
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s: table %s is not in a build that read every file", step, name)
		}
	}
}

// removeUnitStore deletes the persisted unit copies, so the next process
// reads every file: the reference build.
func removeUnitStore(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, ".mxcli", "cache")); err != nil {
		t.Fatal(err)
	}
}
