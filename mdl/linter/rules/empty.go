// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// EmptyMicroflowRule checks for flows with no activities.
//
// LintContext.Microflows() yields microflows, nanoflows and rules alike — they
// share one catalog table — so this reports on all three and names each by what
// it actually is. Hardcoding "microflow" is what made it announce
// "Microflow 'Rule1' has no activities" about a rule.
type EmptyMicroflowRule struct{}

// NewEmptyMicroflowRule creates a new empty microflow rule.
func NewEmptyMicroflowRule() *EmptyMicroflowRule {
	return &EmptyMicroflowRule{}
}

func (r *EmptyMicroflowRule) ID() string                       { return "MPR002" }
func (r *EmptyMicroflowRule) Name() string                     { return "EmptyMicroflow" }
func (r *EmptyMicroflowRule) Category() string                 { return "quality" }
func (r *EmptyMicroflowRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *EmptyMicroflowRule) Description() string {
	return "Checks for microflows, nanoflows and rules that have no activities"
}

// returnsValue reports whether a flow's end event returns a value. A flow whose
// only content is `return <expr>;` has no activities — ActivityCount excludes
// the start and end events — but it computes something and is not empty
// (ako/mxcli#953). A non-Void return type is the witness: mxbuild requires
// every end event of such a flow to return a value.
func returnsValue(mf linter.Microflow) bool {
	return mf.ReturnType != "" && mf.ReturnType != "Void"
}

// Check runs the empty microflow check.
func (r *EmptyMicroflowRule) Check(ctx *linter.LintContext) []linter.Violation {
	var violations []linter.Violation

	for mf := range ctx.Microflows() {
		if mf.ActivityCount == 0 && !returnsValue(mf) {
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message:  fmt.Sprintf("%s '%s' has no activities", mf.DocumentNounTitle(), mf.Name),
				Location: linter.Location{
					Module:       mf.ModuleName,
					DocumentType: mf.DocumentNoun(),
					DocumentName: mf.Name,
					DocumentID:   mf.ID,
				},
				Suggestion: fmt.Sprintf("Add activities or remove the unused %s", mf.DocumentNoun()),
			})
		}
	}

	return violations
}
