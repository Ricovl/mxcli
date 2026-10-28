// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1615: `init` and `init --sync-skills` overwrote an edited
// bundled rule (example_microflow.star, shipped to be customised) and
// recreated it beside a renamed copy, so CUSTOM002 fired from the stock list
// again at every session start.

const exampleRule = "example_microflow.star"

// v0270ExampleRule is example_microflow.star exactly as mxcli v0.27.0 shipped
// it (md5 cec8b599…, the stock copy the report measured).
func v0270ExampleRule(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "lint-rules", "example_microflow.v0.27.0.star"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bundledRule(t *testing.T, name string) string {
	t.Helper()
	data, err := lintRulesFS.ReadFile("lint-rules/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// editedExampleRule is the report's edit: the stock rule with extra prefixes.
func editedExampleRule(t *testing.T) string {
	t.Helper()
	stock := string(v0270ExampleRule(t))
	edited := strings.Replace(stock, `"OCH_"]`, `"OCH_", "TEST_", "GET_", "PRS_"]`, 1)
	if edited == stock {
		t.Fatal("fixture no longer has the VALID_PREFIXES line the edit targets")
	}
	return edited
}

func TestSyncProjectTooling_KeepsEditedBundledLintRule(t *testing.T) {
	dir := newSyncProject(t)
	rule := filepath.Join(dir, ".claude", "lint-rules", exampleRule)
	edited := editedExampleRule(t)
	mustWrite(t, rule, edited)

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, rule); got != edited {
		t.Fatalf("sync overwrote the edited %s", exampleRule)
	}
	if strings.Contains(strings.Join(res.LintRules, ","), exampleRule) {
		t.Errorf("sync reports the kept file as refreshed: %v", res.LintRules)
	}
	if !strings.Contains(strings.Join(res.LintRulesKept, ","), exampleRule) {
		t.Errorf("sync does not report the kept file: %+v", res)
	}
}

// Control: a copy that is a version mxcli shipped is the stock rule, not an
// edit, and is brought up to date — or upgrades would stop reaching every
// project initialised by an older mxcli.
func TestSyncProjectTooling_RefreshesPreviouslyShippedLintRule(t *testing.T) {
	dir := newSyncProject(t)
	rule := filepath.Join(dir, ".claude", "lint-rules", exampleRule)
	old := v0270ExampleRule(t)
	if string(old) == bundledRule(t, exampleRule) {
		t.Fatal("fixture equals the current rule; it cannot show a refresh")
	}
	mustWrite(t, rule, string(old))

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, rule); got != bundledRule(t, exampleRule) {
		t.Errorf("the v0.27.0 stock %s was not refreshed", exampleRule)
	}
	if !strings.Contains(strings.Join(res.LintRules, ","), exampleRule) {
		t.Errorf("refresh not reported: %v", res.LintRules)
	}
	if len(res.LintRulesKept) != 0 {
		t.Errorf("stock copy reported as an edit: %v", res.LintRulesKept)
	}
}

// A stock copy checked out with CRLF line endings is the stock rule too.
func TestSyncProjectTooling_CRLFStockLintRuleIsCurrent(t *testing.T) {
	dir := newSyncProject(t)
	rule := filepath.Join(dir, ".claude", "lint-rules", exampleRule)
	crlf := strings.ReplaceAll(bundledRule(t, exampleRule), "\n", "\r\n")
	mustWrite(t, rule, crlf)

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, rule); got != crlf {
		t.Error("a CRLF checkout of the current rule was rewritten")
	}
	if len(res.LintRulesKept) != 0 || strings.Contains(strings.Join(res.LintRules, ","), exampleRule) {
		t.Errorf("CRLF copy not treated as current: written %v, kept %v", res.LintRules, res.LintRulesKept)
	}
}

// The session-start sync says what it kept once per mxcli, not every session.
func TestReportToolingSync_KeptLintRulesSaidOncePerMxcli(t *testing.T) {
	dir := newSyncProject(t)
	mustWrite(t, filepath.Join(dir, ".claude", "lint-rules", exampleRule), editedExampleRule(t))
	mustWrite(t, filepath.Join(dir, ".claude", "lint-rules", "my_rules.star"), "RULE_ID = \"MPR001\"\n")

	report := func() string {
		t.Helper()
		res, err := syncProjectTooling(dir)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		reportToolingSync(&out, &errOut, res)
		return out.String() + errOut.String()
	}
	first := report()
	if !strings.Contains(first, "Kept 1 edited bundled lint rule(s)") || !strings.Contains(first, exampleRule) {
		t.Errorf("first sync does not name the kept rule:\n%s", first)
	}
	if second := report(); second != "" {
		t.Errorf("second sync is not quiet:\n%s", second)
	}
}

// The report's third step: the edit moved to a new file name, keeping its
// rule ID. Recreating the stock file beside it runs CUSTOM002 twice, once
// from the stock list.
func TestSyncProjectTooling_DoesNotRecreateRenamedBundledLintRule(t *testing.T) {
	dir := newSyncProject(t)
	rulesDir := filepath.Join(dir, ".claude", "lint-rules")
	renamed := filepath.Join(rulesDir, "my_prefixes.star")
	mustWrite(t, renamed, editedExampleRule(t))

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rulesDir, exampleRule)); !os.IsNotExist(err) {
		t.Fatalf("sync recreated %s beside the renamed copy", exampleRule)
	}
	if got := strings.Join(res.LintRulesShadowed, ","); !strings.Contains(got, exampleRule) || !strings.Contains(got, "my_prefixes.star") {
		t.Errorf("skipped rule not reported with the file that defines its ID: %q", got)
	}
}

// Control: a deleted bundled rule with no copy defining its ID is written
// again — deleting the file is how an edited one is reset to mxcli's version.
func TestSyncProjectTooling_RecreatesDeletedBundledLintRule(t *testing.T) {
	dir := newSyncProject(t)
	rule := filepath.Join(dir, ".claude", "lint-rules", exampleRule)

	if _, err := syncProjectTooling(dir); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, rule); got != bundledRule(t, exampleRule) {
		t.Errorf("deleted %s was not written again", exampleRule)
	}
}

// The same rules hold for an explicit `mxcli init` re-run, which wrote every
// bundled file unconditionally ("Created 31 lint rule files").
func TestInit_KeepsEditedAndRenamedBundledLintRules(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".claude", "lint-rules")
	edited := editedExampleRule(t)

	// Edited in place.
	mustWrite(t, filepath.Join(rulesDir, exampleRule), edited)
	runInit(t, []string{"claude"}, dir)
	if got := mustRead(t, filepath.Join(rulesDir, exampleRule)); got != edited {
		t.Fatalf("init overwrote the edited %s", exampleRule)
	}

	// Moved to a new name.
	if err := os.Rename(filepath.Join(rulesDir, exampleRule), filepath.Join(rulesDir, "my_prefixes.star")); err != nil {
		t.Fatal(err)
	}
	runInit(t, []string{"claude"}, dir)
	if _, err := os.Stat(filepath.Join(rulesDir, exampleRule)); !os.IsNotExist(err) {
		t.Errorf("init recreated %s beside the renamed copy", exampleRule)
	}
	// Every other bundled rule is still written.
	if got := mustRead(t, filepath.Join(rulesDir, "orphaned_elements.star")); got != bundledRule(t, "orphaned_elements.star") {
		t.Error("init did not write the other bundled rules")
	}
}

// Every rule this binary ships must be in the list of shipped versions, or the
// next mxcli will take an unedited copy of it for an edit and stop updating
// it. The list is append-only: an older version's line stays.
func TestShippedLintRuleHashes_CoverEmbeddedRules(t *testing.T) {
	names, err := bundledLintRuleNames()
	if err != nil || len(names) == 0 {
		t.Fatalf("no bundled lint rules: %v", err)
	}
	shipped := shippedLintRuleHashes()
	for _, name := range names {
		h := lintRuleHash([]byte(bundledRule(t, name)))
		if !shipped[name][h] {
			t.Errorf("%s: this version is not in cmd/mxcli/%s; append the line\n%s  %s",
				name, shippedLintRulesFile, h, name)
		}
	}
	// The v0.27.0 fixture has to be recognised as well.
	if !shipped[exampleRule][lintRuleHash(v0270ExampleRule(t))] {
		t.Errorf("the v0.27.0 %s is not a shipped version", exampleRule)
	}
}

func TestLintRuleID(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"x.star", "RULE_ID = \"CUSTOM002\"\n", "CUSTOM002"},
		{"x.star", "# RULE_ID = \"NOPE\"\nRULE_ID='Q1'\n", "Q1"},
		{"my_rule.star", "def check():\n    return []\n", "my_rule"},
	} {
		if got := lintRuleID(c.name, []byte(c.src)); got != c.want {
			t.Errorf("lintRuleID(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
