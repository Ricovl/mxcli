// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1514: a microflow header without its parameter parentheses —
// `create microflow M.F returns Boolean as $b` — gave "missing '(' at
// 'returns'", "missing ':' at 'Boolean'", and with a bare `$l: List of …`
// parameter "missing ')' at 'returns'". None says the parentheses are mandatory.
package visitor

import (
	"strings"
	"testing"
)

const routineParensHint = "parameter list in parentheses"

func TestRoutineHeaderWithoutParensIsHinted(t *testing.T) {
	for name, src := range map[string]string{
		"no params":   "create microflow Tmp.F returns Boolean as $b\nbegin\n  declare $b Boolean = true;\n  return $b;\nend;",
		"bare param":  "create microflow Tmp.F $l: List of Tmp.Item returns Boolean as $b\nbegin\n  return true;\nend;",
		"nanoflow":    "create nanoflow Tmp.NF begin\nend;",
		"or modify":   "create or modify microflow Tmp.F returns Boolean\nbegin\n  return true;\nend;",
		"rule as var": "create rule Tmp.R returns Boolean as $r\nbegin\n  return true;\nend;",
	} {
		_, errs := Build(src)
		got := errsText(errs)
		if len(errs) == 0 {
			t.Fatalf("%s: expected a parse error", name)
		}
		// One mistake, one explanation — the cascade on the header line must not
		// repeat it.
		if n := strings.Count(got, routineParensHint); n != 1 {
			t.Errorf("%s: hint appears %d times, want 1:\n%s", name, n, got)
		}
	}
}

// The control: the form the hint recommends parses, and a correct header with a
// different error on its line is not told to add parentheses.
func TestRoutineHeaderWithParensNotHinted(t *testing.T) {
	ok := "create microflow Tmp.F () returns Boolean as $b\nbegin\n  declare $b Boolean = true;\n  return $b;\nend;"
	if _, errs := Build(ok); len(errs) != 0 {
		t.Fatalf("the hinted form does not parse: %v", errs)
	}
	bad := "create microflow Tmp.F ($l List of Tmp.Item) returns Boolean\nbegin\n  return true;\nend;"
	_, errs := Build(bad)
	if len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
	if got := errsText(errs); strings.Contains(got, routineParensHint) {
		t.Errorf("a header with parentheses got the parentheses hint:\n%s", got)
	}
}
