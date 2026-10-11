// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func TestV1PolicyPromotesOnlyV1Codes(t *testing.T) {
	in := []linter.Violation{
		{RuleID: "MDL-V1-SLASH", Severity: linter.SeverityWarning},
		{RuleID: FlowRebuildRule, Severity: linter.SeverityWarning},
		{RuleID: "MDL-DEPR001", Severity: linter.SeverityWarning},
		{RuleID: "MDL065", Severity: linter.SeverityWarning},
	}
	if got := ApplyV1Policy(in, deprecation.Warn); got[0].Severity != linter.SeverityWarning {
		t.Errorf("--v1=warn promoted %s", got[0].RuleID)
	}
	got := ApplyV1Policy(in, deprecation.Error)
	for i, want := range []linter.Severity{linter.SeverityError, linter.SeverityError, linter.SeverityWarning, linter.SeverityWarning} {
		if got[i].Severity != want {
			t.Errorf("%s under --v1=error: severity %v, want %v", got[i].RuleID, got[i].Severity, want)
		}
	}
	if in[0].Severity != linter.SeverityWarning {
		t.Error("ApplyV1Policy modified its input")
	}
}

// The real path: a headerless script's `/` line reaches ApplyV1Policy as
// MDL-V1-SLASH from ValidateProgram.
func TestV1PolicyThroughValidateProgram(t *testing.T) {
	prog := mustProgram(t, "create module V1Gate;\n/\n")
	got := ApplyV1Policy(ValidateProgram(prog, ""), deprecation.Error)
	found := false
	for _, v := range got {
		if v.RuleID == "MDL-V1-SLASH" {
			found = true
			if v.Severity != linter.SeverityError {
				t.Errorf("MDL-V1-SLASH severity %v under --v1=error", v.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("no MDL-V1-SLASH in %+v", got)
	}
}
