// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestRegisterEndpointAppliesEntityAccess pins the generated startup flow's
// "apply entity access" setting (ako/mxcli#1654, mendixlabs/mxcli#1645).
//
// MxTest.RegisterEndpoint calls the project's own after-startup microflow.
// Written without the annotation it did not apply entity access, and mxbuild
// refuses a non-applying microflow that calls an applying one (CE0114): any
// project whose after-startup microflow applies entity access could not run a
// single test. Measured with mx check on 11.13.0, the rule has one direction —
// non-applying → applying is CE0114, applying → non-applying and applying →
// applying are 0 errors, under security Off and Production alike — so a
// RegisterEndpoint that applies entity access may call either kind. The setting
// governs only its own two activities: the Java action ignores it, and each
// chained microflow runs with its own.
func TestRegisterEndpointAppliesEntityAccess(t *testing.T) {
	for _, chain := range []string{"", "MyModule.ASU_Startup"} {
		prog, errs := visitor.Build(GenerateEndpointMDL(chain))
		if len(errs) > 0 {
			t.Fatalf("chain %q: generated MDL does not parse: %v", chain, errs[0])
		}
		var found *ast.CreateMicroflowStmt
		for _, s := range prog.Statements {
			if mf, ok := s.(*ast.CreateMicroflowStmt); ok && mf.Name.String() == endpointStartupFlow {
				found = mf
			}
		}
		if found == nil {
			t.Fatalf("chain %q: no %s in the generated MDL", chain, endpointStartupFlow)
		}
		if found.ApplyEntityAccess == nil || !*found.ApplyEntityAccess {
			t.Errorf("chain %q: %s does not apply entity access, so it cannot call an after-startup microflow that does (CE0114)",
				chain, endpointStartupFlow)
		}
	}
}

// TestBuildErrorInGeneratedStartupFlowIsMxclis pins the attribution half of
// ako/mxcli#1654. The CE0114 above was located in MxTest.RegisterEndpoint — a
// document mxcli generated for the run — and the error said "These errors are in
// the project, not in this run's tests", sending the reader through their own
// model for a defect in mxcli's.
func TestBuildErrorInGeneratedStartupFlowIsMxclis(t *testing.T) {
	suite := suiteOf("test_1")
	ce0114 := problem("CE0114",
		"A microflow that does not apply entity access can only call microflows that also do not apply entity access.",
		"MxTest", "Microflow 'RegisterEndpoint'", "Call microflow activity 'ASU_Startup'")
	inProject := problem("CE0109", "Undefined variable 'x'.", "Sudoku", "Microflow 'SUB_Deal'", "End event")

	t.Run("only the generated flow", func(t *testing.T) {
		_, err := resultsForBuildFailure(buildFailure(failedBuild(ce0114)), suite)
		if err == nil {
			t.Fatal("no error for a failed build")
		}
		msg := err.Error()
		if strings.Contains(msg, "in the project") {
			t.Errorf("an error in a document mxcli generated is blamed on the project:\n%s", msg)
		}
		for _, want := range []string{endpointStartupFlow, "generated", "mxcli", "--skip-app-startup"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message does not say %q:\n%s", want, msg)
			}
		}
	})

	t.Run("the generated flow and the project", func(t *testing.T) {
		_, err := resultsForBuildFailure(buildFailure(failedBuild(ce0114, inProject)), suite)
		msg := err.Error()
		if !strings.Contains(msg, endpointStartupFlow) || !strings.Contains(msg, "in the project") {
			t.Errorf("want both attributions:\n%s", msg)
		}
	})

	t.Run("control: only the project", func(t *testing.T) {
		_, err := resultsForBuildFailure(buildFailure(failedBuild(inProject)), suite)
		msg := err.Error()
		if !strings.Contains(msg, "These errors are in the project, not in this run's tests.") {
			t.Errorf("a project error is no longer attributed to the project:\n%s", msg)
		}
		if strings.Contains(msg, "generated") {
			t.Errorf("a project error is attributed to mxcli:\n%s", msg)
		}
	})
}
