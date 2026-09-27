// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// ADR-0010 R11: `;` terminates every statement, and the SQL*Plus `/` line is
// not a terminator. The grammar keeps both optional (`SEMICOLON? SLASH?`) so a
// headerless script parses exactly as before; the version decides here.

// semicolonRequired is the new rejection of a statement without `;`.
var semicolonRequired = langver.Change{
	Code:  "MDL-V1-SEMI",
	Since: langver.V1,
	Old:   "a statement without a terminating `;` is accepted",
	New:   "an error: every statement ends with `;`",
}

// slashIsNotATerminator is the new rejection of the SQL*Plus `/` line.
var slashIsNotATerminator = langver.Change{
	Code:  "MDL-V1-SLASH",
	Since: langver.V1,
	Old:   "a `/` after a statement is accepted as a terminator (SQL*Plus style)",
	New:   "an error: `;` is the only statement terminator",
}

// ExitStatement applies R11's terminator rules to one top-level statement.
func (b *Builder) ExitStatement(ctx *parser.StatementContext) {
	if !endsWithSemicolon(ctx) {
		if at := ctx.GetStop(); at != nil && b.gate(semicolonRequired, ctx) {
			b.addError(fmt.Errorf("line %d: the statement ending at %q has no terminating `;`: "+
				"under %s every statement ends with `;`", at.GetLine(), at.GetText(), b.langVersion))
		}
	}
	if s := ctx.SLASH(); s != nil && b.gate(slashIsNotATerminator, ctx) {
		b.addError(fmt.Errorf("line %d: `/` is not a statement terminator under %s; end the statement "+
			"with `;` and delete the `/` line", s.GetSymbol().GetLine(), b.langVersion))
	}
}

// endsWithSemicolon reports whether the statement is terminated by `;`: its own
// SEMICOLON, or one a statement rule consumed itself (`create java action …
// as $$…$$;` ends in `SEMICOLON?` inside its rule, before the statement's).
func endsWithSemicolon(ctx *parser.StatementContext) bool {
	if ctx.SEMICOLON() != nil {
		return true
	}
	for i := ctx.GetChildCount() - 1; i >= 0; i-- {
		switch c := ctx.GetChild(i).(type) {
		case antlr.TerminalNode:
			if c.GetSymbol().GetTokenType() == parser.MDLParserSLASH {
				continue
			}
			return c.GetSymbol().GetTokenType() == parser.MDLParserSEMICOLON
		case antlr.ParserRuleContext:
			stop := c.GetStop()
			return stop != nil && stop.GetTokenType() == parser.MDLParserSEMICOLON
		}
	}
	return false
}
