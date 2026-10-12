// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestExamplesParse is the anti-drift guard for `mxcli syntax`.
//
// The registry is hand-maintained while the grammar moves underneath it, and
// nothing tied the two together: the other tests here check structure (fields
// populated, aliases resolve, see-also targets exist) rather than whether the
// documented MDL is real. So entries went stale silently — and the registry is
// the first place an agent looks, so a gap here becomes a wrong workaround in a
// generated app rather than a puzzled user.
//
// Each Example block is also held to `check --deprecations=error` in every
// context it parses in: the reference an agent copies from must not teach a
// spelling `check` warns on (MDL-DEPRnnn). Syntax fields are held to the same
// registry by TestSyntaxShowsNoDeprecatedSpelling.
//
// Only Example is parsed. Syntax fields carry metasyntax (`GET|POST`,
// `[OR MODIFY]`, `<statements>`) and are not meant to parse.
//
// Examples come in several legitimate shapes, so each block is tried in turn as
// a whole statement, a microflow activity, a page widget, a workflow activity,
// and a retrieve clause. A failure means the block parses as none of them — not
// that it failed the first attempt.
func TestExamplesParse(t *testing.T) {
	for _, f := range syntax.All() {
		f := f
		t.Run(f.Path, func(t *testing.T) {
			// Examples routinely show several independent snippets separated by
			// blank lines — a statement, then a variant, then a related clause.
			// Checking each block on its own keeps the docs readable while still
			// holding every snippet to "this really parses".
			blocks := mdlBlocks(f.Example)
			// TestFeatureFieldsPopulated already rejects an empty Example, but an
			// example made entirely of comments would yield no blocks and quietly
			// opt out of this guard. Keep that a failure everywhere except the
			// handful of prose topics below, so the opt-out cannot spread by
			// accident.
			if len(blocks) == 0 {
				if nonStatementTopics[f.Path] {
					t.Skipf("example is deliberately not MDL statements — see nonStatementTopics")
				}
				t.Fatalf("Example contains no MDL to check:\n%s", f.Example)
			}
			for _, block := range blocks {
				if hosts := parsingHosts(block); len(hosts) > 0 {
					t.Logf("block parses as %s", hosts[0].context)
					for _, h := range hosts {
						for _, v := range deprecationErrors(h.source) {
							t.Errorf("example block uses a deprecated spelling (as %s): %s: %s\n--- block ---\n%s",
								h.context, v.RuleID, v.Message, block)
						}
					}
					continue
				}
				_, errs := visitor.Build(block)
				var first any = "unknown"
				if len(errs) > 0 {
					first = errs[0]
				}
				t.Errorf("example block parses as no known construct "+
					"(statement / microflow activity / page widget / workflow activity).\n"+
					"top-level error: %v\n--- block ---\n%s", first, block)
			}
		})
	}
}

// deprecationErrors is what `mxcli check --deprecations=error` refuses in src:
// every MDL-DEPRnnn the validator reports, promoted to an error. Other
// violations are left alone — the probe wrappers declare nothing, so a
// reference or variable check would fail on the wrapper rather than on the
// documented spelling.
func deprecationErrors(src string) []linter.Violation {
	prog, errs := visitor.Build(src)
	if len(errs) > 0 || prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, v := range executor.ApplyDeprecationPolicy(executor.ValidateProgram(prog, ""), deprecation.Error) {
		if deprecation.IsDeprecationCode(v.RuleID) && v.Severity == linter.SeverityError {
			out = append(out, v)
		}
	}
	return out
}

// nonStatementTopics are the entries whose Example is legitimately not made of
// MDL statements — the troubleshooting topics pair an error message with its fix
// as prose, and `oql` documents a CLI invocation whose quoted argument is OQL,
// not MDL.
//
// Keep this list short and justified: an entry added here stops being checked at
// all, which is the one way a stale example could still slip through.
var nonStatementTopics = map[string]bool{
	"errors":           true, // error/fix prose
	"errors.execution": true, // error/fix prose
	"errors.reference": true, // error/fix prose
	"errors.syntax":    true, // error/fix prose
	"oql":              true, // `mxcli oql "<OQL>"` — a shell command, and OQL is not MDL
}

// mdlBlocks splits an Example into blank-line-separated blocks, dropping shell
// commands, comment-only blocks, and leading prose.
func mdlBlocks(example string) []string {
	var blocks []string
	for _, raw := range strings.Split(stripNonMDL(example), "\n\n") {
		block := strings.TrimSpace(stripNonMDL(raw))
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// exampleContext is one host an Example block may be written for: the block is
// wrapped into a complete statement and parsed.
type exampleContext struct {
	name string
	wrap func(string) string
}

// pageWidget hosts a widget fragment on a probe page.
func pageWidget(s string) string {
	return "CREATE PAGE SyntaxDoc.Probe (Title: 'Probe', Layout: 'Atlas_Core.Atlas_Default') {\n" + s + "\n};"
}

var exampleContexts = []exampleContext{
	{"statement", func(s string) string { return s }},
	{"microflow activity", func(s string) string {
		return "CREATE MICROFLOW SyntaxDoc.Probe ()\nBEGIN\n" + s + "\nEND;"
	}},
	{"page widget", pageWidget},
	// Some page rules fire only inside their container — a data grid column's
	// name, for one, is recorded as unstored (MDL-DEPR005) because the column
	// sits in a grid. A fragment documented on its own is therefore also hosted
	// in the containers it is written for, so those rules see it.
	{"data grid column", func(s string) string {
		return pageWidget("DATAGRID dgProbe (DataSource: DATABASE SyntaxDoc.Entity) {\n" + s + "\n}")
	}},
	{"data view widget", func(s string) string {
		return pageWidget("DATAVIEW dvProbe (DataSource: DATABASE SyntaxDoc.Entity) {\n" + s + "\n}")
	}},
	{"workflow activity", func(s string) string {
		return "CREATE WORKFLOW SyntaxDoc.Probe PARAMETER $Ctx: SyntaxDoc.Ctx\nBEGIN\n" + s + "\nEND WORKFLOW;"
	}},
	// XPath topics illustrate a constraint on its own (`WHERE [...]`), which
	// is a clause rather than a statement. Hanging it off a RETRIEVE checks
	// the constraint itself without forcing the docs to repeat a full
	// statement around every example.
	{"retrieve clause", func(s string) string {
		return "CREATE MICROFLOW SyntaxDoc.Probe ()\nBEGIN\n" +
			"RETRIEVE $Probe FROM SyntaxDoc.Entity\n" + s + ";\nEND;"
	}},
}

// parsesInSomeContext reports the first context the example parses in.
func parsesInSomeContext(example string) (string, bool) {
	hosts := parsingHosts(example)
	if len(hosts) == 0 {
		return "", false
	}
	return hosts[0].context, true
}

// parsedHost is an example block wrapped in a context it parses in.
type parsedHost struct {
	context string
	source  string
}

// parsingHosts returns every context the example parses in, with the wrapped
// source. Several can apply — a widget parses at page level and inside a data
// view — and each is a place a reader may paste it, so each is validated.
func parsingHosts(example string) []parsedHost {
	var out []parsedHost
	for _, c := range exampleContexts {
		src := c.wrap(example)
		if _, errs := visitor.Build(src); len(errs) == 0 {
			out = append(out, parsedHost{c.name, src})
		}
	}
	if len(out) > 0 {
		return out
	}
	// A block may list several alternative clauses one per line (the xpath
	// function reference does this). Accept it when every line is a valid clause
	// on its own — the block is a menu, not one statement.
	if lines := nonEmptyLines(example); len(lines) > 1 {
		var perLine []parsedHost
		for _, line := range lines {
			hosts := parsingHosts(line)
			if len(hosts) == 0 {
				return nil
			}
			for _, h := range hosts {
				perLine = append(perLine, parsedHost{"per-line clauses: " + h.context, h.source})
			}
		}
		return perLine
	}
	return nil
}

// stripTestAnnotations removes `/** … @test … */` blocks.
//
// DOC_COMMENT is a real token in MDL, not a hidden one, and the grammar admits
// it only ahead of a handful of declarations. The `@test`/`@expect` annotations
// in a .test.mdl file sit ahead of ordinary microflow statements instead, and
// are consumed by the test runner's own front end before the MDL is parsed.
// Removing them here keeps the statements underneath under test rather than
// exempting the whole entry.
func stripTestAnnotations(s string) string {
	for {
		start := strings.Index(s, "/**")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "*/")
		if end < 0 {
			break
		}
		end += start + len("*/")
		if !strings.Contains(s[start:end], "@test") && !strings.Contains(s[start:end], "@expect") {
			break
		}
		s = s[:start] + s[end:]
	}
	return s
}

// nonEmptyLines returns the block's lines with blanks and comments dropped.
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// stripNonMDL removes leading prose and any shell-command lines. Some entries
// deliberately show the `mxcli …` equivalent alongside the MDL; those lines are
// documentation, not statements to validate.
func stripNonMDL(s string) string {
	s = stripTestAnnotations(s)
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "mxcli ") {
			continue
		}
		// A lone `/` is MDL's statement separator (used between blocks in
		// .test.mdl files). It carries no syntax to validate and would otherwise
		// land inside a wrapper's body.
		if t == "/" {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")

	// Drop a leading run of comment/blank lines so a prose preamble does not
	// decide the parse context.
	lines := strings.Split(out, "\n")
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "--") {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}
