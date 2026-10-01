// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
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
//
// The terminators are read off the statement's last tokens rather than its
// own SEMICOLON and SLASH: the microflow, nanoflow and workflow rules end in
// `SEMICOLON? SLASH?` themselves, and `create java action … as $$…$$;` in
// `SEMICOLON?`, so either can belong to the inner rule.
func (b *Builder) ExitStatement(ctx *parser.StatementContext) {
	b.exitStatementDocs(ctx)
	last := lastTerminals(ctx, 2)
	if len(last) == 0 {
		return
	}
	var slash antlr.Token
	if last[0].GetTokenType() == parser.MDLParserSLASH {
		slash, last = last[0], last[1:]
	}
	if len(last) > 0 && last[0].GetTokenType() != parser.MDLParserSEMICOLON {
		if b.gate(semicolonRequired, ctx) {
			b.addError(fmt.Errorf("line %d: the statement ending at %q has no terminating `;`: "+
				"under %s every statement ends with `;`", last[0].GetLine(), last[0].GetText(), b.langVersion))
		} else {
			b.fixLastNote(semicolonRequired.Code, &ast.Fix{Edits: []ast.TextEdit{insertAt(last[0].GetStop()+1, ";")}}, "")
		}
	}
	if slash != nil {
		if b.gate(slashIsNotATerminator, ctx) {
			b.addError(fmt.Errorf("line %d: `/` is not a statement terminator under %s; end the statement "+
				"with `;` and delete the `/` line", slash.GetLine(), b.langVersion))
		} else {
			b.fixLastNote(slashIsNotATerminator.Code, &ast.Fix{Edits: []ast.TextEdit{slashLineFix(slash)}}, "")
		}
	}
}

// lastTerminals returns up to n of the tree's last tokens, the last first.
func lastTerminals(tree antlr.Tree, n int) []antlr.Token {
	var out []antlr.Token
	var walk func(antlr.Tree)
	walk = func(t antlr.Tree) {
		if len(out) == n {
			return
		}
		if tn, ok := t.(antlr.TerminalNode); ok {
			if tok := tn.GetSymbol(); tok != nil && tok.GetTokenType() != antlr.TokenEOF {
				out = append(out, tok)
			}
			return
		}
		for i := t.GetChildCount() - 1; i >= 0 && len(out) < n; i-- {
			walk(t.GetChild(i))
		}
	}
	walk(tree)
	return out
}
