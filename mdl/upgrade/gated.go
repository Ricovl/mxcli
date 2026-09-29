// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// GatedRewriter rewrites one header-gated construct so that, once the script
// carries the new header, it still means what it meant without one.
//
// note is the ast.LanguageNote the visitor recorded through Builder.gate when
// it parsed the headerless script (its Code is the langver.Change's rule ID);
// prog is that parse. The rewriter returns the source edits. An error says why
// this occurrence has no mechanical rewrite; the upgrade then reports it and
// refuses the header (HeaderBlockedError).
type GatedRewriter func(src *Source, prog *ast.Program, note ast.LanguageNote) ([]Edit, error)

// gatedRewriters holds one rewrite per langver.Change code: a change of meaning
// or new rejection that applies only under `mdl 1` (ADR-0011 decision 2).
//
// A change lands with its rewrite here (proposal §9, "Each change lands with a
// registry entry, an fmt --upgrade rewrite where one is possible"), or in
// unrewritable with the reason it has none. TestGatedRegistryIsComplete holds
// every change the visitor gates to exactly one of the two.
var gatedRewriters = map[string]GatedRewriter{
	"MDL-V1-SEMI":      visitorFix, // insert the missing `;`
	"MDL-V1-SLASH":     visitorFix, // delete the `/` line
	"MDL-V1-ESCAPE":    visitorFix, // write the string's mdl 0 value with `''` as the only escape
	"MDL-V1-LIMIT1":    visitorFix, // `limit 1` (an object) -> `first`
	"MDL-V1-SET":       visitorFix, // `$x = e` -> `set $x = e`
	"MDL-V1-LIST":      visitorFix, // call form -> statement form, or `set` for the string function
	"MDL-V1-REPLACE02": visitorFix, // `create or replace user role` (a plain create) -> `create user role`
	"MDL-V1-WHILE":     visitorFix, // insert the missing `begin` and `while` after `end`
	"MDL-V1-TEMPLATE":  visitorFix, // a template literal over lines -> `'{1}' with ({1} = literal)`
}

// unrewritable lists the changes with no mechanical rewrite at all, and why.
// A script that uses one keeps its version: `fmt --upgrade --header` reports
// each use and refuses the header. This list may only shrink.
var unrewritable = map[string]string{
	"MDL-V1-PROP": "an unknown property key is ignored under mdl 0 and an error under mdl 1; " +
		"which key was meant cannot be guessed, so correct or delete it by hand",
	"MDL-V1-PROPVALUE": "a value its key does not take is ignored or read by its shape under mdl 0; " +
		"what was meant cannot be guessed, so correct it by hand",
	"MDL-V1-REPLACE01": "`create or replace view entity` drops and recreates the view entity under mdl 0, " +
		"which no mdl 1 statement does: write `create or modify view entity` to keep its identity, or " +
		"`drop entity` then `create view entity` to discard it",
	"MDL-V1-SESSION": "a session command (`connect`, `set format`, `status`, `help`, …) is refused in an mdl 1 " +
		"script, and no model statement does what it does: move it out of the script, to the command line " +
		"(`-p app.mpr` to connect, `--json` for the output format) or the REPL",
	"MDL-V1-SHOWSUMMARY": "`show entity X` / `show association X` print a summary no mdl 1 statement prints: " +
		"`describe` prints the definition as MDL and `list entities` / `list associations` the summary columns, so " +
		"either would change the script's output; choose one by hand",
}

// visitorFix applies the rewrite the visitor computed from the parse tree when
// it recorded the note (mdl/visitor/visitor_upgrade_fixes.go). An occurrence
// without one carries the reason.
func visitorFix(_ *Source, _ *ast.Program, n ast.LanguageNote) ([]Edit, error) {
	if n.Fix == nil {
		if n.NoFix != "" {
			return nil, errors.New(n.NoFix)
		}
		return nil, errors.New("no mechanical rewrite for this occurrence")
	}
	return n.Fix.Edits, nil
}
