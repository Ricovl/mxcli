// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateDeprecations reports every use of a deprecated spelling the visitor
// recorded, one warning per use, under the entry's MDL-DEPRnnn code.
//
// A warning, never an error by itself: a deprecated spelling means exactly
// what its canonical form means, and scripts in the wild use it. `check` and
// `exec` promote these to errors only under --deprecations=error (see
// ApplyDeprecationPolicy), which is for CI over docs, skills and examples.
func ValidateDeprecations(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, d := range prog.Deprecations {
		e, ok := deprecation.Lookup(d.Code)
		if !ok {
			continue // the visitor records only registered codes; pinned by tests
		}
		on := ""
		if d.Subject != "" {
			on = " (" + d.Subject + ")"
		}
		out = append(out, linter.Violation{
			RuleID:   e.Code,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("line %d: `%s`%s is deprecated; write `%s` — same meaning. "+
				"Refused from `mdl %d`.", d.Line, e.Old, on, e.Canonical, e.RemovedIn),
			Suggestion: deprecationSuggestion(e),
		})
	}
	return out
}

// ApplyDeprecationPolicy returns violations with every MDL-DEPRnnn warning
// promoted to an error under deprecation.Error, and unchanged otherwise.
func ApplyDeprecationPolicy(violations []linter.Violation, policy deprecation.Policy) []linter.Violation {
	if policy != deprecation.Error {
		return violations
	}
	out := make([]linter.Violation, len(violations))
	for i, v := range violations {
		if deprecation.IsDeprecationCode(v.RuleID) {
			v.Severity = linter.SeverityError
		}
		out[i] = v
	}
	return out
}

// deprecationSuggestion says how to rewrite a deprecated spelling: the keyword
// to swap, or for a structural rewrite, the rewrite itself.
func deprecationSuggestion(e deprecation.Entry) string {
	if e.Rewrite.Structural != "" {
		return fmt.Sprintf("Rewrite the %s. %s", e.Rewrite.Structural, e.Note)
	}
	return fmt.Sprintf("Replace `%s` with `%s`. %s", e.Rewrite.Token, e.Rewrite.Replacement, e.Note)
}
