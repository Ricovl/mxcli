// SPDX-License-Identifier: Apache-2.0

// Package upgrade rewrites an MDL script to the canonical language: the engine
// behind `mxcli fmt --upgrade` (ADR-0011 decision 1, plan item 1.3 of
// PROPOSAL_mdl_beta_syntax_freeze.md).
//
// It does two kinds of rewrite, and nothing else:
//
//   - Every use of a deprecated spelling registered in mdl/deprecation becomes
//     its canonical spelling. A deprecated spelling is a respelling, so the
//     rewrite never changes meaning. The visitor records where each use is
//     (ast.Program.Deprecations); the entry's Rewrite says what replaces it.
//   - When the caller asks for the language header, every construct whose
//     meaning differs under the new version (ast.Program.LanguageNotes, one per
//     langver.Change) is rewritten to the spelling that keeps its OLD meaning
//     under the new header, and `mdl <n>;` is added. The rewrites live in
//     gatedRewriters (gated.go).
//
// Anything it cannot rewrite is reported, never guessed at: a deprecated use
// whose entry has no rewrite is left in place and listed in Result.Unrewritten,
// and a header-gated construct without a rewrite blocks the header, because
// adding the header over it would silently change the script's meaning.
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
}

// Changed reports whether the upgrade rewrote anything.
func (r Result) Changed() bool {
	return r.HeaderAdded || len(r.Rewritten) > 0 || len(r.GatedRewritten) > 0
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
		if e.Rewrite.IsZero() {
			res.Unrewritten = append(res.Unrewritten, d)
			continue
		}
		ed, err := tokenSwap(text, d, e.Rewrite)
		if err != nil {
			return res, err
		}
		edits = append(edits, ed)
		res.Rewritten[d.Code]++
	}

	addHeader := opts.AddHeader && prog.LanguageHeaderLine == 0
	if addHeader {
		var missing []string
		for _, n := range prog.LanguageNotes {
			rw, ok := u.gated[n.Code]
			if !ok {
				missing = append(missing, fmt.Sprintf("line %d: %s", n.Line, n.Code))
				continue
			}
			ed, err := rw(text, prog, n)
			if err != nil {
				return res, fmt.Errorf("line %d: rewriting %s: %w", n.Line, n.Code, err)
			}
			edits = append(edits, ed...)
			res.GatedRewritten[n.Code]++
		}
		if len(missing) > 0 {
			return res, fmt.Errorf("cannot add the `%s;` header: %d construct(s) would change meaning under it "+
				"and have no rewrite yet, so the script is left at %s:\n  %s",
				langver.Latest, len(missing), prog.LanguageVersion, strings.Join(missing, "\n  "))
		}
	}

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
		if e, ok := u.lookup(d.Code); ok && !e.Rewrite.IsZero() {
			return fmt.Errorf("line %d: the upgraded script still uses a deprecated spelling (%s); its rewrite did not produce %q",
				d.Line, d.Code, e.Canonical)
		}
	}
	return nil
}

// Edit replaces Len runes at (Line, Column) of the source with Text. Line is
// 1-based and Column 0-based, in runes, as ANTLR reports token positions.
type Edit struct {
	Line   int
	Column int
	Len    int
	Text   string
}

// Source is a script split into lines of runes, the coordinates the parser
// reports positions in.
type Source struct {
	lines [][]rune
}

func newSource(src string) *Source {
	parts := strings.Split(src, "\n")
	s := &Source{lines: make([][]rune, len(parts))}
	for i, p := range parts {
		s.lines[i] = []rune(p)
	}
	return s
}

// At returns the n runes at (line, col), or false when they are out of range.
func (s *Source) At(line, col, n int) (string, bool) {
	if line < 1 || line > len(s.lines) || col < 0 || n < 0 || col+n > len(s.lines[line-1]) {
		return "", false
	}
	return string(s.lines[line-1][col : col+n]), true
}

// apply returns the source with edits applied. Edits may not overlap.
func (s *Source) apply(edits []Edit) (string, error) {
	sorted := append([]Edit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].Column < sorted[j].Column
	})
	for i := 1; i < len(sorted); i++ {
		a, b := sorted[i-1], sorted[i]
		if a.Line == b.Line && a.Column+a.Len > b.Column {
			return "", fmt.Errorf("line %d: two upgrade rewrites overlap at columns %d and %d", a.Line, a.Column, b.Column)
		}
	}
	lines := make([][]rune, len(s.lines))
	copy(lines, s.lines)
	// Right to left, so an earlier edit's column is still valid after a later
	// one on the same line changed its length.
	for i := len(sorted) - 1; i >= 0; i-- {
		e := sorted[i]
		if _, ok := s.At(e.Line, e.Column, e.Len); !ok {
			return "", fmt.Errorf("line %d column %d: upgrade rewrite is outside the source", e.Line, e.Column)
		}
		l := lines[e.Line-1]
		nl := make([]rune, 0, len(l)-e.Len+len(e.Text))
		nl = append(nl, l[:e.Column]...)
		nl = append(nl, []rune(e.Text)...)
		nl = append(nl, l[e.Column+e.Len:]...)
		lines[e.Line-1] = nl
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = string(l)
	}
	return strings.Join(out, "\n"), nil
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
	return Edit{Line: d.Line, Column: d.Column, Len: n, Text: matchCase(got, rw.Replacement)}, nil
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
