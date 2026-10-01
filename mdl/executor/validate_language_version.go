// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateLanguageVersion reports what the script's `mdl <n>;` header means
// for it (ADR-0011): one warning per construct the visitor kept at its older
// meaning, under that change's own rule ID, so a headerless script lists
// everything whose meaning differs under mdl 1.
//
// They are warnings: an mdl 0 script must keep running unchanged. An unknown
// version, or a repeated header that conflicts with the first, is refused
// earlier, by the parser. mdl 1 is frozen (ako/mxcli#714), so a header naming
// it is a contract and reports nothing — the MDL-LANG01 "preview" warning it
// carried before beta is gone.
func ValidateLanguageVersion(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, n := range prog.LanguageNotes {
		out = append(out, linter.Violation{
			RuleID:   n.Code,
			Severity: linter.SeverityWarning,
			Message:  fmt.Sprintf("line %d: %s", n.Line, n.Message),
		})
	}
	return out
}
