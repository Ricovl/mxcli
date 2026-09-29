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
		b.langNotes[len(b.langNotes)-1].Fix, b.langNotes[len(b.langNotes)-1].NoFix = b.escapeFix(t)
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

// VisitTerminal collects the string literals of the tree for escapeFix.
func (b *Builder) VisitTerminal(node antlr.TerminalNode) {
	if t := node.GetSymbol(); t != nil && t.GetTokenType() == parser.MDLLexerSTRING_LITERAL {
		if b.stringLits == nil {
			b.stringLits = map[int]antlr.TerminalNode{}
		}
		b.stringLits[t.GetTokenIndex()] = node
	}
}

// escapeFix is the rewrite that keeps an mdl 0 string literal's meaning under
// mdl 1, where a backslash is an ordinary character.
//
// What the literal means under mdl 0 depends on where it is. On its own (a
// name, a caption) and in an expression the builder re-renders, it is its
// unescaped value, so the rewrite writes that value with a doubled apostrophe as the only
// escape. In an expression the builder stores as written — one that spans
// lines (shouldPreserveExpressionSource) — mdl 0 already passes the backslash
// through to Mendix, exactly as mdl 1 does, so nothing changes.
//
// An escaped line break in a re-rendered expression has no rewrite: writing the
// break into the source makes the builder store the expression as written,
// which is not always what it wrote before. The exception is a text template
// written as one literal (mdl-examples/bug-tests/264-log-node-expression-roundtrip.mdl):
// under mdl 1 that literal is the template text whether or not it spans lines
// (templateLineBreak, #746), which is what the one-line literal was under mdl 0.
func (b *Builder) escapeFix(t antlr.Token) (*ast.Fix, string) {
	requoted := requoteForV1(t.GetText())
	rewrite := &ast.Fix{Edits: []ast.TextEdit{replaceSpan(t, t, requoted)}}
	node := b.stringLits[t.GetTokenIndex()]
	if node == nil {
		return rewrite, ""
	}
	var top antlr.ParserRuleContext
	for p := node.GetParent(); p != nil; p = p.GetParent() {
		if e, ok := p.(*parser.ExpressionContext); ok {
			top = e
		}
	}
	if top == nil {
		return rewrite, ""
	}
	source := strings.TrimSpace(extractExpressionText(top))
	if shouldPreserveExpressionSource(source) {
		return &ast.Fix{}, ""
	}
	if strings.ContainsAny(requoted, "\r\n") && !isTemplateMessage(top) {
		return nil, "under mdl 1 the line break is written into the string itself, which makes the expression " +
			"one that is stored as written rather than re-rendered, and that can change what it builds; " +
			"rewrite it by hand"
	}
	return rewrite, ""
}

// requoteForV1 writes an mdl 0 string literal so that it has the same value
// under mdl 1, where a backslash is an ordinary character and a doubled apostrophe the only
// escape.
func requoteForV1(lit string) string {
	return "'" + strings.ReplaceAll(unquoteString(lit), "'", "''") + "'"
}

// isTemplateMessage reports whether expr is, in full, the message of a log,
// show message or validation feedback and one string literal: the text of a
// template, which a line break does not turn into an expression under mdl 1.
func isTemplateMessage(expr antlr.ParserRuleContext) bool {
	e, ok := expr.(*parser.ExpressionContext)
	if !ok || loneStringLiteral(e) == nil {
		return false
	}
	switch p := e.GetParent().(type) {
	case *parser.LogStatementContext:
		return logMessageExpression(p) == e
	case *parser.ShowMessageStatementContext:
		return p.Expression() == e
	case *parser.ValidationFeedbackStatementContext:
		return p.Expression() == e
	}
	return false
}
