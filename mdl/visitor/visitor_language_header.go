// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// ExitLanguageHeader records the script's `mdl <n>;` header (ADR-0011).
//
// An unknown version is an error rather than a warning: running a script under
// rules older than the ones it was written for is the silent change of meaning
// the header exists to prevent.
func (b *Builder) ExitLanguageHeader(ctx *parser.LanguageHeaderContext) {
	word, num := ctx.IDENTIFIER(), ctx.NUMBER_LITERAL()
	if word == nil || num == nil {
		return // a syntax error has already been reported
	}
	// The grammar takes any word here so that `mdl` stays usable as a name;
	// only `mdl` is a header.
	if !strings.EqualFold(word.GetText(), "mdl") {
		b.addError(fmt.Errorf("line %d: `%s %s;` is not a statement; the only statement of this shape is "+
			"the language header, `mdl <n>;`", ctx.GetStart().GetLine(), word.GetText(), num.GetText()))
		return
	}
	n, err := strconv.Atoi(num.GetText())
	if errors.Is(err, strconv.ErrRange) {
		// All digits, only too large: a version this mxcli does not know.
		b.addError(fmt.Errorf("line %d: %s", ctx.GetStart().GetLine(), langver.UnknownVersionError(num.GetText())))
		return
	}
	if err != nil {
		b.addError(fmt.Errorf("line %d: the language version in `mdl %s;` must be a whole number, e.g. `mdl 1;`",
			ctx.GetStart().GetLine(), num.GetText()))
		return
	}
	v := langver.Version(n)
	if !v.Known() {
		b.addError(fmt.Errorf("line %d: %s", ctx.GetStart().GetLine(), langver.UnknownVersionError(num.GetText())))
		return
	}
	b.langVersion = v
	b.langHeaderLine = ctx.GetStart().GetLine()
	// The implicit header of a REPL or -c input (implicitHeaderSource) is on
	// no line of the input; the program still reads as headerless to
	// everything that asks where its header is written (fmt --upgrade).
	b.implicitHeader = b.langHeaderLine == 0
}

// gate decides which meaning a version-dependent construct gets. It returns
// true when the script's language version has the new meaning. Otherwise the
// caller keeps the old meaning, and gate records a warning naming the
// construct, so a script without the header lists everything whose meaning
// differs under the newer language (ADR-0011 decision 2).
//
// Every change of meaning or new rejection goes through here, declared as a
// langver.Change next to the code that implements both meanings:
//
//	var limitOneIsAList = langver.Change{Code: "MDL-V1-…", Since: langver.V1, …}
//	if b.gate(limitOneIsAList, ctx) { /* mdl 1 meaning */ } else { /* mdl 0 meaning */ }
func (b *Builder) gate(c langver.Change, ctx antlr.ParserRuleContext) bool {
	if c.Applies(b.langVersion) {
		return true
	}
	line := 0
	if ctx != nil && ctx.GetStart() != nil {
		line = ctx.GetStart().GetLine()
	}
	b.langNotes = append(b.langNotes, ast.LanguageNote{
		Line:    line,
		Code:    c.Code,
		Message: c.Warning(b.langVersion),
	})
	return false
}

// ExitRepeatedLanguageHeader accepts a language header after the first
// statement when it names the script's own version, and refuses one that
// names another (freeze decision 5, ako/mxcli#714). Every describe output
// starts with `mdl 1;`, so a file made by concatenating several of them
// repeats the header: that file is still one mdl 1 script. A header that
// changes the version part-way would make the statements around it mean
// different things in one file, which ADR-0011 rules out — and in a file whose
// first statement is no header, the script is mdl 0 and a later `mdl 1;`
// would not reach the statements above it.
func (b *Builder) ExitRepeatedLanguageHeader(ctx *parser.RepeatedLanguageHeaderContext) {
	word, num := ctx.IDENTIFIER(), ctx.NUMBER_LITERAL()
	if word == nil || num == nil {
		return // a syntax error has already been reported
	}
	line := ctx.GetStart().GetLine()
	if !strings.EqualFold(word.GetText(), "mdl") {
		b.addError(fmt.Errorf("line %d: `%s %s;` is not a statement; the only statement of this shape is "+
			"the language header, `mdl <n>;`", line, word.GetText(), num.GetText()))
		return
	}
	n, err := strconv.Atoi(num.GetText())
	if errors.Is(err, strconv.ErrRange) || err == nil && !langver.Version(n).Known() {
		b.addError(fmt.Errorf("line %d: %s", line, langver.UnknownVersionError(num.GetText())))
		return
	}
	if err != nil {
		b.addError(fmt.Errorf("line %d: the language version in `mdl %s;` must be a whole number, e.g. `mdl 1;`",
			line, num.GetText()))
		return
	}
	v := langver.Version(n)
	if v == b.langVersion {
		return // the same header again: concatenated output of one language
	}
	if b.langHeaderLine == 0 && !b.implicitHeader {
		b.addError(fmt.Errorf("line %d: `%s;` must be the first statement: this script starts without a "+
			"header, so it is %s, and a language header cannot change the version part-way through a script. "+
			"Move the header to the top (`mxcli fmt --upgrade` adds it and rewrites what it changes)",
			line, v, b.langVersion))
		return
	}
	where := fmt.Sprintf("line %d", b.langHeaderLine)
	if b.implicitHeader {
		where = "the session's language"
	}
	b.addError(fmt.Errorf("line %d: `%s;` conflicts with `%s;` (%s): a script is written in one language "+
		"version. A repeated header must name the same version; put %s statements in a script of their own",
		line, v, b.langVersion, where, v))
}

// implicitHeaderSource is the lexer of input read in a language it does not
// state (BuildSession): it yields the tokens of `mdl <n>;` before the input's
// own, so the parse tree carries the header every version-dependent rule
// reads, while each real token keeps its line and column.
type implicitHeaderSource struct {
	*parser.MDLLexer
	pending []antlr.Token
}

func newImplicitHeaderSource(lexer *parser.MDLLexer, v langver.Version) *implicitHeaderSource {
	src := &antlr.TokenSourceCharStreamPair{}
	mk := func(ttype int, text string) antlr.Token {
		// Line 0 marks the header as implicit (Builder.implicitHeader): no
		// line of the input holds it.
		return antlr.CommonTokenFactoryDEFAULT.Create(src, ttype, text, antlr.TokenDefaultChannel, -1, -1, 0, 0)
	}
	return &implicitHeaderSource{
		MDLLexer: lexer,
		pending: []antlr.Token{
			mk(parser.MDLLexerIDENTIFIER, "mdl"),
			mk(parser.MDLLexerNUMBER_LITERAL, fmt.Sprint(int(v))),
			mk(parser.MDLLexerSEMICOLON, ";"),
		},
	}
}

// NextToken yields the implicit header, then the input.
func (s *implicitHeaderSource) NextToken() antlr.Token {
	if len(s.pending) > 0 {
		t := s.pending[0]
		s.pending = s.pending[1:]
		return t
	}
	return s.MDLLexer.NextToken()
}
