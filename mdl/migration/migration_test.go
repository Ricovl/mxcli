// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// The generated tables on the docs page are what the registries say today. A
// change or deprecation registered without regenerating the page fails here
// (and in `make check-migration-reference`), the way sync-skills drift does.
func TestMigrationReferenceIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", PagePath)
	page, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Splice(string(page), GeneratedTables())
	if err != nil {
		t.Fatal(err)
	}
	if want != string(page) {
		t.Fatalf("%s is stale: run `make gen-migration-reference` and commit the page", PagePath)
	}
}

// The staleness check must be able to fail: a page whose tables lack a code
// is not what Splice produces. The control for the test above.
func TestMigrationReferenceDetectsAStalePage(t *testing.T) {
	gen := GeneratedTables()
	page, err := Splice("intro\n"+BeginMarker+"\n"+EndMarker+"\noutro\n", gen)
	if err != nil {
		t.Fatal(err)
	}
	first := Changes()[0].Code
	stale := strings.Replace(page, "| `"+first+"` |", "| `MDL-V1-GONE` |", 1)
	if again, _ := Splice(stale, gen); again == stale {
		t.Fatal("Splice left a stale page unchanged: the check could never fail")
	}
	if again, _ := Splice(page, gen); again != page {
		t.Fatal("Splice is not idempotent on a current page")
	}
	if !strings.HasPrefix(page, "intro\n") || !strings.HasSuffix(page, "outro\n") {
		t.Fatal("Splice changed the hand-written text outside the markers")
	}
}

func TestSpliceNeedsBothMarkersOnce(t *testing.T) {
	for _, page := range []string{
		"no markers",
		BeginMarker + " only",
		EndMarker + "\n" + BeginMarker,
		BeginMarker + BeginMarker + EndMarker,
	} {
		if _, err := Splice(page, "x"); err == nil {
			t.Errorf("Splice(%q) succeeded, want an error", page)
		}
	}
}

// Every registered code has a row: one per language change and one per
// deprecated spelling, under its own code.
func TestGeneratedTablesListEveryCode(t *testing.T) {
	gen := GeneratedTables()
	changes, deps := Changes(), Deprecations()
	if len(changes) == 0 || len(deps) == 0 {
		t.Fatalf("empty registries: %d changes, %d deprecations", len(changes), len(deps))
	}
	for _, c := range changes {
		if strings.Count(gen, "| `"+c.Code+"` |") != 1 {
			t.Errorf("%s: want exactly one row", c.Code)
		}
	}
	for _, e := range deps {
		if strings.Count(gen, "| `"+e.Code+"` |") != 1 {
			t.Errorf("%s: want exactly one row", e.Code)
		}
	}
	rows := 0
	for _, line := range strings.Split(gen, "\n") {
		if strings.HasPrefix(line, "| `MDL-") {
			rows++
			// A `|` inside a cell must be escaped, or it splits the row.
			if cells := strings.Count(strings.ReplaceAll(line, `\|`, ""), "|"); cells != 6 {
				t.Errorf("row has %d separators, want 6: %s", cells, line)
			}
		}
	}
	if rows != len(changes)+len(deps) {
		t.Errorf("%d rows, want %d", rows, len(changes)+len(deps))
	}
}

// Both lists are complete: the executor's exec-time changes are in the table
// as well as the visitor's, and each parse-time change says what the upgrade
// does with it.
func TestChangesCoverVisitorAndExecutor(t *testing.T) {
	seen := map[Decided]int{}
	for _, c := range Changes() {
		seen[c.Decided]++
		if c.Decided == Parse && !c.Rewritable && c.NoRewrite == "" {
			t.Errorf("%s: a parse-time change with neither a rewrite nor a reason", c.Code)
		}
		if c.Since != langver.V1 {
			t.Errorf("%s: since %s; the page documents mdl 1 changes only", c.Code, c.Since)
		}
	}
	if seen[Parse] == 0 || seen[Exec] == 0 {
		t.Errorf("changes by stage = %v, want both parse and exec", seen)
	}
}

// `mxcli help <code>` answers for every code, case-insensitively, with the
// four facts decision 3 asks for.
func TestHelpEveryCode(t *testing.T) {
	for _, e := range Deprecations() {
		text, ok := Help(strings.ToLower(e.Code))
		if !ok {
			t.Errorf("%s: no help", e.Code)
			continue
		}
		for _, want := range []string{e.Code, "Old form:", e.Old, "New form:", e.Canonical, "Rewrite:", "Refused from:"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: help lacks %q:\n%s", e.Code, want, text)
			}
		}
	}
	for _, c := range Changes() {
		text, ok := Help(c.Code)
		if !ok {
			t.Errorf("%s: no help", c.Code)
			continue
		}
		for _, want := range []string{c.Code, c.Old, c.New, "Rewrite:", c.RewriteSummary()} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: help lacks %q:\n%s", c.Code, want, text)
			}
		}
	}
	for _, code := range []string{"MDL-DEPR999", "MDL-V1-NOPE", "MDL001", ""} {
		if _, ok := Help(code); ok {
			t.Errorf("Help(%q) answered for an unregistered code", code)
		}
	}
}

func TestIsCode(t *testing.T) {
	for s, want := range map[string]bool{
		"MDL-DEPR001": true, "mdl-depr001": true, "MDL-V1-LIMIT1": true, "mdl-v1-x": true,
		"exec": false, "MDL001": false, "MDL-LANG01": false,
	} {
		if got := IsCode(s); got != want {
			t.Errorf("IsCode(%q) = %v, want %v", s, got, want)
		}
	}
}
