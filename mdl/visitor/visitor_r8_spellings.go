// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R8 (PROPOSAL_mdl_beta_syntax_freeze.md §3, ako/mxcli#752): words, not
// SCREAMING_SNAKE, and one spelling per keyword. Every old spelling here is a
// respelling — the grammar builds the same statement from both — so each use
// is recorded as a deprecation with the edit that rewrites it, and nothing is
// gated on the language version.

// recordRespelling records code at tok with a fix that replaces tok's text.
// The subject names the use and its rewrite (`show_page -> show page`), since
// the registry entry covers a family of spellings.
func (b *Builder) recordRespelling(code string, tok antlr.Token, text string) {
	if tok == nil {
		return
	}
	b.recordDeprecation(code, tok, tok.GetText()+" -> "+text)
	b.fixLastDeprecation(code, &ast.Fix{Edits: []ast.TextEdit{{Start: tok.GetStart(), Stop: tok.GetStop() + 1, Text: text}}}, "")
}

// recordSnakeWord records a page-action token spelled with an underscore
// (`save_changes`), whose canonical spelling is the same words with a space.
// The lexer tokens that admit both spellings are told apart by the underscore.
func (b *Builder) recordSnakeWord(n antlr.TerminalNode) {
	if n == nil {
		return
	}
	tok := n.GetSymbol()
	if text := tok.GetText(); strings.Contains(text, "_") {
		b.recordRespelling(deprecation.PageActionWord, tok, strings.ReplaceAll(text, "_", " "))
	}
}

// ExitActionExprV3 reports a page action written in its snake-case spelling,
// or a flow call written without `call`.
func (b *Builder) ExitActionExprV3(ctx *parser.ActionExprV3Context) {
	for _, n := range []antlr.TerminalNode{ctx.SAVE_CHANGES(), ctx.CANCEL_CHANGES(), ctx.SHOW_PAGE(),
		ctx.CREATE_OBJECT(), ctx.SIGN_OUT(), ctx.COMPLETE_TASK()} {
		b.recordSnakeWord(n)
	}
	if n := ctx.DELETE_OBJECT(); n != nil {
		tok := n.GetSymbol()
		b.recordRespelling(deprecation.PageActionWord, tok, keywordLike(tok.GetText(), "delete"))
	}
	if ctx.CALL() == nil {
		for _, n := range []antlr.TerminalNode{ctx.MICROFLOW(), ctx.NANOFLOW()} {
			if n == nil {
				continue
			}
			tok := n.GetSymbol()
			b.recordDeprecation(deprecation.PageActionWord, tok, tok.GetText()+" -> "+keywordLike(tok.GetText(), "call")+" "+tok.GetText())
			b.fixLastDeprecation(deprecation.PageActionWord,
				&ast.Fix{Edits: []ast.TextEdit{insertAt(tok.GetStart(), keywordLike(tok.GetText(), "call")+" ")}}, "")
		}
	}
}

// ExitClosePageV3 reports `close_page`.
func (b *Builder) ExitClosePageV3(ctx *parser.ClosePageV3Context) {
	b.recordSnakeWord(ctx.CLOSE_PAGE())
}

// ExitOpenLinkV3 reports `open_link`.
func (b *Builder) ExitOpenLinkV3(ctx *parser.OpenLinkV3Context) { b.recordSnakeWord(ctx.OPEN_LINK()) }

// ExitNavMenuItemDef reports a menu item's `sign_out`: menus use the page
// action vocabulary.
func (b *Builder) ExitNavMenuItemDef(ctx *parser.NavMenuItemDefContext) {
	b.recordSnakeWord(ctx.SIGN_OUT())
}

// errorMessageWord is the canonical spelling of the message keyword, in the
// letter case of the spelling it replaces.
func errorMessageWord(like string) string { return keywordLike(like, "error message") }

// recordErrorMessageToken reports an ERROR_MESSAGE token spelled `error_message`
// or `errormessage`. Only where the token is the message keyword: an attribute
// or key named ErrorMessage lexes as the same token.
func (b *Builder) recordErrorMessageToken(n antlr.TerminalNode, subject string) {
	if n == nil {
		return
	}
	tok := n.GetSymbol()
	if text := tok.GetText(); !strings.ContainsAny(text, " \t\r\n") {
		b.recordRespelling(deprecation.ErrorMessageKeyword, tok, errorMessageWord(text))
	}
}

// ExitConstraintErrorKeyword reports `not null error '…'` (and after unique
// and required).
func (b *Builder) ExitConstraintErrorKeyword(ctx *parser.ConstraintErrorKeywordContext) {
	if n := ctx.ERROR(); n != nil {
		tok := n.GetSymbol()
		b.recordRespelling(deprecation.ErrorMessageKeyword, tok, errorMessageWord(tok.GetText()))
		return
	}
	b.recordErrorMessageToken(ctx.ERROR_MESSAGE(), "attribute constraint")
}

// ExitErrorMessageClause reports an association's `error_message '…'`.
func (b *Builder) ExitErrorMessageClause(ctx *parser.ErrorMessageClauseContext) {
	b.recordErrorMessageToken(ctx.ERROR_MESSAGE(), "association")
}

// ExitMicroflowConcurrencyError reports `disallow concurrent execution
// error_message '…'`.
func (b *Builder) ExitMicroflowConcurrencyError(ctx *parser.MicroflowConcurrencyErrorContext) {
	b.recordErrorMessageToken(ctx.ERROR_MESSAGE(), "microflow")
}

// recordValidationRuleFeedback reports a validation rule's `feedback '…'`.
func (b *Builder) recordValidationRuleFeedback(ctx *parser.CreateValidationRuleStatementContext) {
	if n := ctx.FEEDBACK(); n != nil {
		tok := n.GetSymbol()
		b.recordRespelling(deprecation.ErrorMessageKeyword, tok, errorMessageWord(tok.GetText()))
		return
	}
	b.recordErrorMessageToken(ctx.ERROR_MESSAGE(), "validation rule")
}

// onDeleteFor is the `on delete` action a delete_behavior behaviour means; the
// mapping is buildDeleteBehavior's, read against buildReferentialAction.
func onDeleteFor(ctx parser.IDeleteBehaviorContext) string {
	db, ok := ctx.(*parser.DeleteBehaviorContext)
	if !ok {
		return ""
	}
	switch {
	case db.CASCADE() != nil, db.DELETE_AND_REFERENCES() != nil:
		return "on delete cascade"
	case db.PREVENT() != nil, db.DELETE_IF_NO_REFERENCES() != nil:
		return "on delete restrict"
	case db.DELETE_BUT_KEEP_REFERENCES() != nil:
		return "on delete set null"
	}
	return ""
}

// recordDeleteBehavior reports `delete_behavior <behaviour>`, rewritten to the
// `on delete` action that means the same.
func (b *Builder) recordDeleteBehavior(kw antlr.TerminalNode, behaviour parser.IDeleteBehaviorContext) {
	if kw == nil || behaviour == nil {
		return
	}
	tok := kw.GetSymbol()
	action := onDeleteFor(behaviour)
	b.recordDeprecation(deprecation.DeleteBehaviorClause, tok, "-> "+action)
	if action == "" || behaviour.GetStop() == nil {
		b.fixLastDeprecation(deprecation.DeleteBehaviorClause, nil, "unrecognised delete behaviour")
		return
	}
	edit := replaceSpan(tok, behaviour.GetStop(), keywordLike(tok.GetText(), action))
	b.fixLastDeprecation(deprecation.DeleteBehaviorClause, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// ExitAssociationOption reports `delete_behavior …` and `reference_set`.
func (b *Builder) ExitAssociationOption(ctx *parser.AssociationOptionContext) {
	b.recordAssociationClauseColon(ctx) // R3: `type: Reference` (visitor_r3_property_lists.go)
	b.recordDeleteBehavior(ctx.DELETE_BEHAVIOR(), ctx.DeleteBehavior())
	if n := ctx.REFERENCE_SET(); n != nil && strings.Contains(n.GetText(), "_") {
		b.recordRespelling(deprecation.ReferenceSetUnderscore, n.GetSymbol(), "ReferenceSet")
	}
}

// ExitRestCallReturnsClause reports `returns none`. The registry entry is a
// keyword swap, so no per-use fix is attached.
func (b *Builder) ExitRestCallReturnsClause(ctx *parser.RestCallReturnsClauseContext) {
	if n := ctx.NONE(); n != nil {
		b.recordDeprecation(deprecation.ReturnsNone, n.GetSymbol(), "rest call")
	}
}
