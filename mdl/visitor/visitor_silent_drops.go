// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// Forms that parsed, passed `check`, and were then dropped or stored as
// something else (ako/mxcli#706). Each is refused here, where the parse tree
// says exactly what was written, rather than removed from the grammar: the
// words involved are also keywords-as-identifiers (`Currency`, `Date`, `throw`),
// so deleting the alternative would let several of them re-parse as something
// else — an enumeration called `float`, say — which is the same silence again.
//
// None of them ever worked, so refusing them changes no script's meaning and
// is not gated on the `mdl` header (ADR-0011).

// ctxPos formats the start of a rule as the `line L:C` prefix syntax errors use.
func ctxPos(ctx antlr.ParserRuleContext) string {
	tok := ctx.GetStart()
	return fmt.Sprintf("line %d:%d", tok.GetLine(), tok.GetColumn())
}

// ExitThrowStatement refuses `throw <expr>`. The rule had no listener, so the
// statement disappeared from the microflow it was written in. Mendix has no
// action that raises a new error carrying a value: inside an error handler an
// error end event re-raises the error being handled (`raise error`); on the
// main flow the only way to fail is a Java action that throws.
func (b *Builder) ExitThrowStatement(ctx *parser.ThrowStatementContext) {
	b.addError(fmt.Errorf("%s: `throw` is not a Mendix action and was never written to the microflow — "+
		"it was parsed and dropped.\n"+
		"  Inside an `on error begin … end error` handler, re-raise the error being handled:\n"+
		"    raise error;\n"+
		"  On the main flow Mendix has no throw: call a Java action that throws, or\n"+
		"  report the problem with `validation feedback` / `log error` and return.", ctxPos(ctx)))
}

// removedPrimitiveType reports the replacement for a type word Mendix does not
// have, or "" when the token is a real type.
//
// float and currency were Mendix 6 attribute types, removed in Mendix 7 in
// favour of Decimal. There is no date-only attribute type — Date is a DateTime.
// mxcli mapped float/currency to String(unlimited) on an attribute (Void in a
// microflow) and date to DateTime, all without a word.
func removedPrimitiveType(floatTok, currencyTok, dateTok antlr.TerminalNode) (word, replacement string) {
	switch {
	case floatTok != nil:
		return floatTok.GetText(), "Decimal"
	case currencyTok != nil:
		return currencyTok.GetText(), "Decimal"
	case dateTok != nil:
		return dateTok.GetText(), "DateTime"
	}
	return "", ""
}

func (b *Builder) rejectRemovedPrimitiveType(ctx antlr.ParserRuleContext, word, replacement string) {
	if word == "" {
		return
	}
	why := "Mendix has no " + word + " type; it was removed in Mendix 7 and mxcli stored it as the wrong type"
	if replacement == "DateTime" {
		why = "Mendix has no date-only type; mxcli silently stored it as DateTime"
	}
	b.addError(fmt.Errorf("%s: type `%s` is not supported — %s.\n  Write `%s` instead.",
		ctxPos(ctx), word, why, replacement))
}

// EnterDataType covers every place a type is written: attributes, microflow
// parameters and return types, declare, constants, and the service rules.
func (b *Builder) EnterDataType(ctx *parser.DataTypeContext) {
	word, repl := removedPrimitiveType(ctx.FLOAT_TYPE(), ctx.CURRENCY_TYPE(), ctx.DATE_TYPE())
	b.rejectRemovedPrimitiveType(ctx, word, repl)
}

// EnterNonListDataType is the same check for the create-object type slot.
func (b *Builder) EnterNonListDataType(ctx *parser.NonListDataTypeContext) {
	word, repl := removedPrimitiveType(ctx.FLOAT_TYPE(), ctx.CURRENCY_TYPE(), ctx.DATE_TYPE())
	b.rejectRemovedPrimitiveType(ctx, word, repl)
}

// rejectParenthesisedAssociation refuses `association X (from … to …, opt, …)`.
// The visitor read only the unparenthesised options, so every option in this
// form was dropped: a ReferenceSet with table storage was stored as a Reference
// in a column. The message rewrites the statement in the form that works.
func (b *Builder) rejectParenthesisedAssociation(ctx *parser.CreateAssociationStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) < 3 {
		return
	}
	var opts []string
	for _, o := range ctx.AllAssociationOption() {
		var toks []string
		collectLeafTokens(o, &toks)
		kept := toks[:0]
		for _, t := range toks {
			if t != ":" {
				kept = append(kept, t)
			}
		}
		opts = append(opts, strings.Join(kept, " "))
	}
	canonical := fmt.Sprintf("create association %s from %s to %s",
		names[0].GetText(), names[1].GetText(), names[2].GetText())
	if len(opts) > 0 {
		canonical += " " + strings.Join(opts, " ")
	}
	b.addError(fmt.Errorf("%s: the parenthesised association form is not supported — its options "+
		"were parsed and dropped, so a ReferenceSet was stored as a Reference.\n"+
		"  Write the options after the entities, without parentheses or colons:\n"+
		"    %s;", ctxPos(ctx), canonical))
}
