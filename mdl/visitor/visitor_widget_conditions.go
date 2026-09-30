// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R5 (ADR-0010; ako/mxcli#753): a page widget's conditional visibility and
// editability are client expressions, written bare — `Visible:
// $currentObject/Status = 'Open'` — and stored as written. `Visible: [expr]`
// wrote one in the brackets of an XPath constraint and rooted a bare
// attribute in $currentObject on the way in; it is the deprecated alias
// MDL-DEPR081, whose rewrite writes the expression the brackets stored.

func isBooleanLiteral(s string) bool {
	return strings.EqualFold(s, "true") || strings.EqualFold(s, "false")
}

func visibleOrEditable(visible, editable antlr.TerminalNode) antlr.TerminalNode {
	if visible != nil {
		return visible
	}
	return editable
}

// BareWidgetCondition reports whether `<key>: <expr>` (key Visible or
// Editable) reads back as a conditional expression storing exactly expr.
// describe asks it before writing a stored condition bare; an expression the
// plain-value reading claims (`true`, a lone name) or the `Attr in (…)` form
// claims does not, and is written in brackets instead.
func BareWidgetCondition(key, expr string) bool {
	if !BareExpression(expr) {
		return false
	}
	ctx, ok := parseRule(key+": "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.WidgetPropertyV3()
	})
	if !ok {
		return false
	}
	w := ctx.(*parser.WidgetPropertyV3Context)
	if visibleOrEditable(w.VISIBLE(), w.EDITABLE()) == nil || w.Expression() == nil {
		return false
	}
	return bareArgumentText(w.Expression()) == expr
}

// ExitWidgetPropertyV3 records a bracketed Visible / Editable in a widget's
// properties.
func (b *Builder) ExitWidgetPropertyV3(ctx *parser.WidgetPropertyV3Context) {
	b.recordBracketedWidgetCondition(visibleOrEditable(ctx.VISIBLE(), ctx.EDITABLE()), ctx.XpathConstraint())
}

// ExitAlterPageAssignment records a bracketed Visible / Editable in `alter
// page … set (…)`.
func (b *Builder) ExitAlterPageAssignment(ctx *parser.AlterPageAssignmentContext) {
	b.recordBracketedWidgetCondition(visibleOrEditable(ctx.VISIBLE(), ctx.EDITABLE()), ctx.XpathConstraint())
}

// recordBracketedWidgetCondition records MDL-DEPR081 with the rewrite that
// replaces the brackets by the expression they store. There is none when that
// expression would not read back bare as itself.
func (b *Builder) recordBracketedWidgetCondition(kw antlr.TerminalNode, xc parser.IXpathConstraintContext) {
	if kw == nil || xc == nil {
		return
	}
	stored := buildConditionalExpression(xc)
	if isBooleanLiteral(stored) {
		// `Editable: [false]` stores a constant CONDITION; bare, `false` is the
		// plain value, which Studio Pro stores differently. A constant
		// condition has no bare spelling, so the brackets are not an alias
		// for one here.
		return
	}
	b.recordDeprecation(deprecation.BracketedWidgetCondition, kw.GetSymbol(), kw.GetText())
	key := "Visible"
	if kw.GetSymbol().GetTokenType() == parser.MDLLexerEDITABLE {
		key = "Editable"
	}
	if holdsInterpretedEscape(xc) || !BareWidgetCondition(key, stored) {
		b.fixLastDeprecation(deprecation.BracketedWidgetCondition, nil,
			"the condition "+nodeText(xc)+" stores "+stored+", which does not read back as the same bare expression; "+
				"write it bare by hand")
		return
	}
	b.fixLastDeprecation(deprecation.BracketedWidgetCondition,
		&ast.Fix{Edits: []ast.TextEdit{replaceSpan(xc.GetStart(), xc.GetStop(), stored)}}, "")
}
