// SPDX-License-Identifier: Apache-2.0

package upgrade

import "github.com/mendixlabs/mxcli/mdl/ast"

// GatedRewriter rewrites one header-gated construct so that, once the script
// carries the new header, it still means what it meant without one.
//
// note is the ast.LanguageNote the visitor recorded through Builder.gate when
// it parsed the headerless script (its Code is the langver.Change's rule ID);
// prog is that parse. The rewriter returns the source edits, in the parser's
// coordinates. When a construct needs more than the note's line to locate it,
// extend ast.LanguageNote with the position rather than re-finding it in text.
type GatedRewriter func(src *Source, prog *ast.Program, note ast.LanguageNote) ([]Edit, error)

// gatedRewriters holds one rewrite per langver.Change code: a change of meaning
// or new rejection that applies only under `mdl 1` (ADR-0011 decision 2).
//
// A change lands with its rewrite here (proposal §9, "Each change lands with a
// registry entry, an fmt --upgrade rewrite where one is possible"). A change
// without one is reported by `fmt --upgrade --header`, which then refuses to
// add the header to a script that uses it: adding the header over it would be
// exactly the silent change of meaning the header exists to prevent.
//
// Empty while no change of meaning is on main; the first ones are the mdl 1
// constructs of ako/mxcli#732 (strict parsing), #733 (list operations as
// statements, mandatory `set`) and #734 (`limit 1` is a list).
var gatedRewriters = map[string]GatedRewriter{}
