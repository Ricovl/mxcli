// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"

	"github.com/antlr4-go/antlr/v4"
)

// Deprecated spellings are recorded from the parse tree because it is the only
// place they are visible: `create or replace` and `create or modify`, `show`
// and `list`, build the same statements. The registry of spellings, and how a
// grammar alias is marked, is in mdl/deprecation.

// recordDeprecation appends one use of a deprecated spelling at tok.
func (b *Builder) recordDeprecation(code string, tok antlr.Token, subject string) {
	if tok == nil {
		return
	}
	b.deprecations = append(b.deprecations, ast.DeprecatedSpelling{
		Code:    code,
		Line:    tok.GetLine(),
		Column:  tok.GetColumn(),
		Subject: subject,
	})
}

// createOrReplaceIsNotAnAlias lists the create kinds (createStatementKind's
// words) where `or replace` does NOT mean `or modify`, so rewriting it would
// change the script. Measured against the visitors, and pinned by
// TestCreateOrReplaceMatchesModifyExceptExemptKinds:
//
//   - translations: `or replace` replaces the whole set, `or modify` merges.
//   - userrole, demouser: the visitor reads only `or modify`; `or replace` is
//     a plain create, which fails on an existing role or user.
//
// `create or replace view entity` (kind "entity") drops and recreates the view
// entity, and is exempted in recordCreateOrReplace.
var createOrReplaceIsNotAnAlias = map[string]bool{
	"translations": true,
	"userrole":     true,
	"demouser":     true,
}

// recordCreateOrReplace records MDL-DEPR001 for a `create or replace` whose
// meaning is exactly `create or modify`.
func (b *Builder) recordCreateOrReplace(ctx *parser.CreateStatementContext) {
	if ctx.OR() == nil || ctx.REPLACE() == nil {
		return
	}
	kind := createStatementKind(ctx)
	if kind == "" || createOrReplaceIsNotAnAlias[kind] {
		return
	}
	if ent, ok := ctx.CreateEntityStatement().(*parser.CreateEntityStatementContext); ok && ent.VIEW() != nil {
		return
	}
	b.recordDeprecation(deprecation.CreateOrReplace, ctx.REPLACE().GetSymbol(), kind)
}

// showNotYetList lists the showStatement forms whose decided canonical form is
// NOT `list` (PROPOSAL_mdl_beta_syntax_freeze.md §3, R6): a single thing
// becomes `describe`, session state a REPL command. Keyed on the token after
// `show` (CATALOG only with STATUS, see showCanonicalIsList). `list` builds the
// same statement for these today, but it is not their canonical form, so
// recommending it would name the wrong form and make `fmt --upgrade` rewrite
// them twice. They get their own registry entries once the canonical forms
// exist (plan item 3.5). Pinned by TestShowRecordsDeprecation.
var showNotYetList = map[int]bool{
	parser.MDLParserENTITY:      true, // show entity X      -> describe entity X
	parser.MDLParserASSOCIATION: true, // show association X -> describe association X
	parser.MDLParserPAGE:        true, // show page X        -> describe page X
	parser.MDLParserNAVIGATION:  true, // show navigation …  -> describe navigation
	parser.MDLParserSTRUCTURE:   true, // show structure     -> describe structure
	parser.MDLParserCONTEXT:     true, // show context of X  -> describe context of X
	parser.MDLParserPROJECT:     true, // show project security -> describe app security
	parser.MDLParserSECURITY:    true, // show security matrix  -> describe security matrix
	parser.MDLParserSETTINGS:    true, // show settings      -> describe (one thing)
	parser.MDLParserVERSION:     true, // session state      -> REPL command (R7)
	parser.MDLParserSTATUS:      true, // session state      -> REPL command (R7)
	parser.MDLParserCONNECTIONS: true, // session state      -> REPL command (R7)
}

// showCanonicalIsList reports whether the showStatement that ctx starts is a
// form whose canonical spelling is `list` (plurals and relationship queries).
func showCanonicalIsList(ctx *parser.ShowOrListContext) bool {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok || stmt.GetChildCount() < 2 {
		return true
	}
	next := func(i int) int {
		if i >= stmt.GetChildCount() {
			return antlr.TokenInvalidType
		}
		if tn, ok := stmt.GetChild(i).(antlr.TerminalNode); ok {
			return tn.GetSymbol().GetTokenType()
		}
		return antlr.TokenInvalidType
	}
	first := next(1)
	if first == parser.MDLParserCATALOG {
		return next(2) != parser.MDLParserSTATUS // catalog status is session state
	}
	return !showNotYetList[first]
}

// ExitShowOrList records MDL-DEPR002 for `show` where its canonical form is
// `list`. showOrList is used only by showStatement, where `show` and `list`
// build the same statement.
func (b *Builder) ExitShowOrList(ctx *parser.ShowOrListContext) {
	if ctx == nil || ctx.SHOW() == nil || !showCanonicalIsList(ctx) {
		return
	}
	b.recordDeprecation(deprecation.Show, ctx.SHOW().GetSymbol(), "")
}
