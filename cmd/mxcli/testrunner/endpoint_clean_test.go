// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestGenerateEndpointMDLDrawsNoDiagnostics: the runner execs this script
// itself, so anything exec's preflight says about it is noise the user cannot
// act on. It printed two MDL-DEPR001 (`create or replace`) and two
// MDL-V1-SLASH per `mxcli test` run (ako/mxcli#943).
func TestGenerateEndpointMDLDrawsNoDiagnostics(t *testing.T) {
	for _, chain := range []string{"", "MyModule.ASU_Startup"} {
		src := GenerateEndpointMDL(chain)
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("chain %q: parse errors: %v\n%s", chain, errs, src)
		}
		if prog.LanguageVersion != langver.V1 {
			t.Errorf("chain %q: script is read as language version %v, want mdl 1", chain, prog.LanguageVersion)
		}
		for _, v := range executor.ValidateProgram(prog, "") {
			t.Errorf("chain %q: %s %s: %s", chain, v.Severity, v.RuleID, v.Message)
		}
	}
}
