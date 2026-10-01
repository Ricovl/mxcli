// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// check reports the flow refusal under the rule ID exec's warning carries, so
// a reader of either finds the same language change (ako/mxcli#876).
func TestFlowRebuildRuleIsTheLanguageChange(t *testing.T) {
	if FlowRebuildRule != flowRebuildRefused.Code {
		t.Fatalf("FlowRebuildRule = %q, flowRebuildRefused.Code = %q", FlowRebuildRule, flowRebuildRefused.Code)
	}
}

// The refusal check reports and the one exec returns are one function's text.
func TestFlowRefusalNamesTheFlowAndTheReason(t *testing.T) {
	d := &flowDecl{name: ast.QualifiedName{Module: "M", Name: "F"}}
	why := cannotSplice("the Loop at (1, 2) changes inside its body")
	msg := flowRefusal(d, why).Error()
	for _, want := range []string{"create or modify microflow M.F", "changes inside its body", "Nothing was written"} {
		if !strings.Contains(msg, want) {
			t.Errorf("want %q in %q", want, msg)
		}
	}
	w := flowRebuildWarning(&ExecContext{LanguageVersion: langver.V0}, d, why)
	if !strings.Contains(w, "microflow M.F is rebuilt as a whole") {
		t.Errorf("rebuild warning: %q", w)
	}
}
