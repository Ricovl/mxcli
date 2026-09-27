// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateLanguageVersion reports what the script's `mdl <n>;` header means
// for it (ADR-0011):
//
//   - MDL-LANG01: the header names a preview version, whose meaning may still
//     change between mxcli releases until it is frozen at beta.
//   - one warning per construct the visitor kept at its older meaning, under
//     that change's own rule ID, so a headerless script lists everything whose
//     meaning differs under the newer language.
//
// Both are warnings: an mdl 0 script must keep running unchanged, and a preview
// is usable, only not yet a contract. An unknown version is refused earlier, by
// the parser.
func ValidateLanguageVersion(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	if v := prog.LanguageVersion; v.IsPreview() {
		out = append(out, linter.Violation{
			RuleID:   "MDL-LANG01",
			Severity: linter.SeverityWarning,
			Message:  fmt.Sprintf("line %d: %s", prog.LanguageHeaderLine, langver.PreviewWarning(v)),
			Suggestion: "Keep using it to try the beta language; omit the header for the " +
				"alpha meaning (mdl 0) if the script must not change under a later release.",
		})
	}
	for _, n := range prog.LanguageNotes {
		out = append(out, linter.Violation{
			RuleID:   n.Code,
			Severity: linter.SeverityWarning,
			Message:  fmt.Sprintf("line %d: %s", n.Line, n.Message),
		})
	}
	return out
}
