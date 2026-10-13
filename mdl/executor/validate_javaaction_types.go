// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateJavaActionTypeNames is MDL-JA01: exec's refusal of an unqualified
// type name that no type parameter declares, reported at check time.
func validateJavaActionTypeNames(s *ast.CreateJavaActionStmt) []linter.Violation {
	bad := undeclaredJavaActionTypeNames(s)
	if len(bad) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-JA01",
		Severity: linter.SeverityError,
		Location: linter.Location{
			Module:       s.Name.Module,
			DocumentType: "java action",
			DocumentName: s.Name.Name,
		},
		Message: fmt.Sprintf("java action '%s': %s, which is neither a qualified entity nor a declared type parameter "+
			"— it would be written as an entity with no module (CE1613)", s.Name.String(), strings.Join(bad, ", ")),
		Suggestion: undeclaredJavaActionTypeHint(s),
	}}
}
