// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/suggest"
)

// statementWords are the words a statement starts with, for suggesting the
// one a misspelt first word was meant to be.
var statementWords = []string{
	"create", "alter", "drop", "describe", "list", "show", "grant", "revoke",
	"rename", "move", "update", "refresh", "search", "select", "import",
	"define", "execute", "help", "exit",
}

// unknownStatementWord reports the word a statement failed to start with: an
// IDENTIFIER that no statement alternative accepts (R7, ako/mxcli#755). Only a
// failure at the start of a statement qualifies — the parser is still in the
// statement (or program) rule, and ANTLR's "no viable alternative" names that
// one word alone — since the same word mid-statement is a different mistake.
func unknownStatementWord(rec antlr.Recognizer, sym any, msg string) (string, bool) {
	tok, ok := sym.(antlr.Token)
	if !ok || tok.GetTokenType() != parser.MDLParserIDENTIFIER {
		return "", false
	}
	p, ok := rec.(antlr.Parser)
	if !ok {
		return "", false
	}
	switch p.GetParserRuleContext().(type) {
	case *parser.StatementContext, *parser.ProgramContext:
	default:
		return "", false
	}
	if msg != "no viable alternative at input '"+tok.GetText()+"'" {
		return "", false
	}
	return tok.GetText(), true
}

// unknownStatementMessage is the error for a statement starting with word.
func unknownStatementMessage(word string) string {
	msg := fmt.Sprintf("unknown statement '%s'", word)
	if near := suggest.Closest(word, statementWords); near != "" {
		return msg + fmt.Sprintf(" — did you mean '%s'?", near)
	}
	return msg + " — a statement starts with a keyword such as create, alter, drop, describe or list"
}
