// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/conformance"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// repoRoot is the repository, seen from this package.
const repoRoot = "../.."

// allowlistPath is the gate's allowlist.
const allowlistPath = "allowlist.txt"

// aliasExamplesDir holds the example scripts that deliberately use deprecated
// spellings: the corpus `fmt --upgrade` is proven on (mdl/upgrade's examples
// test and mdl/roundtrip's execute-both test). They are the one part of
// mdl-examples the gate does not hold to the canonical form.
const aliasExamplesDir = "mdl-examples/deprecated-aliases"

// keepsMdl0 are the example scripts outside the alias corpus that stay
// headerless on purpose, because mdl 0 is what they test. They are held to the
// canonical spellings but not to mdl 1 (no header, no MDL-V1-* refusals).
// The list may only shrink; TestKeepsMdl0StillNeedsIt drops a stale entry.
var keepsMdl0 = map[string]string{
	// ADR-0011: a headerless script keeps the pre-#750 quoted expressions.
	// Its mdl 1 twin is 750-dynamicclasses-legacy-quoted-expression.fail.mdl.
	"mdl-examples/bug-tests/836-quoted-expression-mdl0.mdl": "a headerless script keeps quoted expressions",
	// Session commands (R7): `lint` and `help <topic>` are REPL commands under
	// mdl 1, refused in a script. These scripts test the commands, so they stay
	// mdl 0 (mdl/upgrade's keepsItsVersion lists them for the same reason).
	"mdl-examples/bug-tests/904-lint-rules-discovery.mdl":    "runs `lint`, a session command",
	"mdl-examples/bug-tests/syntax-1025-topic-drilldown.mdl": "runs `help <topic>`, a session command",
	"mdl-examples/doctype-tests/20-help-examples.mdl":        "runs `help <topic>`, a session command",
}

// markdownRoots are the documents the gate reads MDL fences from. The skills
// are the user-facing ones — .claude/skills/mendix is what `make sync-skills`
// copies into cmd/mxcli/skills and every project, packs ship with the binary —
// not the contributor skills, which quote old and wrong spellings on purpose.
//
// TODO(feature/skills-mdl1): the skills move to mdl 1 on that branch (wave 10b,
// ako/mxcli#714 decision 4). Their `header` and MDL-V1-* lines in allowlist.txt
// were measured when this gate started parsing everything as mdl 1, before the
// skills had been upgraded; that branch's `make conformance-shrink` drops them.
var markdownRoots = []string{
	".claude/skills/mendix",
	".claude/skills/packs",
	"docs-site/src",
	"docs/01-project/MDL_QUICK_REFERENCE.md",
	"mdl-examples",
}

// TestConformanceGate is plan item 1.4 of PROPOSAL_mdl_beta_syntax_freeze.md:
// everything the project teaches is parsed as mdl 1, with deprecations, mdl 1
// refusals and a complete script without the header as errors (ako/mxcli#714
// decision 1), against an allowlist that may only shrink. `make check-conformance` runs it;
// MXCLI_CONFORMANCE_SHRINK=1 (`make conformance-shrink`) lowers the allowlist
// to what it measures instead of failing on a count that fell.
func TestConformanceGate(t *testing.T) {
	units := gatherUnits(t)
	var findings []conformance.Finding
	for _, u := range units {
		findings = append(findings, conformance.Check(u)...)
	}
	measured := conformance.Count(findings)

	allowed := readAllowlist(t)
	drift := conformance.Compare(measured, allowed)

	bySource := map[conformance.Key][]conformance.Finding{}
	for _, f := range findings {
		k := conformance.Key{Source: f.Source, Class: f.Class}
		bySource[k] = append(bySource[k], f)
	}
	for _, k := range drift.Grown {
		t.Errorf("%s in %s: %d found, the allowlist tolerates %d. Write the canonical form "+
			"(`mxcli fmt --upgrade --header` rewrites a deprecated spelling and an mdl 0 construct and adds "+
			"the `mdl 1;` header; `mxcli check --deprecations=error` shows a deprecation), or fence a block "+
			"that is not MDL as ```text:\n%s",
			k.Class, k.Source, measured[k], allowed[k], indent(bySource[k]))
	}

	if len(drift.Shrinkable) > 0 {
		if os.Getenv("MXCLI_CONFORMANCE_SHRINK") != "" {
			out := conformance.FormatAllowlist(conformance.Shrink(measured, allowed))
			if err := os.WriteFile(allowlistPath, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("lowered %d allowlist entr(y/ies) in mdl/conformance/%s", len(drift.Shrinkable), allowlistPath)
		} else {
			var lines []string
			for _, k := range drift.Shrinkable {
				lines = append(lines, fmt.Sprintf("  %s in %s: allowlist says %d, measured %d", k.Class, k.Source, allowed[k], measured[k]))
			}
			t.Errorf("the allowlist can shrink — it may only shrink, so lower it with `make conformance-shrink`:\n%s",
				strings.Join(lines, "\n"))
		}
	}

	total := 0
	for _, n := range measured {
		total += n
	}
	t.Logf("%d units checked, %d findings tolerated by the allowlist over %d source/class pairs", len(units), total, len(measured))
}

// TestConformanceGateReadsEverySource is the positive control: a gate that read
// nothing would pass. Each kind of source must contribute units.
func TestConformanceGateReadsEverySource(t *testing.T) {
	counts := map[string]int{}
	for _, u := range gatherUnits(t) {
		switch {
		case strings.HasPrefix(u.Source, "syntax:"):
			counts["mxcli syntax"]++
		case strings.HasPrefix(u.Source, "mdl-examples/"):
			counts["mdl-examples"]++
		default:
			counts[strings.SplitN(u.Source, "/", 3)[0]+"/"+strings.SplitN(u.Source, "/", 3)[1]]++
		}
	}
	for kind, min := range map[string]int{
		"mxcli syntax":    150,
		"mdl-examples":    500,
		".claude/skills":  300,
		"docs-site/src":   500,
		"docs/01-project": 30,
	} {
		if counts[kind] < min {
			t.Errorf("%s contributes %d units, expected at least %d — the walk is broken", kind, counts[kind], min)
		}
	}
}

// TestAliasExamplesExerciseAliases: every script kept in the old-spelling
// corpus parses and uses at least one deprecated spelling or one construct the
// `mdl 1;` header changes (an MDL-V1-* note other than a statement terminator,
// which every headerless script has). One that does neither has lost its
// reason to be exempt from the gate.
func TestAliasExamplesExerciseAliases(t *testing.T) {
	paths := walk(t, aliasExamplesDir, ".mdl")
	if len(paths) == 0 {
		t.Fatalf("no scripts under %s", aliasExamplesDir)
	}
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(repoRoot, p))
		if err != nil {
			t.Fatal(err)
		}
		prog, errs := visitor.Build(string(b))
		if len(errs) > 0 {
			t.Errorf("%s does not parse: %v", p, errs[0])
			continue
		}
		if len(prog.Deprecations) == 0 && len(gatedConstructs(prog)) == 0 {
			t.Errorf("%s uses no deprecated spelling and no construct the mdl 1 header changes: "+
				"move it back to the canonical examples", p)
		}
	}
}

// TestKeepsMdl0StillNeedsIt: every keepsMdl0 script exists, is still
// headerless, and still uses a construct the header changes — otherwise it
// can take the header and leave the list.
func TestKeepsMdl0StillNeedsIt(t *testing.T) {
	for p := range keepsMdl0 {
		b, err := os.ReadFile(filepath.Join(repoRoot, p))
		if err != nil {
			t.Errorf("%s: %v — drop it from keepsMdl0", p, err)
			continue
		}
		prog, errs := visitor.Build(string(b))
		if len(errs) > 0 {
			t.Errorf("%s does not parse: %v", p, errs[0])
			continue
		}
		if prog.LanguageHeaderLine != 0 {
			t.Errorf("%s has a language header: drop it from keepsMdl0", p)
		}
		if len(gatedConstructs(prog)) == 0 {
			t.Errorf("%s uses no construct the mdl 1 header changes: upgrade it with "+
				"`mxcli fmt --upgrade --header` and drop it from keepsMdl0", p)
		}
	}
}

// gatedConstructs are the program's MDL-V1-* notes other than the statement
// terminators.
func gatedConstructs(prog *ast.Program) []string {
	var codes []string
	for _, n := range prog.LanguageNotes {
		if n.Code != "MDL-V1-SEMI" && n.Code != "MDL-V1-SLASH" {
			codes = append(codes, n.Code)
		}
	}
	return codes
}

func readAllowlist(t *testing.T) conformance.Tally {
	t.Helper()
	f, err := os.Open(allowlistPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	allowed, err := conformance.ParseAllowlist(f)
	if err != nil {
		t.Fatal(err)
	}
	return allowed
}

func gatherUnits(t *testing.T) []conformance.Unit {
	t.Helper()
	var units []conformance.Unit

	for _, f := range syntax.All() {
		units = append(units, conformance.Unit{Source: "syntax:" + f.Path, Line: 1, Text: f.Example})
	}

	for _, root := range markdownRoots {
		for _, p := range walk(t, root, ".md") {
			if strings.HasPrefix(p, aliasExamplesDir+"/") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(repoRoot, p))
			if err != nil {
				t.Fatal(err)
			}
			if testrunner.IsTestFile(p) {
				units = append(units, testFileUnit(t, p, string(b)))
				continue
			}
			units = append(units, conformance.MarkdownUnits(p, string(b))...)
		}
	}

	for _, p := range walk(t, "mdl-examples", ".mdl") {
		if strings.HasPrefix(p, aliasExamplesDir+"/") || isNegativeTest(p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(repoRoot, p))
		if err != nil {
			t.Fatal(err)
		}
		if testrunner.IsTestFile(p) {
			units = append(units, testFileUnit(t, p, string(b)))
			continue
		}
		units = append(units, conformance.Unit{Source: p, Line: 1, Text: string(b), Script: true, KeepsMdl0: keepsMdl0[p] != ""})
	}

	// A skill pack ships real .mdl files. They carry {{MODULE}}-style
	// placeholders, substituted when the pack is installed; they are checked in
	// the form anyone runs (as `make check-skill-mdl` does).
	for _, p := range walk(t, ".claude/skills/packs", ".mdl") {
		b, err := os.ReadFile(filepath.Join(repoRoot, p))
		if err != nil {
			t.Fatal(err)
		}
		src := strings.NewReplacer("{{MODULE_PATH}}", "mymodule", "{{MODULE}}", "MyModule",
			"{{NAMESPACE_PATH}}", "acme", "{{NAMESPACE}}", "acme").Replace(string(b))
		units = append(units, conformance.Unit{Source: p, Line: 1, Text: src, Script: true})
	}
	return units
}

// testFileUnit renders a .test.mdl / .test.md file as the microflows it
// becomes, on the source's own lines, the way `mxcli check` reads it.
func testFileUnit(t *testing.T, path, content string) conformance.Unit {
	t.Helper()
	checked, err := testrunner.CheckSource(content, path)
	if err != nil {
		// Not a well-formed test file: parse it as it stands, which fails and
		// is counted as syntax.
		return conformance.Unit{Source: path, Line: 1, Text: content, Script: true, KeepsMdl0: true}
	}
	return conformance.Unit{Source: path, Line: 1, Text: checked.MDL, Script: true, KeepsMdl0: true}
}

// isNegativeTest reports a script that is meant to fail `mxcli check`.
func isNegativeTest(p string) bool {
	return strings.HasSuffix(p, ".fail.mdl") || strings.HasSuffix(p, ".fail.test.mdl")
}

// walk returns the repository-relative paths under root (a directory or a
// file) that end in ext, sorted.
func walk(t *testing.T, root, ext string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join(repoRoot, root), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ext) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func indent(fs []conformance.Finding) string {
	var b strings.Builder
	for i, f := range fs {
		if i == 20 {
			fmt.Fprintf(&b, "    … and %d more\n", len(fs)-i)
			break
		}
		fmt.Fprintf(&b, "    %s\n", f)
	}
	return b.String()
}
