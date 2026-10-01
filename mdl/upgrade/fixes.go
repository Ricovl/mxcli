// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// Fix is what `fmt --upgrade` does to one recorded use, on its own: the quick
// fix an editor offers on that use's warning (ako/mxcli#714 decision 5).
type Fix struct {
	// Code is the use's warning code: MDL-DEPRnnn, or a version-neutral
	// MDL-V1-* change.
	Code string
	// Line is the use's 1-based line. Column is its 0-based rune column, as
	// the parser reports it, for a deprecated spelling; -1 for a language
	// change, which the parser records by line only.
	Line, Column int
	// Edits rewrite the use, in rune offsets into the script (ast.TextEdit).
	Edits []Edit
}

// FixesOnLines returns the rewrite of each use on lines [from, to] (1-based,
// inclusive) that has one on its own: every deprecated spelling with a
// rewrite, and every header-gated construct whose rewrite is version-neutral.
// Any other header-gated construct has none on its own — its rewrite keeps the
// old meaning only under the header, so it comes with the whole
// `fmt --upgrade --header` — and is left out, as is a use with no rewrite.
//
// Each fix is checked alone the way Upgrade checks the whole: the rewritten
// script parses, keeps every statement, and no longer records the use. A fix
// that fails the check is left out rather than offered. It returns an error
// only when src does not parse.
func FixesOnLines(src string, from, to int) ([]Fix, error) {
	return upgrader{lookup: deprecation.Lookup, gated: gatedRewriters}.fixesOnLines(src, from, to)
}

func (u upgrader) fixesOnLines(src string, from, to int) ([]Fix, error) {
	prog, err := parse(src)
	if err != nil {
		return nil, fmt.Errorf("the script does not parse, so it has no upgrade fixes: %w", err)
	}
	text := newSource(src)
	on := func(line int) bool { return line >= from && line <= to }
	var fixes []Fix
	for _, d := range prog.Deprecations {
		if !on(d.Line) {
			continue
		}
		edits, ok, err := u.deprecationEdits(text, d)
		if err != nil || !ok || len(edits) == 0 {
			continue
		}
		if u.fixHolds(text, prog, edits, d.Code) {
			fixes = append(fixes, Fix{Code: d.Code, Line: d.Line, Column: d.Column, Edits: edits})
		}
	}
	for _, n := range prog.LanguageNotes {
		if !on(n.Line) || !versionNeutral[n.Code] || n.Fix == nil || len(n.Fix.Edits) == 0 {
			continue
		}
		if u.fixHolds(text, prog, n.Fix.Edits, n.Code) {
			fixes = append(fixes, Fix{Code: n.Code, Line: n.Line, Column: -1, Edits: n.Fix.Edits})
		}
	}
	return fixes, nil
}

// fixHolds applies edits alone and checks the result: it parses, keeps every
// statement, and records one use of code fewer.
func (u upgrader) fixHolds(text *Source, prog *ast.Program, edits []Edit, code string) bool {
	out, err := text.apply(edits)
	if err != nil {
		return false
	}
	after, err := parse(out)
	if err != nil || len(after.Statements) != len(prog.Statements) {
		return false
	}
	return uses(after, code) == uses(prog, code)-1
}

// uses counts the recorded uses of code in prog.
func uses(prog *ast.Program, code string) int {
	n := 0
	for _, d := range prog.Deprecations {
		if d.Code == code {
			n++
		}
	}
	for _, l := range prog.LanguageNotes {
		if l.Code == code {
			n++
		}
	}
	return n
}
