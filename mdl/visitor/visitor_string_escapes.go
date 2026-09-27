// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"sort"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// backslashIsLiteral is ADR-0010 R11's string rule: a doubled apostrophe is the only escape in
// a string literal. Under mdl 0 a backslash also escapes — `\n` is a newline,
// `\'` an apostrophe, `\\` one backslash — which contradicts Mendix's own
// expressions and makes `'C:\temp'` a tab. It changes what text means, so it is
// tied to the header (#732).
var backslashIsLiteral = langver.Change{
	Code:  "MDL-V1-ESCAPE",
	Since: langver.V1,
	Old:   "a backslash in a string literal is an escape (`\\n` is a newline, `\\t` a tab, `\\'` an apostrophe, `\\\\` one backslash)",
	New:   "an ordinary character, as in a Mendix expression; `''` is the only escape",
}

// newScriptStream is the character stream a script is lexed from. Whether a
// backslash escapes decides where a string literal ends, so the rule is fixed
// here, from the header, before the first token (see StrictEscapeStream).
func newScriptStream(input string) antlr.CharStream {
	var is antlr.CharStream = antlr.NewInputStream(input)
	if backslashIsLiteral.Applies(langver.ScanHeader(input)) {
		is = &parser.StrictEscapeStream{CharStream: is}
	}
	return is
}

// unquoteStringLit is the value of a STRING_LITERAL (a terminal node, or a
// rule whose text is one), read under the escape rule it was lexed with.
func unquoteStringLit(n interface{ GetText() string }) string {
	text := n.GetText()
	if !lexedWithStrictEscapes(n) {
		return unquoteString(text)
	}
	if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
		text = text[1 : len(text)-1]
	}
	return strings.ReplaceAll(text, "''", "'")
}

func lexedWithStrictEscapes(n any) bool {
	var tok antlr.Token
	switch x := n.(type) {
	case antlr.TerminalNode:
		tok = x.GetSymbol()
	case antlr.ParserRuleContext:
		tok = x.GetStart()
	}
	return tok != nil && parser.HasStrictEscapes(tok.GetInputStream())
}

// noteBackslashEscapes records MDL-V1-ESCAPE for every string literal of an
// mdl 0 script whose value would differ under mdl 1: one holding an escape
// unquoteString interprets. A backslash before any other character is kept
// as written under both, so it is not reported.
func (b *Builder) noteBackslashEscapes(tokens []antlr.Token) {
	if backslashIsLiteral.Applies(b.langVersion) {
		return
	}
	added := false
	for _, t := range tokens {
		if t.GetTokenType() != parser.MDLLexerSTRING_LITERAL || !hasInterpretedEscape(t.GetText()) {
			continue
		}
		b.langNotes = append(b.langNotes, ast.LanguageNote{
			Line:    t.GetLine(),
			Code:    backslashIsLiteral.Code,
			Message: "the string " + t.GetText() + ": " + backslashIsLiteral.Warning(b.langVersion),
		})
		added = true
	}
	if added {
		sort.SliceStable(b.langNotes, func(i, j int) bool { return b.langNotes[i].Line < b.langNotes[j].Line })
	}
}

// hasInterpretedEscape reports whether an mdl 0 string literal's text holds a
// backslash escape unquoteString turns into something else.
func hasInterpretedEscape(lit string) bool {
	for i := 0; i+1 < len(lit); i++ {
		if lit[i] != '\\' {
			continue
		}
		switch lit[i+1] {
		case 'n', 'r', 't', '\\', '\'':
			return true
		}
	}
	return false
}
