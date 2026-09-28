// SPDX-License-Identifier: Apache-2.0

// Package conformance is the canonical-form gate over the MDL the project
// teaches (ako/mxcli#756, PROPOSAL_mdl_beta_syntax_freeze.md plan item 1.4).
//
// The docs are where people and agents copy MDL from, so a deprecated spelling
// left in them is re-taught faster than `fmt --upgrade` can remove it from
// scripts. The gate parses every MDL block in the sources it is given — the
// `mxcli syntax` examples, the user-facing skills, docs-site, the quick
// reference and mdl-examples — and reports two classes of finding per source:
//
//   - a registered deprecated spelling (MDL-DEPRnnn), exactly what `mxcli check
//     --deprecations=error` fails on; every use is counted;
//   - a block that parses in no context at all ("syntax"). Docs legitimately
//     hold templates (`<Name>`, `...`), so these are counted per source rather
//     than treated as a defect each, and the count may not grow.
//
// What the gate tolerates is an explicit allowlist of per-source, per-class
// ceilings. It fails in both directions: a count above its ceiling (or one with
// no entry) is a regression, and a count below it is an entry that must be
// lowered, so the list can only shrink. Lowering is mechanical — Shrink never
// raises or adds an entry — which is what lets a docs change that lands after
// the list was measured fix the list in one command.
package conformance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ClassSyntax is the class of a block that parses in no known context.
const ClassSyntax = "syntax"

// Unit is one piece of MDL to check.
type Unit struct {
	// Source is the allowlist key: a repository-relative path with forward
	// slashes, or `syntax:<topic>` for an `mxcli syntax` entry.
	Source string
	// Line is the 1-based line in Source where the unit's text starts.
	Line int
	// Text is the MDL.
	Text string
	// Script says Text is a whole script, parsed as it stands. Otherwise it is
	// a documentation block, which may be a fragment and is tried in each
	// context a fragment is written for (Contexts).
	Script bool
	// Lenient says Text may not be MDL at all (an untagged markdown fence): its
	// deprecated spellings are reported, but not failing to parse.
	Lenient bool
}

// Finding is one non-conforming use.
type Finding struct {
	Source string
	// Line is the 1-based line in Source, as near as the unit allows.
	Line int
	// Class is ClassSyntax or a registry code (MDL-DEPRnnn).
	Class string
	// Detail is the parse error or the deprecated spelling's subject.
	Detail string
}

func (f Finding) String() string {
	if f.Detail == "" {
		return fmt.Sprintf("%s:%d: %s", f.Source, f.Line, f.Class)
	}
	return fmt.Sprintf("%s:%d: %s: %s", f.Source, f.Line, f.Class, f.Detail)
}

// Context is a construct a documentation fragment can be written for. Wrap
// places the fragment where that construct is legal; Prefix is the number of
// lines Wrap puts before it, to map a line back to the source.
type Context struct {
	Name   string
	Wrap   func(string) string
	Prefix int
}

// Contexts are tried in order; the first that parses decides the findings.
// They are the shapes `mxcli syntax` examples come in (TestExamplesParse in
// cmd/mxcli/syntax): a whole statement, a microflow activity, a page widget, a
// workflow activity and an XPath clause hung off a retrieve.
var Contexts = []Context{
	{"statement", func(s string) string { return s }, 0},
	{"microflow activity", func(s string) string {
		return "create microflow Conformance.Probe ()\nbegin\n" + s + "\nend;"
	}, 2},
	{"page widget", func(s string) string {
		return "create page Conformance.Probe (Title: 'Probe', Layout: Atlas_Core.Atlas_Default) {\n" + s + "\n};"
	}, 1},
	{"workflow activity", func(s string) string {
		return "create workflow Conformance.Probe parameter $Ctx: Conformance.Ctx\nbegin\n" + s + "\nend workflow;"
	}, 2},
	{"retrieve clause", func(s string) string {
		return "create microflow Conformance.Probe ()\nbegin\nretrieve $Probe from Conformance.Entity\n" + s + ";\nend;"
	}, 3},
}

// Check returns the findings for one unit.
func Check(u Unit) []Finding {
	if u.Script {
		return checkScript(u)
	}
	findings := checkBlock(u)
	if !u.Lenient {
		return findings
	}
	kept := findings[:0]
	for _, f := range findings {
		if f.Class != ClassSyntax {
			kept = append(kept, f)
		}
	}
	return kept
}

func checkScript(u Unit) []Finding {
	prog, errs := build(u.Text)
	if len(errs) > 0 {
		return []Finding{{Source: u.Source, Line: u.Line, Class: ClassSyntax, Detail: firstLine(errs[0].Error())}}
	}
	return deprecationFindings(u, prog.Deprecations, 0)
}

// checkBlock tries the whole block in each context; when none parses, the block
// is taken to be several independent snippets separated by blank lines (the
// usual shape of an example: a statement, then a variant) and each is checked
// on its own, so one template in a block does not hide a deprecated spelling
// in the statement next to it.
func checkBlock(u Unit) []Finding {
	text := stripNonMDL(u.Text)
	if strings.TrimSpace(stripComments(text)) == "" {
		return nil
	}
	if deps, prefix, ok := parseInSomeContext(text); ok {
		return deprecationFindings(u, deps, prefix)
	}
	chunks := splitChunks(text)
	if len(chunks) <= 1 {
		return []Finding{syntaxFinding(u, text)}
	}
	var out []Finding
	for _, c := range chunks {
		cu := u
		cu.Line = u.Line + c.offset
		if deps, prefix, ok := parseInSomeContext(c.text); ok {
			out = append(out, deprecationFindings(cu, deps, prefix)...)
			continue
		}
		out = append(out, syntaxFinding(cu, c.text))
	}
	return out
}

// perLine marks deprecations whose Line is already relative to the block.
const perLine = -1

// parseInSomeContext reports the deprecations of the first context the text
// parses in, and that context's line prefix.
func parseInSomeContext(text string) ([]ast.DeprecatedSpelling, int, bool) {
	for _, c := range Contexts {
		if prog, errs := build(c.Wrap(text)); len(errs) == 0 {
			return prog.Deprecations, c.Prefix, true
		}
	}
	// A block may list alternative clauses one per line (the XPath function
	// reference does): accept it when every line parses on its own.
	lines := nonEmptyLines(text)
	if len(lines) < 2 {
		return nil, 0, false
	}
	var deps []ast.DeprecatedSpelling
	for _, l := range lines {
		d, _, ok := parseInSomeContext(l.text)
		if !ok {
			return nil, 0, false
		}
		for _, x := range d {
			x.Line = l.index + 1
			deps = append(deps, x)
		}
	}
	return deps, perLine, true
}

func deprecationFindings(u Unit, deps []ast.DeprecatedSpelling, prefix int) []Finding {
	out := make([]Finding, 0, len(deps))
	for _, d := range deps {
		line := u.Line
		if d.Line > 0 {
			if prefix == perLine {
				line = u.Line + d.Line - 1
			} else {
				line = u.Line + d.Line - 1 - prefix
			}
		}
		if line < u.Line {
			line = u.Line
		}
		out = append(out, Finding{Source: u.Source, Line: line, Class: d.Code, Detail: d.Subject})
	}
	return out
}

func syntaxFinding(u Unit, text string) Finding {
	_, errs := build(text)
	detail := "parses in no context"
	if len(errs) > 0 {
		detail = firstLine(errs[0].Error())
	}
	return Finding{Source: u.Source, Line: u.Line, Class: ClassSyntax, Detail: detail}
}

// build is visitor.Build, with a crash reported as a parse error. The visitor
// is written for input that parsed; a few of its exit handlers dereference a
// token that error recovery left out, and one malformed doc block must not take
// down the whole gate.
func build(src string) (prog *ast.Program, errs []error) {
	defer func() {
		if r := recover(); r != nil {
			prog, errs = nil, []error{fmt.Errorf("the parser crashed on this input: %v", r)}
		}
	}()
	return visitor.Build(src)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type chunk struct {
	text   string
	offset int // lines from the start of the block
}

// splitChunks splits text on blank lines, dropping comment-only chunks.
func splitChunks(text string) []chunk {
	var out []chunk
	lines := strings.Split(text, "\n")
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		body := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(stripComments(body)) != "" {
			out = append(out, chunk{text: body, offset: start})
		}
		start = -1
	}
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(lines))
	return out
}

type indexedLine struct {
	text  string
	index int
}

func nonEmptyLines(s string) []indexedLine {
	var out []indexedLine
	for i, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		out = append(out, indexedLine{text: t, index: i})
	}
	return out
}

// stripComments drops `--` line comments, to tell a comment-only chunk.
func stripComments(s string) string {
	var kept []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "--") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

// stripNonMDL blanks the lines of a documentation block that are not MDL: a
// shell command shown next to the statement it runs, MDL's lone `/` separator
// and the `@test` annotations of a test file. Lines are blanked rather than
// removed, so line numbers still point into the source.
func stripNonMDL(s string) string {
	lines := strings.Split(s, "\n")
	inTestDoc := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case inTestDoc:
			if strings.Contains(t, "*/") {
				inTestDoc = false
			}
			lines[i] = ""
		case strings.HasPrefix(t, "/**") && (strings.Contains(t, "@test") || strings.Contains(t, "@expect")):
			inTestDoc = !strings.Contains(t, "*/")
			lines[i] = ""
		case strings.HasPrefix(t, "mxcli ") || strings.HasPrefix(t, "./bin/mxcli ") || strings.HasPrefix(t, "$ "):
			lines[i] = ""
		case t == "/":
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// Key names one allowlist ceiling.
type Key struct {
	Source string
	Class  string
}

// Tally counts findings per source and class.
type Tally map[Key]int

// Count tallies findings.
func Count(findings []Finding) Tally {
	t := Tally{}
	for _, f := range findings {
		t[Key{f.Source, f.Class}]++
	}
	return t
}

// Drift is how measured counts compare with the allowlist.
type Drift struct {
	// Grown are the keys measured above their ceiling, or with no entry.
	Grown []Key
	// Shrinkable are the keys measured below their ceiling, zero included.
	Shrinkable []Key
}

// Compare compares measured counts with the allowlist.
func Compare(measured, allowed Tally) Drift {
	var d Drift
	for k, n := range measured {
		if n > allowed[k] {
			d.Grown = append(d.Grown, k)
		}
	}
	for k, n := range allowed {
		if measured[k] < n {
			d.Shrinkable = append(d.Shrinkable, k)
		}
	}
	SortKeys(d.Grown)
	SortKeys(d.Shrinkable)
	return d
}

// Shrink returns the allowlist lowered to what was measured. It never raises a
// ceiling and never adds a key, so it cannot hide a regression.
func Shrink(measured, allowed Tally) Tally {
	out := Tally{}
	for k, n := range allowed {
		if m := measured[k]; m < n {
			n = m
		}
		if n > 0 {
			out[k] = n
		}
	}
	return out
}

// SortKeys orders keys by source, then class.
func SortKeys(ks []Key) {
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].Source != ks[j].Source {
			return ks[i].Source < ks[j].Source
		}
		return ks[i].Class < ks[j].Class
	})
}
