// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"regexp"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// quotedExpressionText is the change of meaning of a quoted value in a
// property that holds ONE Mendix expression (ako/mxcli#836).
//
// Before #750 these properties took the expression's TEXT in a string, its own
// quotes doubled:
//
//	dynamicclasses: 'if $currentObject/F then ''on'' else '''''
//	HttpPassword: '@Mod.ApiPassword'
//
// #750 made them first-class expressions, written as-is, so a quoted value is
// a Mendix string — and refused the old spelling outright (MDL-WIDGET33,
// MDL-ODATA07), in every script. That broke committed scripts without the
// header, which ADR-0011 forbids. Under mdl 0 the old spelling keeps its
// meaning — the literal's content is the expression — and warns; `fmt
// --upgrade` writes the content bare, which means the same under both
// versions. Only mdl 1 refuses it.
//
// Only a value whose content is expression text is the old spelling: for
// DynamicClasses / DynamicCellClass a `$`, a quote or a leading `if` (the
// MDL-WIDGET33 test) or an `@Module.Const`; for an OData client's
// HttpUsername / HttpPassword / ClientCertificate and header values doubled
// outer quotes or an `@Module.Const` (the MDL-ODATA07 test). Any other quoted
// value — a class name, a plain credential — is the string under both
// versions: its old meaning stored the bare word as the expression, which is
// no valid Mendix expression, so that write was wrong under every reading.
var quotedExpressionText = langver.Change{
	Code:  "MDL-V1-QUOTEDEXPR",
	Since: langver.V1,
	Old: "a quoted value holding expression text in an expression property (DynamicClasses, DynamicCellClass, " +
		"an OData client's HttpUsername / HttpPassword / ClientCertificate or header value) is that expression: " +
		"the outer quotes are dropped and the doubled ones undone",
	New: "a Mendix string, so the old spelling is refused (MDL-WIDGET33, MDL-ODATA07); " +
		"write the expression itself, without the outer quotes and with its own quotes single",
}

// quotedConstantRefRe matches a constant reference `@Module.Const` anywhere in
// the content of a quoted value: the whole `'@Module.Const'`, and the compound
// text the old describe printed for a stored expression. It quoted every
// expression (formatExprValue), so a header holding `'Bearer ' + @Module.Token`
// came out doubled at the start only, which the doubled-outer-quotes test
// misses:
//
//	'''Bearer '' + @Module.Token'
//
// The `@` must not follow a word character, a dot or another `@`, so an
// e-mail address (`user@example.com`) holds no constant, and a Tailwind
// `@container` has no dot.
var quotedConstantRefRe = regexp.MustCompile(`(^|[^A-Za-z0-9_.@])@[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+`)

// widgetExpressionTextRe is MDL-WIDGET33's test for the content of the old
// spelling: a class name or class list never holds a `$` or a quote, and does
// not start with `if`.
var widgetExpressionTextRe = regexp.MustCompile(`\$|'|^\s*if\b`)

// IsLegacyWidgetExpressionText reports whether the content of a quoted
// DynamicClasses / DynamicCellClass value is expression text: the pre-#750
// spelling. The executor's MDL-WIDGET33 asks the same question under mdl 1.
func IsLegacyWidgetExpressionText(content string) bool {
	return widgetExpressionTextRe.MatchString(content) || quotedConstantRefRe.MatchString(content)
}

// IsLegacyODataExpressionText is the same question for an OData client's
// credential or header value (MDL-ODATA07): doubled outer quotes, or a
// constant reference in the quoted text.
func IsLegacyODataExpressionText(content string) bool {
	return (len(content) >= 2 && strings.HasPrefix(content, "'") && strings.HasSuffix(content, "'")) ||
		quotedConstantRefRe.MatchString(content)
}

// loneLiteralToken returns the string literal a value node consists of, or
// nil when it is anything else.
func loneLiteralToken(n antlr.ParserRuleContext) antlr.Token {
	if n == nil || n.GetStart() == nil || n.GetStop() == nil {
		return nil
	}
	start := n.GetStart()
	if start.GetTokenType() != parser.MDLLexerSTRING_LITERAL || start.GetTokenIndex() != n.GetStop().GetTokenIndex() {
		return nil
	}
	return start
}

// legacyQuotedExpression returns the expression an mdl 0 quoted value holds:
// the literal's content, when valueNode is exactly one string literal whose
// content legacy reports as expression text, in a script written before mdl 1.
// Under mdl 1 it reports false, and the value is the string it spells.
func legacyQuotedExpression(valueNode antlr.ParserRuleContext, legacy func(string) bool) (string, antlr.Token, bool) {
	lit := loneLiteralToken(valueNode)
	if lit == nil || quotedExpressionText.Applies(languageVersionOf(valueNode)) {
		return "", nil, false
	}
	content := unquoteStringLit(valueNode) // mdl 0: the escapes the old reading interpreted
	if !legacy(content) {
		return "", nil, false
	}
	return content, lit, true
}

// noteQuotedExpression records MDL-V1-QUOTEDEXPR for a value kept at its mdl 0
// meaning, with the rewrite that writes the content bare. readsBack says
// whether the bare content reads back as exactly that expression; a literal
// holding an escape the string-escape rewrite owns (MDL-V1-ESCAPE) is left to
// the author, since both rewrites would edit the same token.
func (b *Builder) noteQuotedExpression(at antlr.ParserRuleContext, lit antlr.Token, content string, readsBack func() bool) {
	if b == nil || b.gate(quotedExpressionText, at) {
		return
	}
	switch {
	case hasInterpretedEscape(lit.GetText()):
		b.fixLastNote(quotedExpressionText.Code, nil,
			"the quoted expression holds a backslash escape; write the expression bare by hand")
	case !readsBack():
		b.fixLastNote(quotedExpressionText.Code, nil,
			"the quoted text "+lit.GetText()+" does not read back bare as the expression "+content+"; write it bare by hand")
	default:
		b.fixLastNote(quotedExpressionText.Code, &ast.Fix{Edits: []ast.TextEdit{replaceSpan(lit, lit, content)}}, "")
	}
}

// widgetExpressionPropValue is the value of DynamicClasses / DynamicCellClass:
// the expression as written, or under mdl 0 the content of the old quoted
// spelling (quotedExpressionText), which it notes when b is the script's
// builder. key is the property as written, for the read-back check.
func (b *Builder) widgetExpressionPropValue(key string, v antlr.ParserRuleContext) any {
	if content, lit, ok := legacyQuotedExpression(v, IsLegacyWidgetExpressionText); ok {
		b.noteQuotedExpression(v, lit, content, func() bool { return widgetExpressionReadsBack(key, content) })
		return content
	}
	return widgetExpressionValue(v)
}

// widgetExpressionReadsBack reports whether `key: expr` in a widget's
// properties stores exactly expr.
func widgetExpressionReadsBack(key, expr string) bool {
	ctx, ok := parseRule(key+": "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.WidgetPropertyV3()
	})
	if !ok {
		return false
	}
	v := lastRuleChild(ctx)
	if v == nil {
		return false
	}
	var none *Builder
	got, isText := none.widgetExpressionPropValue(key, v).(string)
	return isText && got == expr
}

// odataExpressionReadsBack reports whether `HttpUsername: expr` stores exactly
// expr.
func odataExpressionReadsBack(expr string) bool {
	ctx, ok := parseRule("HttpUsername: "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.OdataPropertyAssignment()
	})
	if !ok {
		return false
	}
	a := ctx.(*parser.OdataPropertyAssignmentContext)
	var none *Builder
	got, _ := none.odataExpressionValue(a.OdataPropertyValue(), a.Expression())
	return got == expr
}

// isOneStringLiteral reports whether expr is exactly one Mendix string
// literal: what odataExpressionValue reports as a literal when expr is written
// bare.
func isOneStringLiteral(expr string) bool {
	if len(expr) < 2 || expr[0] != '\'' || expr[len(expr)-1] != '\'' {
		return false
	}
	inner := expr[1 : len(expr)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\'' {
			if i+1 >= len(inner) || inner[i+1] != '\'' {
				return false
			}
			i++
		}
	}
	return true
}
