// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1533 (2): `returns` written after `exposed as workflow
// action …` gave "mismatched input 'returns' expecting BEGIN", which reads as
// if an exposed microflow cannot return a value. It can; the grammar fixes the
// order — the return type comes before every option clause.
package visitor

import (
	"strings"
	"testing"
)

const returnsAfterOptionsHint = "`returns` must come right after the parameter list"

func TestReturnsAfterExposedIsHinted(t *testing.T) {
	for name, src := range map[string]string{
		"exposed": "create microflow M.ACT_A ($Ctx: M.Item)\nexposed as workflow action 'Approve' in 'Cat'\nreturns Boolean as $Ok\nbegin\n  return true;\nend;",
		"folder":  "create nanoflow M.NF () folder 'Sub' returns Boolean\nbegin\n  return true;\nend;",
		// The exposed clause cut short: the error lands inside the clause.
		"inside": "create microflow M.ACT_A ($Ctx: M.Item)\nexposed as workflow action\nreturns Boolean as $Ok\nbegin\n  return true;\nend;",
	} {
		_, errs := Build(src)
		got := errsText(errs)
		if !strings.Contains(got, "input 'returns'") {
			t.Fatalf("%s: expected the parse error on 'returns', got:\n%s", name, got)
		}
		if n := strings.Count(got, returnsAfterOptionsHint); n != 1 {
			t.Errorf("%s: hint appears %d times, want 1:\n%s", name, n, got)
		}
	}
}

// The control: the order the hint recommends parses.
func TestReturnsBeforeExposedParses(t *testing.T) {
	src := "create microflow M.ACT_A ($Ctx: M.Item)\nreturns Boolean as $Ok\nexposed as workflow action 'Approve' in 'Cat'\nbegin\n  return true;\nend;"
	if _, errs := Build(src); len(errs) != 0 {
		t.Fatalf("the hinted order does not parse: %v", errs)
	}
}

// A stray `returns` inside a microflow BODY is not a header-order mistake and
// must not get this hint (the body has its own `returns` clauses — a REST call's).
func TestReturnsInBodyNotHintedAsHeaderOrder(t *testing.T) {
	src := "create microflow M.F ()\nbegin\n  returns Boolean;\n  return true;\nend;"
	_, errs := Build(src)
	got := errsText(errs)
	if !strings.Contains(got, "input 'returns'") {
		t.Fatalf("expected the parse error on 'returns', got:\n%s", got)
	}
	if strings.Contains(got, returnsAfterOptionsHint) {
		t.Errorf("body error got the header-order hint:\n%s", got)
	}
}
