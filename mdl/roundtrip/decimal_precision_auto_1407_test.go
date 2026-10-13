// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1407 parts 1 and 2: a DecimalPrecision of -1 (Studio Pro's
// "no fixed precision") on a dynamic-text parameter and on a text box is
// described as -1, and that description has to parse, pass check, and execute
// back to the same page.
func TestDecimalPrecisionAutoRoundTrips_1407(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.addToFixture(`create module RT1407;
create persistent entity RT1407.T (Amount: Decimal);
create page RT1407.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $T: RT1407.T )) {
  dataview dv (DataSource: $T) {
    dynamictext dt (Content: '{1}', ContentParams: ({1} = Amount format (decimalPrecision: -1)))
    textbox tb (Label: 'Amount', Attribute: Amount, DecimalPrecision: -1)
  }
};`)
	snap := h.snapshot()

	first, err := h.describe("page RT1407.P")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, want := range []string{"format (decimalPrecision: -1)", "DecimalPrecision: -1"} {
		if !strings.Contains(first, want) {
			t.Fatalf("describe does not print %s:\n%s", want, first)
		}
	}
	prog, errs := visitor.Build(first)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs[0], first)
	}
	for _, v := range executor.ValidateProgram(prog, h.mpr) {
		if v.Severity == linter.SeverityError {
			t.Errorf("check refuses the describe output: [%s] %s", v.RuleID, v.Message)
		}
	}
	if errs := h.exe.ValidateProgram(prog); len(errs) > 0 {
		t.Errorf("check --references refuses the describe output: %v", errs)
	}
	if err := h.exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("exec the describe output: %v", err)
	}
	if changed := snap.diff(h.snapshot()); len(changed) > 0 {
		t.Errorf("executing the describe output wrote:\n  %s", strings.Join(changed, "\n  "))
	}
	if again, _ := h.describe("page RT1407.P"); again != first {
		t.Errorf("describe -> exec -> describe changed the page:\n%s", lineDiff(first, again))
	}
}
