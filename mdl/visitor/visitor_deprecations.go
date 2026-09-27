// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"

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
// words) where `or replace` does NOT mean `or modify` under any language
// version, so rewriting it would change the script. Measured against the
// visitors, and pinned by TestCreateOrReplaceMatchesModifyExceptExemptKinds:
//
//   - translations: `or replace` replaces the whole set, `or modify` merges.
//
// Three more kinds differ only under mdl 0 and are gated below: a view entity,
// a user role and a demo user.
var createOrReplaceIsNotAnAlias = map[string]bool{
	"translations": true,
}

// viewEntityReplaceIsModify is R1 for view entities (ADR-0010; #731). Under
// mdl 0, `create or replace view entity` deletes the entity and creates a new
// one: a new $ID and GUID, and the access rules and associations that pointed
// at the old one are gone with it. From mdl 1 it is `create or modify`, the
// identity-carrying rewrite, like `or replace` on every other document type.
var viewEntityReplaceIsModify = langver.Change{
	Code:  "MDL-V1-REPLACE01",
	Since: langver.V1,
	Old:   "`create or replace view entity` deletes the view entity and creates a new one (a new identity; its access rules and associations are lost)",
	New:   "`create or modify view entity`, which rewrites it in place and keeps its identity",
}

// roleReplaceIsModify is R1 for user roles and demo users (#731). Under mdl 0
// their visitors read only `or modify`, so `or replace` is a plain create that
// fails on an existing role or user. From mdl 1 it is `create or modify`.
var roleReplaceIsModify = langver.Change{
	Code:  "MDL-V1-REPLACE02",
	Since: langver.V1,
	Old:   "`create or replace` on a user role or demo user is a plain create, which fails when it already exists",
	New:   "`create or modify`",
}

// replaceGate returns the language change `or replace` goes through for this
// create statement, and false when its meaning does not depend on the version.
func replaceGate(ctx *parser.CreateStatementContext) (langver.Change, bool) {
	switch createStatementKind(ctx) {
	case "userrole", "demouser":
		return roleReplaceIsModify, true
	case "entity":
		if ent, ok := ctx.CreateEntityStatement().(*parser.CreateEntityStatementContext); ok && ent.VIEW() != nil {
			return viewEntityReplaceIsModify, true
		}
	}
	return langver.Change{}, false
}

// replaceMeansModify reports whether `create or replace` on createStmt builds
// `create or modify` under the script's language version. The document
// builders ask it; recordCreateOrReplace reports what it decided.
func (b *Builder) replaceMeansModify(createStmt *parser.CreateStatementContext) bool {
	if createStmt == nil || createStmt.OR() == nil || createStmt.REPLACE() == nil {
		return false
	}
	if c, gated := replaceGate(createStmt); gated {
		return c.Applies(b.langVersion)
	}
	return true
}

// recordCreateOrReplace records MDL-DEPR001 for a `create or replace` whose
// meaning is exactly `create or modify`, and the language change for one whose
// meaning is kept at mdl 0.
func (b *Builder) recordCreateOrReplace(ctx *parser.CreateStatementContext) {
	if ctx.OR() == nil || ctx.REPLACE() == nil {
		return
	}
	kind := createStatementKind(ctx)
	if kind == "" || createOrReplaceIsNotAnAlias[kind] {
		return
	}
	if c, gated := replaceGate(ctx); gated && !b.gate(c, ctx) {
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
