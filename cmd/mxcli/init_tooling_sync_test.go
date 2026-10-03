// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSyncProject is a project as `mxcli init` left it, ready for a sync: an
// .mpr, a .claude/lint-rules directory, and the tooling dirs.
func newSyncProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Demo")
	for _, d := range []string{".claude/lint-rules", ".ai-context/skills"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Demo.mpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ako/mxcli#952: `init --sync-skills` refreshed only the skills, so a project
// kept the lint rules of whichever mxcli first initialised it. The bundled
// rules are refreshed now; a project's own rule beside them is not touched.
func TestSyncProjectTooling_RefreshesBundledLintRulesOnly(t *testing.T) {
	dir := newSyncProject(t)
	names, err := bundledLintRuleNames()
	if err != nil || len(names) == 0 {
		t.Fatalf("no bundled lint rules: %v", err)
	}
	rulesDir := filepath.Join(dir, ".claude", "lint-rules")
	stale := filepath.Join(rulesDir, names[0])
	mustWrite(t, stale, "# yesterday's rule\n")
	user := filepath.Join(rulesDir, "zz_my_project_rule.star")
	const userRule = "RULE_ID = \"PROJ001\"\ndef check():\n    return []\n"
	mustWrite(t, user, userRule)

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := lintRulesFS.ReadFile("lint-rules/" + names[0])
	if got := mustRead(t, stale); got != string(want) {
		t.Errorf("bundled rule %s not refreshed", names[0])
	}
	if got := mustRead(t, user); got != userRule {
		t.Errorf("project rule was modified:\n%s", got)
	}
	if !strings.Contains(strings.Join(res.LintRules, ","), names[0]) {
		t.Errorf("result does not report %s: %v", names[0], res.LintRules)
	}

	// Control: a second sync has nothing to do.
	res2, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.LintRules) != 0 || len(res2.Docs) != 0 || res2.Skills.Stale() || res2.StampWritten {
		t.Errorf("second sync was not a no-op: %+v", res2)
	}
}

// A project without .claude/lint-rules (a tool that does not use them) does
// not grow one from the sync.
func TestSyncProjectTooling_NoLintRulesDirStaysAbsent(t *testing.T) {
	dir := newSyncProject(t)
	if err := os.RemoveAll(filepath.Join(dir, ".claude", "lint-rules")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncProjectTooling(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "lint-rules")); !os.IsNotExist(err) {
		t.Error("sync created .claude/lint-rules")
	}
}

// The mxcli section of CLAUDE.md is refreshed; what the project wrote above
// and below the markers survives byte for byte.
func TestSyncProjectTooling_RefreshesOnlyTheMarkedSection(t *testing.T) {
	dir := newSyncProject(t)
	const above = "<!-- project preface -->\n"
	const below = "\n## Our conventions\n\nCustomers are never deleted.\n"
	path := filepath.Join(dir, "CLAUDE.md")
	mustWrite(t, path, above+wrapMxcliSection("# Mendix Project: Demo\n\nOld guidance from an older mxcli.\n")+below)

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, path)
	want := above + wrapMxcliSection(generateClaudeMD("Demo", "Demo.mpr")) + below
	if got != want {
		t.Errorf("CLAUDE.md after sync:\n%s", got)
	}
	if strings.Contains(got, "Old guidance") {
		t.Error("stale mxcli section survived")
	}
	if len(res.Docs) != 1 || res.Docs[0] != "CLAUDE.md" {
		t.Errorf("Docs = %v, want [CLAUDE.md]", res.Docs)
	}
	// AGENTS.md did not exist and must not be created by a sync.
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("sync created AGENTS.md")
	}
}

// A CLAUDE.md from before the markers cannot be split into mxcli's text and
// the project's, so the unattended sync leaves it alone and says why.
func TestSyncProjectTooling_UnmarkedDocIsLeftAlone(t *testing.T) {
	dir := newSyncProject(t)
	path := filepath.Join(dir, "AGENTS.md")
	const legacy = "# Mendix Project: Demo\n\nWritten by an mxcli without markers, then edited by hand.\n"
	mustWrite(t, path, legacy)

	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != legacy {
		t.Errorf("unmarked AGENTS.md was rewritten:\n%s", got)
	}
	var out, errOut bytes.Buffer
	reportToolingSync(&out, &errOut, res)
	if !strings.Contains(errOut.String(), "AGENTS.md has no mxcli section markers") {
		t.Errorf("no notice about the missing markers:\n%s", errOut.String())
	}
}

// The sync stamps the version it ran with.
func TestSyncProjectTooling_WritesStamp(t *testing.T) {
	dir := newSyncProject(t)
	withBinaryVersion(t, "v0.25.0", "2026-10-01T00:00:00Z")
	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.StampWritten {
		t.Error("stamp not written")
	}
	if s := readToolingStamp(dir); s == nil || s.Version != "v0.25.0" {
		t.Errorf("stamp = %+v", s)
	}
}

// An older binary must not "refresh" the project back to what it knows: that
// would downgrade the skills, rules and CLAUDE.md a newer mxcli wrote.
func TestSyncProjectTooling_OlderBinaryRefuses(t *testing.T) {
	dir := newSyncProject(t)
	withBinaryVersion(t, "v0.25.0", "")
	if _, err := syncProjectTooling(dir); err != nil {
		t.Fatal(err)
	}
	names, _ := bundledLintRuleNames()
	rule := filepath.Join(dir, ".claude", "lint-rules", names[0])
	mustWrite(t, rule, "# written by v0.25.0, newer than this binary knows\n")

	withBinaryVersion(t, "v0.24.0", "")
	res, err := syncProjectTooling(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.RefusedFor == nil {
		t.Fatal("older binary synced")
	}
	if got := mustRead(t, rule); !strings.Contains(got, "written by v0.25.0") {
		t.Error("older binary overwrote the newer lint rule")
	}
	if s := readToolingStamp(dir); s.Version != "v0.25.0" {
		t.Errorf("stamp downgraded to %s", s.Version)
	}
	var out, errOut bytes.Buffer
	reportToolingSync(&out, &errOut, res)
	if !strings.Contains(errOut.String(), "v0.24.0") || !strings.Contains(errOut.String(), "v0.25.0") {
		t.Errorf("refusal does not name both versions:\n%s", errOut.String())
	}
}

func TestMergeMxcliSection(t *testing.T) {
	cases := []struct {
		name, existing, want string
		ok                   bool
	}{
		{"no markers", "# hand written\n", "", false},
		{"begin only", mxcliSectionBegin + "\nx\n", "", false},
		{"end before begin", mxcliSectionEnd + "\n" + mxcliSectionBegin + "\n", "", false},
		{"replaced in place", "a\n" + wrapMxcliSection("old") + "b\n", "a\n" + wrapMxcliSection("new") + "b\n", true},
		{"older begin wording still matches", "<!-- mxcli:begin (v1) -->\nold\n" + mxcliSectionEnd + "\ntail", wrapMxcliSection("new") + "tail", true},
		{"end marker at EOF without newline", mxcliSectionBegin + "\nold\n" + mxcliSectionEnd, wrapMxcliSection("new"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := mergeMxcliSection(c.existing, "new")
			if ok != c.ok || (ok && got != c.want) {
				t.Errorf("merge = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

// ako/mxcli#952 item 4: CLAUDE.md says scripts live in mdlsource/, and init
// did not create it. Init also stamps the version and writes CLAUDE.md inside
// the markers; re-running it keeps the project's notes below them.
func TestInit_CreatesMdlsourceStampAndMarkedDocs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Demo.mpr"), "")
	runInit(t, []string{"claude"}, dir)

	if _, err := os.Stat(filepath.Join(dir, "mdlsource", "README.md")); err != nil {
		t.Errorf("mdlsource/README.md not created: %v", err)
	}
	if readToolingStamp(dir) == nil {
		t.Errorf("%s not written", toolingStampRel)
	}
	for _, doc := range []string{"CLAUDE.md", "AGENTS.md"} {
		got := mustRead(t, filepath.Join(dir, doc))
		if !strings.HasPrefix(got, mxcliSectionBeginPrefix) || !strings.Contains(got, mxcliSectionEnd) {
			t.Errorf("%s is not wrapped in the mxcli markers", doc)
		}
	}

	path := filepath.Join(dir, "CLAUDE.md")
	notes := "\n## Project notes\n\nInvoices are immutable once sent.\n"
	mustWrite(t, path, mustRead(t, path)+notes)
	readme := filepath.Join(dir, "mdlsource", "README.md")
	mustWrite(t, readme, "our own readme\n")

	runInit(t, []string{"claude"}, dir)
	if got := mustRead(t, path); !strings.HasSuffix(got, notes) {
		t.Errorf("re-running init dropped the project notes:\n%s", got)
	}
	if got := mustRead(t, readme); got != "our own readme\n" {
		t.Error("re-running init overwrote mdlsource/README.md")
	}
}
