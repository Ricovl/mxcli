// SPDX-License-Identifier: Apache-2.0

// Package upgrade rewrites an MDL script to the canonical language: the engine
// behind `mxcli fmt --upgrade` (ADR-0011 decision 1, plan item 1.3 of
// PROPOSAL_mdl_beta_syntax_freeze.md).
//
// It does three kinds of rewrite, and nothing else:
//
//   - Every use of a deprecated spelling registered in mdl/deprecation becomes
//     its canonical spelling. A deprecated spelling is a respelling, so the
//     rewrite never changes meaning. The visitor records where each use is
//     (ast.Program.Deprecations); the entry's Rewrite says what replaces it.
//   - When the caller asks for the language header, every construct whose
//     meaning differs under the new version (ast.Program.LanguageNotes, one per
//     langver.Change) is rewritten to the spelling that keeps its OLD meaning
//     under the new header, and `mdl <n>;` is added. The rewrites live in
//     gatedRewriters (gated.go); the changes with none, in unrewritable.
//   - When the caller passes the project (Options.Commits), a bare `commit`
//     in a `create or modify` flow whose stored flow commits without events is
//     given that flag, so the upgraded script builds what is stored
//     (commit_events.go, ako/mxcli#873).
//
// A rewrite that is more than a keyword swap — a structural deprecation such as
// a list operation's call form, or a header-gated construct — is computed from
// the parse tree by the visitor that records the construct, as an ast.Fix of
// rune-offset edits (mdl/visitor/visitor_upgrade_fixes.go). An occurrence the
// visitor cannot rewrite carries the reason instead.
//
// Anything it cannot rewrite is reported, never guessed at: a deprecated use
// with no rewrite is left in place and listed in Result.Unrewritten, and a
// header-gated construct without one blocks the header (HeaderBlockedError),
// because adding the header over it would silently change the script's meaning.
//
// The rewrite works on the source text, not on the AST, so comments, layout
// and every spelling it does not own survive byte for byte. The output is
// re-parsed before it is returned: it must parse, record no deprecation the
// upgrade knows how to rewrite, and keep every statement.
//
// The proof that a rewrite is safe is the execute-both property test in
// mdl/roundtrip (upgrade_property_test.go): each mdl-examples script and its
// upgrade run on two copies of the PedApp fixture and must write the same
// model, compared as canonical BSON per unit.
package upgrade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Options says what Upgrade may do beyond the alias rewrites.
type Options struct {
	// AddHeader adds the `mdl <Latest>;` header to a headerless script, after
	// rewriting every construct whose meaning the header would change. While
	// mdl 1 is a preview this is opt-in (`fmt --upgrade --header`); at beta it
	// becomes the default (langver.Frozen is the single switch).
	AddHeader bool
	// Flows answers what the project's microflows and nanoflows return, for
	// the header-gated constructs whose meaning depends on it (`find(…)` over
	// a call's result, ako/mxcli#860). Nil when no project is given: such a
	// construct then blocks the header, as before.
	Flows FlowTypes
	// Commits answers what the project's stored flows commit, for the bare
	// `commit $X;` whose meaning #895 changed (ako/mxcli#873): where the
	// stored flow commits without events, the upgrade says so. Nil when no
	// project is given: each such flow is then reported in Notes instead.
	// It applies with and without the header — the change is not gated.
	Commits StoredCommits
}

// FlowTypes is the project a script runs against, as far as the upgrade needs
// it: whether a called flow returns a String. `fmt --upgrade -p app.mpr`
// passes one backed by the project (the flow builder's own lookup, so the two
// cannot disagree); tests pass a map.
type FlowTypes interface {
	// ReturnsString reports whether the microflow (the nanoflow when nanoflow
	// is set) named qualifiedName returns a String. found is false when the
	// project has no such flow.
	ReturnsString(nanoflow bool, qualifiedName string) (isString, found bool)
}

// DefaultOptions is what `fmt --upgrade` does when no header flag is given:
// add the header only once the version it names is frozen.
func DefaultOptions() Options { return Options{AddHeader: langver.Latest <= langver.Frozen} }

// Result is an upgraded script and what the upgrade could not do.
type Result struct {
	// Source is the upgraded script. It equals the input when nothing applied.
	Source string
	// Rewritten counts the deprecated uses rewritten, by registry code.
	Rewritten map[string]int
	// GatedRewritten counts the header-gated constructs rewritten, by code.
	GatedRewritten map[string]int
	// HeaderAdded reports whether the language header was added.
	HeaderAdded bool
	// Unrewritten lists the deprecated uses left in place because their
	// registry entry carries no rewrite.
	Unrewritten []ast.DeprecatedSpelling
	// CommitsPinned counts the bare commits given the stored flow's
	// `without events` (ako/mxcli#873).
	CommitsPinned int
	// Notes are what the upgrade left and the author should know.
	Notes []Note
}

// Changed reports whether the upgrade rewrote anything.
func (r Result) Changed() bool {
	return r.HeaderAdded || len(r.Rewritten) > 0 || len(r.GatedRewritten) > 0 || r.CommitsPinned > 0
}

// Upgrade rewrites src to the canonical language. It returns an error when src
// does not parse, when the header was asked for but a construct under it has no
// rewrite, or when the rewritten script fails its re-parse check.
func Upgrade(src string, opts Options) (Result, error) {
	return upgrader{lookup: deprecation.Lookup, gated: gatedRewriters}.upgrade(src, opts)
}

// upgrader carries the two registries so a test can substitute them.
type upgrader struct {
	lookup func(code string) (deprecation.Entry, bool)
	gated  map[string]GatedRewriter
}

func (u upgrader) upgrade(src string, opts Options) (Result, error) {
	return u.upgradeProg(src, opts, parse)
}

func parse(src string) (*ast.Program, error) {
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return prog, nil
}

// upgradeProg is upgrade with the parser as a parameter, so a test can hand it
// a Program carrying constructs the visitor does not produce yet.
func (u upgrader) upgradeProg(src string, opts Options, parse func(string) (*ast.Program, error)) (Result, error) {
	res := Result{Source: src, Rewritten: map[string]int{}, GatedRewritten: map[string]int{}}
	prog, err := parse(src)
	if err != nil {
		return res, fmt.Errorf("the script does not parse, so it cannot be upgraded: %w", err)
	}
	text := newSource(src)

	var edits []Edit
	for _, d := range prog.Deprecations {
		e, ok := u.lookup(d.Code)
		if !ok {
			return res, fmt.Errorf("line %d: the visitor recorded %s, which is not in the deprecation registry", d.Line, d.Code)
		}
		switch {
		case e.Rewrite.Token != "":
			ed, err := tokenSwap(text, d, e.Rewrite)
			if err != nil {
				return res, err
			}
			edits = append(edits, ed)
		case e.Rewrite.Structural != "" && d.Fix != nil:
			// A structural rewrite is computed from the parse tree by the
			// visitor that recorded the use (mdl/visitor/visitor_upgrade_fixes.go).
			edits = append(edits, d.Fix.Edits...)
		default:
			res.Unrewritten = append(res.Unrewritten, d)
			continue
		}
		res.Rewritten[d.Code]++
	}

	addHeader := opts.AddHeader && prog.LanguageHeaderLine == 0
	if !addHeader {
		// A version-neutral gated rewrite applies without the header too: its
		// output means the same under the script's own version.
		for _, n := range prog.LanguageNotes {
			if !versionNeutral[n.Code] || n.Fix == nil {
				continue
			}
			edits = append(edits, n.Fix.Edits...)
			res.GatedRewritten[n.Code]++
		}
	}
	if addHeader {
		var blocked []Blocked
		for _, n := range prog.LanguageNotes {
			n = resolveOperand(n, opts.Flows)
			rw, ok := u.gated[n.Code]
			if !ok {
				reason := unrewritable[n.Code]
				if reason == "" {
					reason = "no rewrite is registered for it"
				}
				blocked = append(blocked, Blocked{Line: n.Line, Code: n.Code, Reason: reason})
				continue
			}
			ed, err := rw(text, prog, n)
			if err != nil {
				blocked = append(blocked, Blocked{Line: n.Line, Code: n.Code, Reason: err.Error()})
				continue
			}
			edits = append(edits, ed...)
			res.GatedRewritten[n.Code]++
		}
		if len(blocked) > 0 {
			return res, &HeaderBlockedError{From: prog.LanguageVersion, Constructs: blocked}
		}
	}

	pins, pinned, notes := pinCommitEvents(prog, opts.Commits)
	edits = append(edits, pins...)
	res.CommitsPinned, res.Notes = pinned, notes

	out, err := text.apply(edits)
	if err != nil {
		return res, err
	}
	if addHeader {
		out = langver.Latest.String() + ";\n" + out
		res.HeaderAdded = true
	}
	if err := u.verify(prog, out, parse); err != nil {
		return res, err
	}
	res.Source = out
	return res, nil
}

// Blocked is one construct that keeps a script from taking the header: its
// meaning would change under it, and it has no mechanical rewrite.
type Blocked struct {
	Line   int
	Code   string // the langver.Change's rule ID
	Reason string // why there is no rewrite, and what to write instead
}

// HeaderBlockedError is Upgrade's refusal to add the language header over
// constructs whose meaning it would change and that it cannot rewrite. The
// script is left at its version; nothing is written.
type HeaderBlockedError struct {
	From       langver.Version
	Constructs []Blocked
}

func (e *HeaderBlockedError) Error() string {
	lines := make([]string, len(e.Constructs))
	for i, c := range e.Constructs {
		lines[i] = fmt.Sprintf("line %d: %s: %s", c.Line, c.Code, c.Reason)
	}
	return fmt.Sprintf("cannot add the `%s;` header: %d construct(s) would change meaning under it "+
		"and have no mechanical rewrite, so the script is left at %s:\n  %s",
		langver.Latest, len(e.Constructs), e.From, strings.Join(lines, "\n  "))
}

// verify re-parses the upgraded script. It must parse, keep every statement,
// and record no deprecated use the upgrade knows how to rewrite: a use that
// survives means a rewrite did not produce the canonical spelling, and a
// second run would rewrite again, so fmt would not be idempotent.
//
// A header-gated construct that survives is caught the same way: under the
// header the parse records no LanguageNote, but the construct must now be
// spelled so that it means what it meant before, which only the execute-both
// test can judge.
func (u upgrader) verify(orig *ast.Program, out string, parse func(string) (*ast.Program, error)) error {
	prog, err := parse(out)
	if err != nil {
		return fmt.Errorf("the upgraded script does not parse (an upgrade rewrite is wrong): %w", err)
	}
	if len(prog.Statements) != len(orig.Statements) {
		return fmt.Errorf("the upgraded script has %d statements, the original %d (an upgrade rewrite is wrong)",
			len(prog.Statements), len(orig.Statements))
	}
	for _, d := range prog.Deprecations {
		if e, ok := u.lookup(d.Code); ok && (e.Rewrite.Token != "" || d.Fix != nil) {
			return fmt.Errorf("line %d: the upgraded script still uses a deprecated spelling (%s); its rewrite did not produce %q",
				d.Line, d.Code, e.Canonical)
		}
	}
	return nil
}

// Edit replaces the runes [Start, Stop) of the source with Text; Start ==
// Stop inserts. Offsets count runes from the start of the script, the
// coordinates ANTLR's character stream reports.
type Edit = ast.TextEdit

// Source is a script as runes, the coordinates the parser reports positions
// in, with the offset each line starts at.
type Source struct {
	runes      []rune
	lineStarts []int
}

func newSource(src string) *Source {
	s := &Source{runes: []rune(src), lineStarts: []int{0}}
	for i, r := range s.runes {
		if r == '\n' {
			s.lineStarts = append(s.lineStarts, i+1)
		}
	}
	return s
}

// Offset returns the rune offset of (line, col): line 1-based, col 0-based in
// runes, as ANTLR reports token positions. False when out of range.
func (s *Source) Offset(line, col int) (int, bool) {
	if line < 1 || line > len(s.lineStarts) || col < 0 {
		return 0, false
	}
	end := len(s.runes)
	if line < len(s.lineStarts) {
		end = s.lineStarts[line] - 1
	}
	off := s.lineStarts[line-1] + col
	if off > end {
		return 0, false
	}
	return off, true
}

// At returns the n runes at (line, col), or false when they are out of range
// or cross a line end.
func (s *Source) At(line, col, n int) (string, bool) {
	off, ok := s.Offset(line, col)
	if !ok || n < 0 {
		return "", false
	}
	if end, ok := s.Offset(line, col+n); !ok || end != off+n {
		return "", false
	}
	return string(s.runes[off : off+n]), true
}

// apply returns the source with edits applied. Edits may not overlap; two at
// the same offset apply in the order given, an insertion before a replacement.
func (s *Source) apply(edits []Edit) (string, error) {
	sorted := append([]Edit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].Stop-sorted[i].Start < sorted[j].Stop-sorted[j].Start
	})
	var b strings.Builder
	at := 0
	for _, e := range sorted {
		if e.Start < 0 || e.Stop < e.Start || e.Stop > len(s.runes) {
			return "", fmt.Errorf("%s: upgrade rewrite is outside the source", s.where(e.Start))
		}
		if e.Start < at {
			return "", fmt.Errorf("%s: two upgrade rewrites overlap", s.where(e.Start))
		}
		b.WriteString(string(s.runes[at:e.Start]))
		b.WriteString(e.Text)
		at = e.Stop
	}
	b.WriteString(string(s.runes[at:]))
	return b.String(), nil
}

// where names a rune offset as "line L column C" (column 1-based).
func (s *Source) where(off int) string {
	line := sort.Search(len(s.lineStarts), func(i int) bool { return s.lineStarts[i] > off })
	if line == 0 {
		return fmt.Sprintf("offset %d", off)
	}
	return fmt.Sprintf("line %d column %d", line, off-s.lineStarts[line-1]+1)
}

// tokenSwap is the edit for a deprecation.Rewrite: the recorded token, which
// must be the rewrite's Token, becomes its Replacement in the same letter case.
func tokenSwap(s *Source, d ast.DeprecatedSpelling, rw deprecation.Rewrite) (Edit, error) {
	n := len([]rune(rw.Token))
	got, ok := s.At(d.Line, d.Column, n)
	if !ok || !strings.EqualFold(got, rw.Token) {
		return Edit{}, fmt.Errorf("line %d column %d: %s is recorded here, but the source has %q where %q was expected",
			d.Line, d.Column, d.Code, got, rw.Token)
	}
	off, _ := s.Offset(d.Line, d.Column)
	return Edit{Start: off, Stop: off + n, Text: matchCase(got, rw.Replacement)}, nil
}

// matchCase spells word in the letter case of like: all upper, capitalised, or
// lower. A script written in upper-case keywords stays upper-case.
func matchCase(like, word string) string {
	switch {
	case like == strings.ToUpper(like) && like != strings.ToLower(like):
		return strings.ToUpper(word)
	case len(like) > 1 && like[:1] == strings.ToUpper(like[:1]) && like[1:] == strings.ToLower(like[1:]):
		return strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	default:
		return strings.ToLower(word)
	}
}

// resolveOperand settles a note whose rewrite waits on what a called flow
// returns (ast.OperandChoice, ako/mxcli#860) by asking the project, the way
// the mdl 0 flow builder reads the call's result type when it builds the
// flow: a String makes `find(…)` / `contains(…)` the string function, anything
// else the List operation. Without a project, or when the project cannot
// answer, the note keeps no fix and says why.
func resolveOperand(n ast.LanguageNote, flows FlowTypes) ast.LanguageNote {
	c := n.Operand
	if n.Fix != nil || c == nil || flows == nil {
		return n
	}
	kinds := append([]bool(nil), c.Known...)
	for _, f := range c.Flows {
		isString, found := flows.ReturnsString(f.Nanoflow, f.Name.String())
		if !found {
			kind := "microflow"
			if f.Nanoflow {
				kind = "nanoflow"
			}
			n.NoFix = fmt.Sprintf("%s is assigned the result of %s %s, and the project has no %s %s, so whether "+
				"`%s(…)` is the string function or the List operation cannot be read from it; write `set $x = %s(…);` "+
				"or `$x = %s %s …;` by hand", c.Variable, kind, f.Name, kind, f.Name, c.Function, c.Function, c.Function, c.Variable)
			return n
		}
		kinds = append(kinds, isString)
	}
	for _, k := range kinds[1:] {
		if k != kinds[0] {
			n.NoFix = visitor.OperandReadingsDisagree(c.Variable, c.Function)
			return n
		}
	}
	if kinds[0] {
		n.Fix, n.NoFix = c.StringFix, ""
	} else {
		n.Fix, n.NoFix = c.ListFix, c.ListNoFix
	}
	return n
}
