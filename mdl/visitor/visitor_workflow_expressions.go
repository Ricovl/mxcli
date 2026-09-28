// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"unicode"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R5 (ADR-0010; ako/mxcli#753): an expression is written bare, never in a
// string. A workflow stores five of them — a decision's condition, `wait for
// timer`'s delay, a timer boundary event's delay, a timer event sub-process's
// first execution time and a due date — and each took a string whose CONTENT
// was the expression. The bare form stores its source text, as a bare workflow
// call argument does (#751); the string form is the deprecated alias
// MDL-DEPR080 and keeps its meaning under every language version, since none
// of these slots takes a string-valued expression.

// workflowExpressionText is the expression a workflowExpression stores: the
// string's content for the deprecated form, the source text for the bare one.
func workflowExpressionText(ctx parser.IWorkflowExpressionContext) string {
	if ctx == nil {
		return ""
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		return unquoteStringLit(lit)
	}
	if e := ctx.Expression(); e != nil {
		return bareArgumentText(e)
	}
	return ""
}

// activityNameAndExpression reads the `name? expression?` slot pair of a
// decision and a `wait for timer`. An identifier directly followed by `(` is
// a function call, not a name and a parenthesised expression: `decision
// length($s) > 0` is one expression. The grammar alone reads it as the name
// `length` and the expression `($s) > 0`, since both parses are valid and the
// name comes first.
func activityNameAndExpression(name parser.IWorkflowActivityNameContext, expr parser.IWorkflowExpressionContext) (string, string) {
	if name == nil || expr == nil || expr.Expression() == nil {
		return workflowActivityNameText(name), workflowExpressionText(expr)
	}
	nameStop, exprStart := name.GetStop(), expr.GetStart()
	if nameStop != nil && exprStart != nil && exprStart.GetTokenType() == parser.MDLLexerLPAREN &&
		nameStop.GetStop()+1 == exprStart.GetStart() {
		joined := extractExpressionText(name) + extractExpressionText(expr)
		if ctx, ok := parseRule(joined, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.Expression() }); ok {
			return "", bareArgumentText(ctx.(parser.IExpressionContext))
		}
	}
	return workflowActivityNameText(name), workflowExpressionText(expr)
}

// WorkflowDecisionReadsBack reports whether `decision<nameClause> <expr>`
// builds a decision with exactly that expression, and a name when nameClause
// (empty, or a space and the name as describe writes it) has one. describe
// asks it before writing a decision's condition bare: an expression that
// starts with a word can otherwise be read as the activity's name.
func WorkflowDecisionReadsBack(nameClause, expr string) bool {
	ctx, ok := parseRule("decision"+nameClause+" "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.WorkflowDecisionStmt()
	})
	if !ok {
		return false
	}
	n := buildWorkflowDecision(ctx.(parser.IWorkflowDecisionStmtContext))
	return n.Expression == expr && (n.Name == "") == (nameClause == "")
}

// WorkflowWaitForTimerReadsBack is WorkflowDecisionReadsBack for `wait for
// timer<nameClause> <expr>`.
func WorkflowWaitForTimerReadsBack(nameClause, expr string) bool {
	ctx, ok := parseRule("wait for timer"+nameClause+" "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.WorkflowWaitForTimerStmt()
	})
	if !ok {
		return false
	}
	n := buildWorkflowWaitForTimer(ctx.(parser.IWorkflowWaitForTimerStmtContext))
	return n.DelayExpression == expr && (n.Name == "") == (nameClause == "")
}

// ExitWorkflowExpression records MDL-DEPR080 on an expression written in a
// string, with the rewrite that takes it out.
func (b *Builder) ExitWorkflowExpression(ctx *parser.WorkflowExpressionContext) {
	lit := ctx.STRING_LITERAL()
	if lit == nil || unquoteStringLit(lit) == "" {
		// `''` is no expression at all — `alter workflow … set due date ''`
		// clears it — so it is not one written in a string. It has no bare
		// spelling and stays as it is.
		return
	}
	b.recordDeprecation(deprecation.WorkflowStringExpression, lit.GetSymbol(), "workflow expression")
	fix, why := workflowStringExpressionFix(ctx, lit)
	b.fixLastDeprecation(deprecation.WorkflowStringExpression, fix, why)
}

// workflowStringExpressionFix replaces the string by its content. It has none
// when the content is not a bare expression that reads back as itself, or —
// for a decision or `wait for timer` with no name — when the bare expression
// would be read as the activity's name.
func workflowStringExpressionFix(ctx *parser.WorkflowExpressionContext, lit antlr.TerminalNode) (*ast.Fix, string) {
	if holdsInterpretedEscape(lit) {
		return nil, "the expression string holds a backslash escape; write the expression bare by hand"
	}
	expr := unquoteStringLit(lit)
	why := "the string " + lit.GetText() + " does not read back as the same bare expression; write it bare by hand"
	if !BareExpression(expr) {
		return nil, why
	}
	switch p := ctx.GetParent().(type) {
	case *parser.WorkflowDecisionStmtContext:
		if p.WorkflowActivityName() == nil && !WorkflowDecisionReadsBack("", expr) {
			return nil, why
		}
	case *parser.WorkflowWaitForTimerStmtContext:
		if p.WorkflowActivityName() == nil && !WorkflowWaitForTimerReadsBack("", expr) {
			return nil, why
		}
	}
	t := lit.GetSymbol()
	// A string needs no space to part it from its neighbours; a bare
	// expression does: `timer'x'comment 'c'`.
	if is := t.GetInputStream(); is != nil {
		if t.GetStart() > 0 && gluesToWord(is.GetText(t.GetStart()-1, t.GetStart()-1)) {
			expr = " " + expr
		}
		if t.GetStop()+1 < is.Size() && gluesToWord(is.GetText(t.GetStop()+1, t.GetStop()+1)) {
			expr += " "
		}
	}
	return &ast.Fix{Edits: []ast.TextEdit{{Start: t.GetStart(), Stop: t.GetStop() + 1, Text: expr}}}, ""
}

// gluesToWord reports whether a neighbouring character would lex as part of
// a bare expression's first or last token.
func gluesToWord(s string) bool {
	for _, r := range s {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_$'\"`%", r)
	}
	return false
}
