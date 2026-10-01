// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// Double quotes in an XPath.
//
// MDL lets a name be double-quoted to escape a parser keyword, and the habit
// carries into XPath: `["Status" = 'Accepted']`. In a Mendix XPath a string is
// written in single quotes, so a double-quoted token is never a string the
// author can mean. On the LEFT of a comparison it is a quoted name and its
// quotes are stripped (stripExpressionIdentifierQuotes), as every XPath sink
// does. On the RIGHT it is a value: Mendix XPath never compares one member with
// another, so `Name = "ServiceManager"` is a string written with the wrong
// quotes. Stripping it like a name stored `Name = ServiceManager`, a member
// path (ako/mxcli#566); keeping it stored a constraint Mendix rejects or reads
// as a literal. Either is a wrong write, so it is refused, under every
// language version (ADR-0011: refusing a silent wrong write).

// xpathQuotedValues returns the double-quoted tokens that stand as the
// right-hand operand of a comparison in an XPath source, outside its
// single-quoted string literals (a doubled apostrophe is the escape inside one).
func xpathQuotedValues(src string) []string {
	var out []string
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '\'' {
			if inString && i+1 < len(src) && src[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		opEnd := -1
		switch {
		case c == '!' && i+1 < len(src) && src[i+1] == '=':
			opEnd = i + 2
		case c == '<' || c == '>':
			opEnd = i + 1
			if i+1 < len(src) && src[i+1] == '=' {
				opEnd = i + 2
			}
		case c == '=':
			opEnd = i + 1
		}
		if opEnd < 0 {
			continue
		}
		j := opEnd
		for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r' || src[j] == '\n') {
			j++
		}
		if j < len(src) && src[j] == '"' {
			if k := strings.IndexByte(src[j+1:], '"'); k >= 0 {
				out = append(out, src[j+1:j+1+k])
				i = j + 1 + k
				continue
			}
		}
		i = opEnd - 1
	}
	return out
}

// refuseXPathQuotedValues records one error per double-quoted comparison value
// in an XPath. inString says the XPath was written as an MDL string literal,
// where the single quotes the fix needs are doubled.
func (b *Builder) refuseXPathQuotedValues(src string, at antlr.Token, inString bool) {
	for _, v := range xpathQuotedValues(src) {
		fix := "'" + strings.ReplaceAll(v, "'", "''") + "'"
		if inString {
			fix = strings.ReplaceAll(fix, "'", "''") + " inside the quoted XPath (or " + fix + " in the bracketed form)"
		}
		b.addError(fmt.Errorf("line %d:%d: %s in an XPath is a double-quoted name, not a string: "+
			"Mendix XPath writes a string in single quotes, and a name cannot be compared with another — "+
			"write %s (ako/mxcli#566)", at.GetLine(), at.GetColumn(), `"`+v+`"`, fix))
	}
}

// ExitXpathConstraint refuses a double-quoted comparison value in any
// bracketed XPath — a retrieve, a grant, a data source, a workflow's targeting.
// Only the outermost group is scanned; a nested predicate is part of its text.
func (b *Builder) ExitXpathConstraint(ctx *parser.XpathConstraintContext) {
	for p := ctx.GetParent(); p != nil; p = p.GetParent() {
		if _, nested := p.(*parser.XpathConstraintContext); nested {
			return
		}
	}
	strict := lexedWithStrictEscapes(ctx)
	src := storedExpressionSource(stripMDLComments(extractOriginalText(ctx), strict), strict)
	b.refuseXPathQuotedValues(src, ctx.GetStart(), false)
}

// refuseQuotedXPathStringValues is ExitXpathConstraint for an XPath written as
// an MDL string literal (`where '[…]'`), given its STRING_LITERAL token.
func (b *Builder) refuseQuotedXPathStringValues(lit antlr.Token) {
	if lit == nil || lit.GetTokenType() != parser.MDLLexerSTRING_LITERAL {
		return
	}
	b.refuseXPathQuotedValues(unquoteStringLit(antlr.NewTerminalNodeImpl(lit)), lit, true)
}

// refuseRetrieveStringXPathValues covers `retrieve … where '<xpath>'`: the
// where clause is then an expression that is one string literal.
func (b *Builder) refuseRetrieveStringXPathValues(ctx *parser.RetrieveStatementContext) {
	if ctx.WHERE() == nil {
		return
	}
	for _, e := range ctx.AllExpression() {
		if e == ctx.GetLimitExpr() || e == ctx.GetOffsetExpr() {
			continue
		}
		if start := e.GetStart(); start != nil && start == e.GetStop() {
			b.refuseQuotedXPathStringValues(start)
		}
	}
}
