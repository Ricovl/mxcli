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
// the header exists to prevent. Whether the version is a preview is reported by
// the validator, next to every other warning check and exec print.
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
