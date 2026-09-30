// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// ledgerLike is shaped like mxcli-ledger's tests/csv-import.test.mdl
// (ako/mxcli#837): a `--` banner, a file header doc comment, @test/@expect doc
// comments, bodies separated by `/`, a doc comment whose prose quotes a
// deprecated spelling, and a last block with no `/` after it.
const ledgerLike = `-- ============================================================
-- CSV_LandImportRows — parser tests
-- Run with:  mxcli test tests/csv-import.test.mdl -p Ledger.mpr --local
-- ============================================================

/**
 * File header: every test lands under its own batch.
 */

/**
 * @test Comma is the default when nothing else is more frequent
 * @expect $row/Amount = 61.20
 */
$n = call java action Ledger.CSV_LandImportRows (
  Csv = '2026-08-20,Albert Heijn,boodschappen,61.20,ING,Groceries',
  Batch = 't-comma', Delimiter = '');
retrieve $row from Ledger.ImportRow where Batch = 't-comma' limit 1;
/

/**
 * @test The first landed row is the header-less one
 * @expect $first/Merchant = 'Albert Heijn'
 *
 * Prose that quotes the old spelling, $x = head($rows), is not code.
 */
retrieve $rows from Ledger.ImportRow where Batch = 't-semi';
$first = head($rows);
$count = COUNT($rows);
/

/**
 * @test Last block, no separator after it
 * @expect $n2 = 1
 */
$rows2 = filter($rows, Batch = 'x');
$n2 = count($rows2);
`

// The deprecated call forms in the bodies become their statement forms, and
// nothing else in the file changes: comments, doc comments (annotations and
// prose alike), separators and layout are kept byte for byte.
func TestUpgradeSource_RewritesBodiesAndKeepsDocComments(t *testing.T) {
	want := strings.NewReplacer(
		"$first = head($rows);", "$first = head $rows;",
		"$count = COUNT($rows);", "$count = COUNT $rows;",
		"$rows2 = filter($rows, Batch = 'x');", "$rows2 = filter $rows by Batch = 'x';",
		"$n2 = count($rows2);", "$n2 = count $rows2;",
	).Replace(ledgerLike)

	res, skipped, err := UpgradeSource(ledgerLike, "tests/csv-import.test.mdl", upgrade.Options{})
	if err != nil {
		t.Fatalf("UpgradeSource: %v", err)
	}
	if skipped {
		t.Error("header reported skipped although none was asked for")
	}
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten["MDL-DEPR003"] != 2 || res.Rewritten["MDL-DEPR004"] != 2 {
		t.Errorf("rewrite counts: %v", res.Rewritten)
	}

	// The runner reads the same tests out of both: names, assertions and lines.
	before, err := parseMDLTests(ledgerLike, "x.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	after, err := parseMDLTests(res.Source, "x.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 3 || len(after) != len(before) {
		t.Fatalf("tests: %d before, %d after", len(before), len(after))
	}
	for i := range before {
		b, a := before[i], after[i]
		if a.Name != b.Name || a.Line != b.Line || a.BodyLine != b.BodyLine || !reflect.DeepEqual(a.Expects, b.Expects) {
			t.Errorf("test %d changed: %+v -> %+v", i, b, a)
		}
	}

	// Idempotent: the upgraded file has nothing left to rewrite.
	again, _, err := UpgradeSource(res.Source, "x.test.mdl", upgrade.Options{})
	if err != nil || again.Source != res.Source || again.Changed() {
		t.Errorf("not idempotent: %v\n%s", err, again.Source)
	}
}

// A test file takes no language header: check and the runner read its blocks
// as mdl 0, and a `mdl 1;` line in front of the first test makes the runner
// drop that test. So the header is not added, the header-gated constructs
// (`limit 1` here) are left as they are, and the caller is told.
func TestUpgradeSource_AddsNoHeaderToATestFile(t *testing.T) {
	res, skipped, err := UpgradeSource(ledgerLike, "x.test.mdl", upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("UpgradeSource: %v", err)
	}
	if !skipped || res.HeaderAdded {
		t.Errorf("skipped=%v HeaderAdded=%v, want the header skipped", skipped, res.HeaderAdded)
	}
	if strings.Contains(res.Source, "mdl 1;") || !strings.Contains(res.Source, "limit 1;") || len(res.GatedRewritten) > 0 {
		t.Errorf("a header-gated rewrite was applied to a test file:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "$first = head $rows;") {
		t.Errorf("the deprecated spellings were not rewritten:\n%s", res.Source)
	}
}

// Line endings survive, and so does the markdown form's prose.
func TestUpgradeSource_CRLFAndMarkdown(t *testing.T) {
	crlf := strings.ReplaceAll(ledgerLike, "\n", "\r\n")
	res, _, err := UpgradeSource(crlf, "x.test.mdl", upgrade.Options{})
	if err != nil {
		t.Fatalf("CRLF: %v", err)
	}
	if !strings.Contains(res.Source, "$first = head $rows;\r\n") || strings.Count(res.Source, "\r\n") != strings.Count(crlf, "\r\n") {
		t.Errorf("CRLF file not kept:\n%q", res.Source)
	}

	md := "# Tests\n\nProse with $x = head($L) in it.\n\n```mdl-test\n/** @test md */\n$h = head($L);\n```\n"
	res, _, err = UpgradeSource(md, "x.test.md", upgrade.Options{})
	if err != nil {
		t.Fatalf("markdown: %v", err)
	}
	if want := strings.Replace(md, "$h = head($L);", "$h = head $L;", 1); res.Source != want {
		t.Errorf("markdown:\ngot:\n%s\nwant:\n%s", res.Source, want)
	}
}

// A file the test parser refuses is reported as itself, as check reports it.
func TestUpgradeSource_RefusesAMalformedTestFile(t *testing.T) {
	src := "/** @test a */\n$x = head($L);\n/** @test b */\n$y = head($L);\n"
	if _, _, err := UpgradeSource(src, "x.test.mdl", upgrade.Options{}); err == nil ||
		!strings.Contains(err.Error(), "separator") {
		t.Fatalf("want the missing separator reported, got %v", err)
	}
}

// Every test file in the repository upgrades, keeps every test as the runner
// reads it, and is left with nothing to rewrite.
func TestUpgradeSource_RepositoryTestFiles(t *testing.T) {
	var files []string
	for _, root := range []string{"../../../mdl-examples", "../../../docs-site"} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && isTestFile(d.Name()) {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Fatal("no test files found")
	}
	for _, p := range files {
		t.Run(filepath.Base(p), func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			src := string(data)
			if _, err := CheckSource(src, p); err != nil {
				t.Skipf("check does not read it either: %v", err)
			}
			res, _, err := UpgradeSource(src, p, upgrade.Options{AddHeader: true})
			if err != nil {
				t.Fatalf("UpgradeSource: %v", err)
			}
			before, err1 := parseAny(src, p)
			after, err2 := parseAny(res.Source, p)
			if err1 != nil || err2 != nil || len(before) != len(after) {
				t.Fatalf("tests before %d (%v), after %d (%v)", len(before), err1, len(after), err2)
			}
			for i := range before {
				if before[i].Name != after[i].Name || !reflect.DeepEqual(before[i].Expects, after[i].Expects) {
					t.Errorf("test %q changed", before[i].Name)
				}
			}
			again, _, err := UpgradeSource(res.Source, p, upgrade.Options{})
			if err != nil || again.Changed() {
				t.Errorf("not idempotent: %v", err)
			}
		})
	}
}

func parseAny(content, path string) ([]TestCase, error) {
	if strings.EqualFold(filepath.Ext(path), ".md") {
		return parseMarkdownTests(content, path)
	}
	return parseMDLTests(content, path)
}
