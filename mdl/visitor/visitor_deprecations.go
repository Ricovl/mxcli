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

// replaceGateFix is the rewrite that keeps an mdl 0 `create or replace` on a
// gated kind meaning what it means there.
//
// For a user role or demo user that is a plain create, so `or replace` is
// dropped. A view entity's drop-and-recreate has no mdl 1 spelling: `create or
// modify` keeps the identity the old meaning discards, and a `drop` in front
// would fail where the old statement created the entity.
func replaceGateFix(ctx *parser.CreateStatementContext, c langver.Change) (*ast.Fix, string) {
	if c.Code != roleReplaceIsModify.Code {
		return nil, "`create or replace view entity` drops and recreates the view entity under mdl 0, which no " +
			"mdl 1 statement does: write `create or modify view entity` to keep its identity, or `drop entity` " +
			"then `create view entity` to discard it"
	}
	or, next := ctx.OR().GetSymbol(), ctx.REPLACE().GetSymbol()
	stop := next.GetStop() + 1
	if is := next.GetInputStream(); stop < is.Size() && is.GetText(stop, stop) == " " {
		stop++
	}
	return &ast.Fix{Edits: []ast.TextEdit{{Start: or.GetStart(), Stop: stop}}}, ""
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
		fix, why := replaceGateFix(ctx, c)
		b.fixLastNote(c.Code, fix, why)
		return
	}
	b.recordDeprecation(deprecation.CreateOrReplace, ctx.REPLACE().GetSymbol(), kind)
}

// showKind is what the showStatement that follows `show` is, for R6
// (PROPOSAL_mdl_beta_syntax_freeze.md §3): a plural or relationship query,
// whose canonical verb is `list` (MDL-DEPR002); a single thing, whose
// canonical verb is `describe` (MDL-DEPR090); or session state, which becomes
// a REPL command (R7) and is not reported yet. Pinned by
// TestShowRecordsDeprecation and TestShowSingleThingIsDescribe.
type showKind int

const (
	showIsList showKind = iota
	showIsDescribe
	showIsSession
)

// showDescribeFirst lists the tokens after `show` that name one thing. Keyed
// on the first token (NAVIGATION is refined below: `navigation homes` is a
// list).
var showDescribeFirst = map[int]bool{
	parser.MDLParserENTITY:      true, // show entity X        -> describe entity X
	parser.MDLParserASSOCIATION: true, // show association X   -> describe association X
	parser.MDLParserPAGE:        true, // show page X          -> describe page X
	parser.MDLParserNAVIGATION:  true, // show navigation …    -> describe navigation
	parser.MDLParserSTRUCTURE:   true, // show structure       -> describe structure
	parser.MDLParserCONTEXT:     true, // show context of X    -> describe context of X
	parser.MDLParserPROJECT:     true, // show project security -> describe app security
	parser.MDLParserSECURITY:    true, // show security matrix -> describe security matrix
	parser.MDLParserSETTINGS:    true, // show settings        -> describe settings
}

// showSession lists the session-state forms (R7): not reported until the REPL
// commands that replace them exist.
var showSession = map[int]bool{
	parser.MDLParserVERSION:     true,
	parser.MDLParserSTATUS:      true,
	parser.MDLParserCONNECTIONS: true,
}

// showStatementKind classifies the showStatement that ctx starts.
func showStatementKind(ctx *parser.ShowOrListContext) showKind {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok || stmt.GetChildCount() < 2 {
		return showIsList
	}
	first := tokenTypeAt(stmt, 1)
	switch {
	case first == parser.MDLParserCATALOG:
		if tokenTypeAt(stmt, 2) == parser.MDLParserSTATUS {
			return showIsSession // catalog status is session state
		}
		return showIsList
	case first == parser.MDLParserNAVIGATION && stmt.HOMES() != nil:
		return showIsList
	case showSession[first]:
		return showIsSession
	case showDescribeFirst[first]:
		return showIsDescribe
	}
	return showIsList
}

// tokenTypeAt is the token type of stmt's i-th child, or TokenInvalidType when
// it is not a terminal.
func tokenTypeAt(stmt antlr.ParserRuleContext, i int) int {
	if i >= stmt.GetChildCount() {
		return antlr.TokenInvalidType
	}
	if tn, ok := stmt.GetChild(i).(antlr.TerminalNode); ok {
		return tn.GetSymbol().GetTokenType()
	}
	return antlr.TokenInvalidType
}

// ExitShowOrList records the deprecated `show`: MDL-DEPR002 where its
// canonical form is `list`, MDL-DEPR090 where it is `describe` (for `list`
// as well: a single thing is described, not listed). showOrList is
// used only by showStatement, where `show` and `list` build the same
// statement.
func (b *Builder) ExitShowOrList(ctx *parser.ShowOrListContext) {
	if ctx == nil {
		return
	}
	switch showStatementKind(ctx) {
	case showIsList:
		if ctx.SHOW() != nil {
			b.recordDeprecation(deprecation.Show, ctx.SHOW().GetSymbol(), "")
		}
	case showIsDescribe:
		// `list entity X` names one thing too, so it is reported with `show`.
		b.recordShowSingleThing(ctx)
	}
}
