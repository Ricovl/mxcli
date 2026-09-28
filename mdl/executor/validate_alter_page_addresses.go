// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateAlterPageAddresses refuses, with no project needed, a target whose
// address FORM a page cannot use: a quoted caption, an @n, a path of more than
// two names. The generic grammar accepts every address form any document type
// uses, and the page family's resolver refuses these whatever the page holds —
// so `check` can say so up front instead of `exec` stopping mid-script. The
// rule is the resolver's own (backend.CheckPageAlterTarget), not a copy.
func validateAlterPageAddresses(stmt ast.Statement) []linter.Violation {
	s, ok := stmt.(*ast.AlterPageStmt)
	if !ok {
		return nil
	}
	kind := strings.ToLower(s.ContainerType)
	if kind == "" {
		kind = "page"
	}
	var out []linter.Violation
	for _, op := range s.Operations {
		for _, ref := range alterPageOperationTargets(op) {
			if err := backend.CheckPageAlterTarget(alterTargetOf(ref)); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-ALTER01",
					Severity: linter.SeverityError,
					Message:  fmt.Sprintf("alter %s %s: %v", kind, s.PageName.String(), err),
					Location: linter.Location{
						Module:       s.PageName.Module,
						DocumentType: kind,
						DocumentName: s.PageName.Name,
					},
				})
			}
		}
	}
	return out
}
