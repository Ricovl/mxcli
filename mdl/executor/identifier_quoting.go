// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	antlr "github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// mdlQuoted renders s as a complete MDL single-quoted string literal, doubling
// every embedded quote — the escape the MDL lexer requires (`'it”s'`), and the
// one Mendix Studio Pro's own expression syntax uses. Backslashes are NOT an
// escape here.
//
// It returns the quotes as well as the escaped body, deliberately. The
// describers used to write `fmt.Sprintf("... '%s'", escapeMe)` and escape the
// argument separately at each site, which is one thing to remember per emit —
// and six of twenty-three sites did not. The worst was a user task's
// `targeting users xpath`, whose payload is an XPath containing quoted
// constraints (`[System.UserRoles = '[%UserRole_Banker%]']`), so DESCRIBE
// WORKFLOW output would not re-parse for any workflow using one
// (mendixlabs/mxcli#1006). Keeping the quotes and the escaping in one function
// makes an unescaped emit impossible to write by omission.
//
// A companion test asserts no `'%s'` remains in the describers.
func mdlQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// mdlQuote writes s as the string literal that reads back as s in the
// language describe writes (describeLanguage): the one place that decides how
// describe spells a string value, so that the freeze of mdl 1 (langver.Frozen)
// switches every describer at once (ako/mxcli#804).
//
// Under mdl 1 a backslash is an ordinary character and a doubled apostrophe
// the only escape (ADR-0010 R11), so the value is written as it is, a line
// break included: mdlQuoted. Under mdl 0 a backslash escapes, so a backslash,
// line break, carriage return and tab are written as `\\`, `\n`, `\r` and
// `\t` (mdl0Quote); written as they are, the mdl 0 reader would turn a
// stored `C:\temp` into a tab.
func mdlQuote(ctx *ExecContext, s string) string {
	if describeLanguage(ctx) >= langver.V1 {
		return mdlQuoted(s)
	}
	return mdl0Quote(s)
}

// mdlIdent renders name as an MDL identifier suitable for DESCRIBE output,
// double-quoting it when it would not lex as a bare IDENTIFIER — e.g. when it
// collides with a reserved keyword ("List", "Column", "Template", …). This keeps
// DESCRIBE output re-parseable by `mxcli check`. See issue #619.
//
// The reserved set is not hardcoded: name is run through the actual MDL lexer,
// so the check stays correct as the grammar's keyword set evolves and never
// produces false positives (a widget named "Dot" lexes as IDENTIFIER, not the
// DOT punctuation token, so it is left unquoted).
func mdlIdent(name string) string {
	if name == "" || lexesAsBareIdentifier(name) {
		return name
	}
	// QUOTED_IDENTIFIER is '"' ~["\r\n]* '"' — no escape sequence, and Mendix
	// element names never contain a double quote, so plain wrapping is safe.
	return `"` + name + `"`
}

// lexesAsBareIdentifier reports whether name lexes as a single IDENTIFIER token
// spanning the whole string — i.e. it is safe to emit unquoted. Unlike
// isBareIdentifier (which only checks the character shape), this also rejects
// reserved keywords such as "List", because they lex to a keyword token.
func lexesAsBareIdentifier(name string) bool {
	lexer := parser.NewMDLLexer(antlr.NewInputStream(name))
	lexer.RemoveErrorListeners()
	tokens := lexer.GetAllTokens()
	if len(tokens) != 1 {
		return false
	}
	t := tokens[0]
	return t.GetTokenType() == parser.MDLLexerIDENTIFIER &&
		t.GetStart() == 0 && t.GetStop() == len(name)-1
}
