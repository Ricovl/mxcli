// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// detachedDocCommentRule is the warning for a doc comment the statement after
// it does not store (ako/mxcli#877).
const detachedDocCommentRule = "MDL089"

// ValidateDetachedDocComments warns on every `/** … */` doc comment written
// before a statement that cannot store documentation — a drop, grant, revoke,
// set, alter, or a create of something without documentation. The statement
// ignores it, so the text is lost with nothing else reporting it: mxcli-rest
// wrote `drop microflow if exists X;` between six flows' comments and their
// creates, and only lint's QUAL002 noticed the missing documentation. The
// warning names the next statement that would have stored it, which is almost
// always the one it was meant for.
//
// A warning, not an error, under every language version: the statement does
// what it says, and only the comment is lost.
func ValidateDetachedDocComments(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, d := range prog.DetachedDocComments {
		msg := fmt.Sprintf("line %d: the doc comment before `%s` is lost: that statement cannot store "+
			"documentation, so the comment is ignored", d.Line, d.Statement)
		suggestion := "Move the doc comment above the statement it documents, or make it a `--` comment."
		if d.Next != "" {
			msg += fmt.Sprintf("; the next statement that can take it is `%s` (line %d)", d.Next, d.NextLine)
			suggestion = fmt.Sprintf("Move the doc comment directly above `%s`, after the `%s`.", d.Next, d.Statement)
		}
		out = append(out, linter.Violation{
			RuleID:     detachedDocCommentRule,
			Severity:   linter.SeverityWarning,
			Message:    msg,
			Suggestion: suggestion,
		})
	}
	return out
}
