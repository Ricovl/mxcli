// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/conformance"
)

// skillRoots are what a project receives from `mxcli init`: the user-facing
// skills (unpacked to .ai-context/skills and .claude/skills), the skill packs,
// and the slash commands. The contributor skills next to them quote old and
// wrong spellings on purpose and are not taught to projects.
var skillRoots = []string{
	".claude/skills/mendix",
	".claude/skills/packs",
	".claude/commands/mendix",
}

// TestSkillsTeachMDL1 is decision 4 on ako/mxcli#714: every complete script a
// skill shows starts with `mdl 1;`, and every block — script, command or
// fragment — is valid mdl 1 and means under it what it says. There is no
// allowlist: a finding is fixed with `mxcli fmt --upgrade --header` on the
// block, or by fencing a block that is not MDL (a counterexample, a template)
// as ```text.
func TestSkillsTeachMDL1(t *testing.T) {
	units := 0
	var findings []conformance.Finding
	for _, root := range skillRoots {
		for _, p := range walk(t, root, ".md") {
			if testrunner.IsTestFile(p) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(repoRoot, p))
			if err != nil {
				t.Fatal(err)
			}
			for _, u := range conformance.MarkdownUnits(p, string(b)) {
				units++
				findings = append(findings, conformance.CheckMDL1(u)...)
			}
		}
	}
	// A pack's .mdl files are scripts, checked after the placeholder
	// substitution that happens on install (as the canonical-form gate does).
	for _, p := range walk(t, ".claude/skills/packs", ".mdl") {
		b, err := os.ReadFile(filepath.Join(repoRoot, p))
		if err != nil {
			t.Fatal(err)
		}
		src := strings.NewReplacer("{{MODULE_PATH}}", "mymodule", "{{MODULE}}", "MyModule",
			"{{NAMESPACE_PATH}}", "acme", "{{NAMESPACE}}", "acme").Replace(string(b))
		units++
		findings = append(findings, conformance.CheckMDL1(conformance.Unit{Source: p, Line: 1, Text: src, Script: true})...)
	}
	// Positive control: a walk that read nothing would pass.
	if units < 1000 {
		t.Fatalf("only %d units read from %v — the walk is broken", units, skillRoots)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
	t.Logf("%d units checked", units)
}
