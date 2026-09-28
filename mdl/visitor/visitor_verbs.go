// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R6 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ADR-0010): one verb per job.
// `show` is split between `list` and `describe`, an alter's children are
// added and dropped, and the words that broke a pattern (`rest call`,
// `define fragment`, `describe widget <name>`, the `column` synonym) take the
// pattern's word. Every old spelling builds the same statement as its
// replacement, so the parse tree is the only place it is visible: the
// listeners below record it (codes MDL-DEPR090..096).

// showNoRewrite says why a single-thing `show` has no mechanical rewrite: its
// summary is not what `describe` prints, so rewriting it would change the
// output. Keyed on the token after `show`.
var showNoRewrite = map[int]string{
	parser.MDLParserENTITY: "`show entity` prints a summary table, `describe entity` the full definition as MDL; " +
		"replace it by hand",
	parser.MDLParserASSOCIATION: "`show association` prints a summary, `describe association` the full definition " +
		"as MDL; replace it by hand",
	parser.MDLParserNAVIGATION: "`show navigation` prints a summary table, `describe navigation` the full " +
		"definition as MDL; replace it by hand",
	parser.MDLParserSETTINGS: "`show settings` prints a summary table, `describe settings` the full definition " +
		"as MDL; replace it by hand",
}

// recordShowSingleThing records MDL-DEPR090 for `show` (or `list`) on a form
// that names one thing, with the rewrite to `describe` where the describe
// statement is the same statement: page, app security, security matrix,
// structure and context.
func (b *Builder) recordShowSingleThing(ctx *parser.ShowOrListContext) {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok {
		return
	}
	verb := ctx.GetStart()
	b.recordDeprecation(deprecation.ShowSingleThing, verb, "")
	first := tokenTypeAt(stmt, 1)
	if why, ok := showNoRewrite[first]; ok {
		if first == parser.MDLParserNAVIGATION && stmt.MENU_KW() != nil {
			why = "`show navigation menu` prints the menu tree, `describe navigation` the profile as MDL; replace it by hand"
		}
		b.fixLastDeprecation(deprecation.ShowSingleThing, nil, why)
		return
	}
	edit := replaceSpan(verb, verb, keywordLike(verb.GetText(), "describe"))
	if first == parser.MDLParserPROJECT {
		// `show project security` -> `describe app security` (R10's name).
		edit = replaceSpan(verb, stmt.SECURITY().GetSymbol(), keywordLike(verb.GetText(), "describe app security"))
	}
	b.fixLastDeprecation(deprecation.ShowSingleThing, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterAlterUserRoleStatement records `remove module roles` for `drop module
// roles`.
func (b *Builder) EnterAlterUserRoleStatement(ctx *parser.AlterUserRoleStatementContext) {
	if ctx.REMOVE() != nil {
		b.recordDeprecation(deprecation.UserRoleRemove, ctx.REMOVE().GetSymbol(), "")
	}
}

// EnterAlterSettingsClause records `remove` on a language or workflow group.
func (b *Builder) EnterAlterSettingsClause(ctx *parser.AlterSettingsClauseContext) {
	if ctx.REMOVE() != nil {
		b.recordDeprecation(deprecation.SettingsRemove, ctx.REMOVE().GetSymbol(), "")
	}
}

// ExitAttributeKw records `column` for `attribute` in alter entity.
func (b *Builder) ExitAttributeKw(ctx *parser.AttributeKwContext) {
	if ctx.COLUMN() != nil {
		b.recordDeprecation(deprecation.ColumnForAttribute, ctx.COLUMN().GetSymbol(), "")
	}
}

// ExitRestCallKw records `rest call` for `call rest service`, rewriting both
// words at once in the letter case of the first.
func (b *Builder) ExitRestCallKw(ctx *parser.RestCallKwContext) {
	if ctx.REST() == nil || ctx.CALL() == nil || ctx.SERVICE() != nil {
		return
	}
	first := ctx.REST().GetSymbol()
	b.recordDeprecation(deprecation.RestCall, first, "")
	edit := replaceSpan(first, ctx.CALL().GetSymbol(), keywordLike(first.GetText(), "call rest service"))
	b.fixLastDeprecation(deprecation.RestCall, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterDescribeStatement records `describe widget <name>` for `describe widget
// type <name>`. Only the widget-definition forms: `describe styling … widget`
// and `describe fragment … widget` name a widget on a page, which is the
// reading `type` exists to keep apart.
func (b *Builder) EnterDescribeStatement(ctx *parser.DescribeStatementContext) {
	if ctx.WIDGET() == nil || ctx.TYPE() != nil || ctx.STYLING() != nil || ctx.FRAGMENT() != nil {
		return
	}
	w := ctx.WIDGET().GetSymbol()
	b.recordDeprecation(deprecation.DescribeWidgetType, w, "")
	edit := insertAt(w.GetStop()+1, " "+keywordLike(w.GetText(), "type"))
	b.fixLastDeprecation(deprecation.DescribeWidgetType, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterDefineFragmentStatement records `define fragment` for `create fragment`.
func (b *Builder) EnterDefineFragmentStatement(ctx *parser.DefineFragmentStatementContext) {
	if ctx.DEFINE() != nil {
		b.recordDeprecation(deprecation.DefineFragment, ctx.DEFINE().GetSymbol(), "")
	}
}
