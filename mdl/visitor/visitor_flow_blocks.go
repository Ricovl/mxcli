// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// R2 (ADR-0010, ako/mxcli#754): brackets have one meaning each. `( … )` holds
// an element's properties, `{ … }` its declarative children, and imperative
// flow is `begin … end <keyword>`. Two microflow blocks broke the rule:
//
//   - a custom error handler was the only brace block inside a microflow. Its
//     canonical form is `on error [without rollback] begin … end error`, and the
//     brace form is a respelling (MDL-DEPR540) that builds the same handler;
//   - `while` made `begin` and the `while` after `end` optional, where `loop`
//     requires both. That is a change of what parses, so it is gated: under
//     mdl 1 both are required, and a headerless script keeps the old grammar
//     and warns (MDL-V1-WHILE).

// whileBlockRequired is the new rejection of a `while` without `begin` or
// without `end while`.
var whileBlockRequired = langver.Change{
	Code:  "MDL-V1-WHILE",
	Since: langver.V1,
	Old:   "`while <condition>` accepts a body without `begin` and an `end` without `while`",
	New:   "an error: a while loop is `while <condition> begin … end while;`, like `loop … begin … end loop;`",
}

// ExitOnErrorClause records MDL-DEPR540 for a custom handler written in braces,
// with the rewrite to `begin … end error`.
func (b *Builder) ExitOnErrorClause(ctx *parser.OnErrorClauseContext) {
	if ctx == nil || ctx.LBRACE() == nil || ctx.RBRACE() == nil {
		return
	}
	lbrace, rbrace := ctx.LBRACE().GetSymbol(), ctx.RBRACE().GetSymbol()
	b.recordDeprecation(deprecation.OnErrorBraces, lbrace, "")
	like := ctx.ERROR(0).GetText()
	b.fixLastDeprecation(deprecation.OnErrorBraces, &ast.Fix{Edits: []ast.TextEdit{
		{Start: lbrace.GetStart(), Stop: lbrace.GetStop() + 1, Text: keywordLike(like, "begin")},
		{Start: rbrace.GetStart(), Stop: rbrace.GetStop() + 1, Text: keywordLike(like, "end error")},
	}}, "")
}

// ExitWhileStatement applies whileBlockRequired.
func (b *Builder) ExitWhileStatement(ctx *parser.WhileStatementContext) {
	if ctx == nil || ctx.END() == nil || ctx.Expression() == nil {
		return // a syntax error has already been reported
	}
	missingBegin, missingWhile := ctx.BEGIN() == nil, len(ctx.AllWHILE()) < 2
	if !missingBegin && !missingWhile {
		return
	}
	if b.gate(whileBlockRequired, ctx) {
		var missing string
		switch {
		case missingBegin && missingWhile:
			missing = "`begin` after the condition and `while` after `end`"
		case missingBegin:
			missing = "`begin` after the condition"
		default:
			missing = "`while` after `end`"
		}
		b.addError(fmt.Errorf("line %d: this while loop has no %s: under %s a while loop is "+
			"`while <condition> begin … end while;`", ctx.GetStart().GetLine(), missing, b.langVersion))
		return
	}
	like := ctx.WHILE(0).GetText()
	var edits []ast.TextEdit
	if missingBegin {
		_, stop := nodeSpan(ctx.Expression())
		edits = append(edits, insertAt(stop+1, " "+keywordLike(like, "begin")))
	}
	if missingWhile {
		edits = append(edits, insertAt(ctx.END().GetSymbol().GetStop()+1, " "+keywordLike(like, "while")))
	}
	b.fixLastNote(whileBlockRequired.Code, &ast.Fix{Edits: edits}, "")
}
