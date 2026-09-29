// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// templateLineBreak is how a text template spells a line break (ako/mxcli#746).
//
// A `log`, `show message` or `validation feedback` whose message is one string
// literal stores that literal as the template text. Under mdl 0 that held only
// for a literal on one line: `\n` was the way to write a line break, and a
// literal that spans lines was an expression stored as written — the template
// became `{1}`, with the literal as its parameter. Under mdl 1 a backslash is
// an ordinary character (#732), so the line break is written into the literal
// itself, and the literal is the template text whether or not it spans lines.
//
// Only a message with no template parameters changes meaning. With parameters,
// mdl 0 wrote a literal that spans lines wrong under every reading (the log's
// template text kept the quotes, a message's `{1}` shifted every parameter one
// place), so that is the template text under both versions.
var templateLineBreak = langver.Change{
	Code:  "MDL-V1-TEMPLATE",
	Since: langver.V1,
	Old: "a message template written as one string literal that spans lines is an expression: " +
		"the template is `{1}` and the literal its parameter",
	New: "the template text, as a literal on one line is; a line break in a template is written into the literal",
}

// loneStringLiteral returns the literal when expr is exactly one string
// literal, and nil otherwise.
func loneStringLiteral(expr parser.IExpressionContext) antlr.Token {
	prc, ok := expr.(antlr.ParserRuleContext)
	if !ok || prc.GetStart() == nil || prc.GetStop() == nil {
		return nil
	}
	start := prc.GetStart()
	if start.GetTokenType() != parser.MDLLexerSTRING_LITERAL || start.GetTokenIndex() != prc.GetStop().GetTokenIndex() {
		return nil
	}
	return start
}

// buildTemplateMessage builds the message of a text template. A lone string
// literal is the template text (a LiteralExpr); a literal spanning lines with
// no parameters keeps its mdl 0 meaning, an expression, in a script without
// the header (see templateLineBreak).
func buildTemplateMessage(expr parser.IExpressionContext, hasParams bool) ast.Expression {
	msg := buildSourceExpression(expr)
	se, ok := msg.(*ast.SourceExpr)
	if !ok || loneStringLiteral(expr) == nil {
		return msg
	}
	if hasParams || templateLineBreak.Applies(languageVersionOf(expr)) {
		return se.Expression
	}
	return msg
}

// noteTemplateLineBreak warns on a message kept at its mdl 0 meaning, and
// records the rewrite that keeps it under mdl 1: the literal moves into a
// `{1}` parameter, which is what mdl 0 made of it, and the template becomes
// `'{1}'`. The parameter clause goes after `after`, the last token of the
// statement that may precede it, and `like` the keyword whose case `with`
// follows. The literal is copied as written: under
// mdl 0 an expression that spans lines is stored as written, escapes and all.
func (b *Builder) noteTemplateLineBreak(msg parser.IExpressionContext, hasParams bool, after, like antlr.Token) {
	lit := loneStringLiteral(msg)
	if lit == nil || hasParams || after == nil || !containsLineBreak(lit.GetText()) {
		return
	}
	if b.gate(templateLineBreak, msg.(antlr.ParserRuleContext)) {
		return
	}
	b.fixLastNote(templateLineBreak.Code, &ast.Fix{Edits: []ast.TextEdit{
		replaceSpan(lit, lit, "'{1}'"),
		insertAt(after.GetStop()+1, " "+keywordLike(like.GetText(), "with")+" ({1} = "+lit.GetText()+")"),
	}}, "")
}

func containsLineBreak(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return true
		}
	}
	return false
}

// ExitLogStatement records a log message kept at its mdl 0 meaning.
func (b *Builder) ExitLogStatement(ctx *parser.LogStatementContext) {
	msg := logMessageExpression(ctx)
	if msg == nil {
		return
	}
	b.noteTemplateLineBreak(msg, ctx.LogTemplateParams() != nil, msg.(antlr.ParserRuleContext).GetStop(), ctx.LOG().GetSymbol())
}

// logMessageExpression is the message of a log: the second expression when
// there is a node, else the first.
func logMessageExpression(ctx *parser.LogStatementContext) parser.IExpressionContext {
	var exprs []parser.IExpressionContext
	for _, child := range ctx.GetChildren() {
		if expr, ok := child.(parser.IExpressionContext); ok {
			exprs = append(exprs, expr)
		}
	}
	if ctx.NODE() != nil && len(exprs) > 1 {
		return exprs[1]
	}
	if len(exprs) > 0 {
		return exprs[0]
	}
	return nil
}
