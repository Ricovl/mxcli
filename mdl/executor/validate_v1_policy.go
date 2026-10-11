// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ApplyV1Policy returns violations with every MDL-V1-* warning promoted to an
// error under deprecation.Error, and unchanged otherwise. It is
// ApplyDeprecationPolicy for the language-version codes: --deprecations gates
// deprecated spellings, --v1 gates what a headerless (mdl 0) script still has
// to change before it can carry `mdl 1;`. Both at error is the full mdl 1
// readiness gate.
func ApplyV1Policy(violations []linter.Violation, policy deprecation.Policy) []linter.Violation {
	if policy != deprecation.Error {
		return violations
	}
	out := make([]linter.Violation, len(violations))
	for i, v := range violations {
		if langver.IsV1Code(v.RuleID) {
			v.Severity = linter.SeverityError
		}
		out[i] = v
	}
	return out
}
