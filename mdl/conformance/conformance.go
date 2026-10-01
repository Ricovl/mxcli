// SPDX-License-Identifier: Apache-2.0

// Package conformance is the canonical-form gate over the MDL the project
// teaches (ako/mxcli#756, PROPOSAL_mdl_beta_syntax_freeze.md plan item 1.4).
//
// The docs are where people and agents copy MDL from, so a deprecated spelling
// left in them is re-taught faster than `fmt --upgrade` can remove it from
// scripts. The gate parses every MDL block in the sources it is given — the
// `mxcli syntax` examples, the user-facing skills, docs-site, the quick
// reference and mdl-examples — as mdl 1 (decision 1 on ako/mxcli#714: only
// mdl 1 is documented), and reports these classes of finding per source:
//
//   - a registered deprecated spelling (MDL-DEPRnnn), exactly what `mxcli check
//     --deprecations=error` fails on; every use is counted;
//   - a construct the `mdl 1;` header refuses, under the rule that names it
//     (MDL-V1-*: a `/` terminator, a missing `;`, a reassignment without
//     `set`, …), or ClassMdl1 when no rule names it;
//   - a complete script without the `mdl 1;` header (ClassHeader). A fragment
//     — one statement, a microflow activity, a widget, a run of queries, REPL
//     input — carries none;
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
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ClassSyntax is the class of a block that parses in no known context.
const ClassSyntax = "syntax"

// ClassHeader is the class of a complete script that does not start with the
// `mdl 1;` language header (decision 1 on ako/mxcli#714: every complete-script
// example is written in mdl 1, and says so).
const ClassHeader = "header"

// ClassMdl1 is the class of MDL that parses headerless but is refused under
// `mdl 1;` without a recorded MDL-V1-* note explaining why. It should not occur
// — every new rejection goes through a langver.Change — and is counted rather
// than lost if it does.
const ClassMdl1 = "mdl1"

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
	// KeepsMdl0 says Text is a whole script that stays mdl 0: a test file,
	// whose `/` separators and bodies the runner reads as mdl 0 until it reads
	// a header (ako/mxcli#847), or an example kept headerless because mdl 0 is
	// what it tests. Only its deprecated spellings are reported.
	KeepsMdl0 bool
	// Lenient says Text may not be MDL at all (an untagged markdown fence): its
	// deprecated spellings are reported, but not failing to parse.
	Lenient bool
}

// Finding is one non-conforming use.
type Finding struct {
	Source string
	// Line is the 1-based line in Source, as near as the unit allows.
	Line int
	// Class is ClassSyntax, ClassHeader, ClassMdl1, a deprecated spelling's
	// registry code (MDL-DEPRnnn) or a language change's rule ID (MDL-V1-*).
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

// sessionCode is the rule ID of a session command in a script (R7). A session
// command is legal REPL input under every version, so a documentation
// fragment showing one — a REPL transcript — is not held to it; a script is.
const sessionCode = "MDL-V1-SESSION"

// header is the language header every unit is parsed under.
var header = langver.Latest.String() + ";\n"

// Check returns the findings for one unit.
//
// Everything is held to mdl 1 (decision 1 on ako/mxcli#714): a unit is parsed
// with the `mdl 1;` header, in the first context it parses in, and what that
// parse refuses is reported. A refusal is classed by the construct's rule ID
// (MDL-V1-*), which the same text parsed headerless names, so a docs author
// sees `MDL-V1-SLASH` rather than a bare parse error; one no rule names is
// ClassMdl1. Deprecated spellings (MDL-DEPRnnn) are reported as before.
//
// A construct that only changes meaning under the header (`limit 1`, a call
// after `set`) is not a finding: the docs are written in mdl 1, where it has
// the mdl 1 meaning. Moving a text to mdl 1 is `fmt --upgrade --header`'s job,
// which rewrites such a construct so that it keeps the meaning it was written
// with.
//
// A complete script must start with the header (ClassHeader); a fragment
// carries none (IsCompleteScript says which is which).
func Check(u Unit) []Finding {
	text, hasHeader := stripHeader(u.Text)
	u.Text = text
	if u.Script || hasHeader {
		return checkScript(u, hasHeader)
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

func checkScript(u Unit, hasHeader bool) []Finding {
	if u.KeepsMdl0 {
		prog, errs := build(u.Text)
		if len(errs) > 0 {
			return []Finding{{Source: u.Source, Line: u.Line, Class: ClassSyntax, Detail: firstLine(errs[0].Error())}}
		}
		return deprecationFindings(u, prog.Deprecations, 0)
	}
	r, ok := parseIn(Contexts[0], u.Text)
	if !ok {
		_, errs := build(header + u.Text)
		return []Finding{{Source: u.Source, Line: u.Line, Class: ClassSyntax, Detail: firstLine(errs[0].Error())}}
	}
	out := r.findings(u, true)
	if !hasHeader {
		out = append(out, Finding{Source: u.Source, Line: u.Line, Class: ClassHeader,
			Detail: "a complete script starts with `mdl 1;` (`mxcli fmt --upgrade --header` adds it)"})
	}
	return out
}

// parsed is a text parsed in one context, under the header and headerless.
type parsed struct {
	ctx Context
	// v1 is the parse under the header, nil when it failed with v1Errs.
	v1     *ast.Program
	v1Errs []error
	// v0 is the headerless parse, set when v1 failed: its notes name the
	// constructs the header refused.
	v0 *ast.Program
}

// program is the parse that succeeded.
func (p parsed) program() *ast.Program {
	if p.v1 != nil {
		return p.v1
	}
	return p.v0
}

// parseIn parses text in one context. It succeeds when the text parses there
// under the header, or headerless — then the header's refusals are findings.
func parseIn(c Context, text string) (parsed, bool) {
	w := c.Wrap(text)
	if prog, errs := build(header + w); len(errs) == 0 {
		return parsed{ctx: c, v1: prog}, true
	} else if prog0, errs0 := build(w); len(errs0) == 0 {
		return parsed{ctx: c, v1Errs: errs, v0: prog0}, true
	}
	return parsed{}, false
}

// ParseInSomeContext parses a documentation fragment in the first context it
// parses in under the `mdl 1;` header, or failing that, the first it parses in
// headerless. It returns the headerless program, the context, and the wrapped
// text that was parsed.
func ParseInSomeContext(text string) (*ast.Program, Context, string, bool) {
	p, ok := parseInSomeContext(text)
	if !ok {
		return nil, Context{}, "", false
	}
	w := p.ctx.Wrap(text)
	prog, errs := build(w)
	if len(errs) > 0 {
		return nil, Context{}, "", false
	}
	return prog, p.ctx, w, true
}

func parseInSomeContext(text string) (parsed, bool) {
	for _, c := range Contexts {
		w := c.Wrap(text)
		if prog, errs := build(header + w); len(errs) == 0 {
			return parsed{ctx: c, v1: prog}, true
		}
	}
	for _, c := range Contexts {
		if p, ok := parseIn(c, text); ok {
			return p, true
		}
	}
	return parsed{}, false
}

// errLine is the line an error message starts with ("line 12:4 …").
var errLine = regexp.MustCompile(`^line (\d+)`)

// codeRe finds a rule ID or registry code in an error message.
var codeRe = regexp.MustCompile(`MDL-(?:V1-[A-Z0-9]+|DEPR\d+)`)

// findings are the unit's deprecated spellings and the header's refusals.
// script says the unit is a script, where a session command is a finding.
func (p parsed) findings(u Unit, script bool) []Finding {
	if p.v1 != nil {
		// Lines in the header-prefixed parse are one further down.
		return deprecationFindings(u, p.v1.Deprecations, p.ctx.Prefix+1)
	}
	out := deprecationFindings(u, p.v0.Deprecations, p.ctx.Prefix)
	notes := append([]ast.LanguageNote(nil), p.v0.LanguageNotes...)
	for _, e := range p.v1Errs {
		msg := firstLine(e.Error())
		line := 0
		if m := errLine.FindStringSubmatch(msg); m != nil {
			line, _ = strconv.Atoi(m[1])
			line-- // the header
		}
		var class string
		class, notes = takeNearestNote(notes, line)
		if class == "" {
			class = codeRe.FindString(msg)
		}
		if class == "" {
			class = ClassMdl1
		}
		if class == sessionCode && !script {
			continue
		}
		if strings.HasPrefix(class, "MDL-DEPR") {
			continue // refused under the header, and already counted as used
		}
		out = append(out, Finding{Source: u.Source, Line: mapLine(u, line, p.ctx.Prefix), Class: class, Detail: msg})
	}
	return out
}

// takeNearestNote takes the note nearest to line off notes and returns its
// rule ID. A refusal is reported where the parser noticed it (the `/` after a
// statement, the token after a missing `;`), the note where the construct is;
// each note explains one refusal, so `create module M` + `/` is one missing `;`
// and one `/`.
func takeNearestNote(notes []ast.LanguageNote, line int) (string, []ast.LanguageNote) {
	best, dist := -1, -1
	for i, n := range notes {
		d := n.Line - line
		if d < 0 {
			d = -d
		}
		if dist < 0 || d < dist {
			best, dist = i, d
		}
	}
	if best < 0 {
		return "", notes
	}
	code := notes[best].Code
	return code, append(notes[:best], notes[best+1:]...)
}

// mapLine maps a 1-based line of the wrapped text back to the source.
func mapLine(u Unit, line, prefix int) int {
	if line <= 0 {
		return u.Line
	}
	l := u.Line + line - 1 - prefix
	if l < u.Line {
		return u.Line
	}
	return l
}

// stripHeader blanks a leading `mdl <n>;` header line, keeping line numbers,
// and reports whether there was one. Only `--` comments and blank lines may
// precede it, as in langver.ScanHeader.
func stripHeader(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		if langver.IsHeaderLine(l) {
			lines[i] = ""
			return strings.Join(lines, "\n"), true
		}
		break
	}
	return s, false
}

// IsCompleteScript reports whether a block parsed as top-level statements is a
// complete script — one that is meant to be run as a file and so starts with
// the language header — rather than a fragment: it holds at least two
// statements, at least one of which writes the model, and no session command
// (that makes it REPL input, which takes no header). A single statement, or a
// run of queries (`show`, `describe`), is a fragment.
func IsCompleteScript(prog *ast.Program) bool {
	if prog == nil || len(prog.Statements) < 2 {
		return false
	}
	for _, n := range prog.LanguageNotes {
		if n.Code == sessionCode {
			return false
		}
	}
	for _, s := range prog.Statements {
		if writesModel(s) {
			return true
		}
	}
	return false
}

// modelVerbs are the statement type prefixes that write the model.
var modelVerbs = []string{"Create", "Alter", "Drop", "Grant", "Revoke", "Rename", "Move", "Update"}

func writesModel(s ast.Statement) bool {
	name := reflect.TypeOf(s).String()
	name = name[strings.LastIndexByte(name, '.')+1:]
	for _, v := range modelVerbs {
		if strings.HasPrefix(name, v) {
			return true
		}
	}
	return false
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
	if out, ok := findingsInSomeContext(u, text, true); ok {
		return out
	}
	chunks := splitChunks(text)
	if len(chunks) <= 1 {
		return []Finding{syntaxFinding(u, text)}
	}
	var out []Finding
	for _, c := range chunks {
		cu := u
		cu.Line = u.Line + c.offset
		if f, ok := findingsInSomeContext(cu, c.text, false); ok {
			out = append(out, f...)
			continue
		}
		out = append(out, syntaxFinding(cu, c.text))
	}
	return out
}

// findingsInSomeContext reports the findings of the first context the text
// parses in. whole says text is a whole documentation block, which may then
// be a complete script that must carry the header.
func findingsInSomeContext(u Unit, text string, whole bool) ([]Finding, bool) {
	if p, ok := parseInSomeContext(text); ok {
		out := p.findings(u, false)
		if whole && p.ctx.Prefix == 0 && IsCompleteScript(p.program()) {
			out = append(out, Finding{Source: u.Source, Line: u.Line, Class: ClassHeader,
				Detail: "a complete script example starts with `mdl 1;` (`mxcli fmt --upgrade --header` adds it)"})
		}
		return out, true
	}
	// A block may list alternative clauses one per line (the XPath function
	// reference does): accept it when every line parses on its own.
	lines := nonEmptyLines(text)
	if len(lines) < 2 {
		return nil, false
	}
	var out []Finding
	for _, l := range lines {
		p, ok := parseInSomeContext(l.text)
		if !ok {
			return nil, false
		}
		lu := u
		lu.Line = u.Line + l.index
		out = append(out, p.findings(lu, false)...)
	}
	return out, true
}

func deprecationFindings(u Unit, deps []ast.DeprecatedSpelling, prefix int) []Finding {
	out := make([]Finding, 0, len(deps))
	for _, d := range deps {
		out = append(out, Finding{Source: u.Source, Line: mapLine(u, d.Line, prefix), Class: d.Code, Detail: d.Subject})
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
// shell command shown next to the statement it runs and the `@test`
// annotations of a test file. MDL's lone `/` separator is not blanked: it is
// refused under mdl 1 (MDL-V1-SLASH). Lines are blanked rather than
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
