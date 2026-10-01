// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// Lowercase keywords are canonical (R8, ako/mxcli#752): describe writes them
// and `mxcli fmt` normalises to them. What makes that safe is telling a
// keyword apart from a NAME that happens to be spelled like one. The lexer
// cannot: `Title`, `Folder` and `User` are keyword tokens wherever they occur,
// and the old formatter upper-cased them all, turning `Issue64.User` into
// `Issue64.USER` — a different module member. Only the parse tree knows, so
// the spans come from it.

// nameRules are the rules through which a keyword token is used as a name, or
// whose text is carried into the model verbatim (an expression, XPath or OQL
// is stored as its source text, so its letter case is the author's). A data
// type (`String(200)`) is a Mendix type name, not a keyword.
var nameRules = map[string]bool{
	"keyword":             true,
	"identifierOrKeyword": true,
	"qualifiedName":       true,
	"dataType":            true,
}

// verbatimRules are the rules whose source text the visitor stores as written
// (extractOriginalText and friends): letter case AND layout are the author's.
var verbatimRules = map[string]bool{
	"expression":         true,
	"catalogSelectQuery": true,
	"sqlPassthrough":     true,
	// A template parameter keeps the whitespace after its expression
	// (appendTemplateParamTrailingWhitespace), so the whole list is kept.
	"templateParams": true,
}

// verbatimRulePrefixes are rule-name prefixes whose text is kept verbatim (an
// annotation, `@Position(…)`, is a name too).
var verbatimRulePrefixes = []string{"xpath", "oql", "annotation"}

// Spans is what the formatter needs from the parse tree.
type Spans struct {
	// Keywords are the rune spans [start, stop) of every keyword that is
	// syntax: not a name, not inside an expression, XPath or OQL, not a
	// property key (a keyword directly followed by `:` or `=` is a key, and
	// keys are case-preserved identifiers), and not a value written the way
	// Mendix writes names and enumeration values — in CamelCase
	// (`ReferenceSet`, `ButtonStyle: Success`) or with a digit
	// (`RenderMode: H2`). An all-caps or all-lower word is a keyword however
	// it is used.
	Keywords [][2]int
	// Verbatim are the rune spans of text stored as written — expressions,
	// XPath, OQL, catalog and SQL queries, and every token that spans lines
	// (a string literal, a code block) — whose layout must not change either.
	Verbatim [][2]int
}

// FormatSpans parses src and returns its Spans. ok is false when src does not
// parse; the caller must then leave it alone.
func FormatSpans(src string) (spans Spans, ok bool) {
	errs := newErrorListener()
	lexer := parser.NewMDLLexer(newScriptStream(src, langver.V0))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	tree := p.Program()
	if len(errs.errors) > 0 {
		return Spans{}, false
	}
	ruleNames := p.GetRuleNames()
	isVerbatimRule := func(name string) bool {
		if verbatimRules[name] {
			return true
		}
		for _, pre := range verbatimRulePrefixes {
			if strings.HasPrefix(name, pre) {
				return true
			}
		}
		return false
	}
	isKey := func(tok antlr.Token) bool {
		i := tok.GetTokenIndex() + 1
		if i >= stream.Size() {
			return false
		}
		switch stream.Get(i).GetTokenType() {
		case parser.MDLLexerCOLON, parser.MDLLexerEQUALS:
			return true
		}
		return false
	}
	// walk visits t; kept says a rule above t keeps its text as written.
	var walk func(t antlr.Tree, kept bool)
	walk = func(t antlr.Tree, kept bool) {
		switch n := t.(type) {
		case antlr.TerminalNode:
			tok := n.GetSymbol()
			if tok.GetTokenType() == antlr.TokenEOF {
				return
			}
			if strings.ContainsAny(tok.GetText(), "\n\r") {
				spans.Verbatim = append(spans.Verbatim, [2]int{tok.GetStart(), tok.GetStop() + 1})
			}
			if kept || !isKeywordToken(tok) || isKey(tok) || writtenAsAName(tok.GetText()) {
				return
			}
			spans.Keywords = append(spans.Keywords, [2]int{tok.GetStart(), tok.GetStop() + 1})
		case antlr.ParserRuleContext:
			name := ruleNames[n.GetRuleIndex()]
			if !kept && isVerbatimRule(name) {
				if start, stop := n.GetStart(), n.GetStop(); start != nil && stop != nil && stop.GetStop() >= start.GetStart() {
					spans.Verbatim = append(spans.Verbatim, [2]int{start.GetStart(), stop.GetStop() + 1})
				}
				kept = true
			}
			kept = kept || nameRules[name]
			for i := 0; i < n.GetChildCount(); i++ {
				walk(n.GetChild(i), kept)
			}
		}
	}
	walk(tree, false)
	return spans, true
}

// isKeywordToken reports whether tok is a keyword: a token other than an
// identifier whose text is a word (or words, for the multi-word tokens).
func isKeywordToken(tok antlr.Token) bool {
	switch tok.GetTokenType() {
	case antlr.TokenEOF, parser.MDLLexerIDENTIFIER, parser.MDLLexerQUOTED_IDENTIFIER:
		return false
	}
	text := tok.GetText()
	if text == "" || !(text[0] >= 'a' && text[0] <= 'z' || text[0] >= 'A' && text[0] <= 'Z') {
		return false
	}
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_',
			r == ' ', r == '\t', r == '\r', r == '\n':
		default:
			return false
		}
	}
	return true
}

// writtenAsAName reports whether a keyword token is spelled like a Mendix name
// or enumeration value: mixed case, or containing a digit.
func writtenAsAName(text string) bool {
	var upper, lower bool
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9':
			return true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		}
	}
	return upper && lower
}

// LowercaseKeywords returns src with every syntax keyword (Spans.Keywords) in
// lower case and everything else untouched.
func LowercaseKeywords(src string, keywords [][2]int) string {
	if len(keywords) == 0 {
		return src
	}
	runes := []rune(src)
	for _, s := range keywords {
		for i := s[0]; i < s[1] && i < len(runes); i++ {
			if r := runes[i]; r >= 'A' && r <= 'Z' {
				runes[i] = r + ('a' - 'A')
			}
		}
	}
	return string(runes)
}
