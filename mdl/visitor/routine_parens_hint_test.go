// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1514 (ako/mxcli#1396): a microflow header without its
// parameter parentheses — `create microflow M.F returns Boolean as $b` — gave
// "missing '(' at 'returns'", "missing ':' at 'Boolean'" and, three lines into
// the body, "extraneous input 'return' expecting the start of a statement";
// with a bare `$l: List of …` parameter "missing '(' at '$l'" and "missing ')'
// at 'returns'". Every one points past the mistake: ANTLR inserts the `(`,
// reads `returns` as a parameter name, swallows the body into its type and
// closes the statement early. The report asked for one message.
package visitor

import (
	"strings"
	"testing"
)

const routineParensHint = "parameter list in parentheses"

func TestRoutineHeaderWithoutParensIsOneError(t *testing.T) {
	for _, c := range []struct {
		name, src, fix string
	}{
		{"no params",
			"create microflow Tmp.F returns Boolean as $b\nbegin\n  declare $b Boolean = true;\n  return $b;\nend;",
			"create microflow Tmp.F () returns"},
		{"bare param",
			"create microflow Tmp.F $l: List of Tmp.Item returns Boolean as $b\nbegin\n  return true;\nend;",
			"create microflow Tmp.F ($l: List of Tmp.Item) returns"},
		{"two bare params",
			"create microflow Tmp.F $a: String, $b: Integer\nbegin\nend;",
			"create microflow Tmp.F ($a: String, $b: Integer)"},
		{"nanoflow", "create nanoflow Tmp.NF begin\nend;", "create nanoflow Tmp.NF ()"},
		{"or modify", "create or modify microflow Tmp.F returns Boolean\nbegin\n  return true;\nend;",
			"create or modify microflow Tmp.F () returns Boolean"},
		{"rule as var", "create rule Tmp.R returns Boolean as $r\nbegin\n  return true;\nend;",
			"create rule Tmp.R () returns"},
		{"body with a statement keyword",
			"create microflow Tmp.F returns Boolean\nbegin\n  set $x = true;\n  return true;\nend;",
			"create microflow Tmp.F () returns"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A statement after the bad one must still parse: the cascade used to
			// swallow the body, and could swallow what followed it.
			_, errs := Build(c.src + "\ncreate entity Tmp.E (A: String(10));")
			got := errsText(errs)
			if len(errs) != 1 {
				t.Fatalf("want exactly one error, got %d:\n%s", len(errs), got)
			}
			if !strings.HasPrefix(got, "line 1:") {
				t.Errorf("the error is not on the header line:\n%s", got)
			}
			if !strings.Contains(got, routineParensHint) {
				t.Errorf("the error does not say the parentheses are mandatory:\n%s", got)
			}
			if !strings.Contains(got, c.fix) {
				t.Errorf("the error does not show the header with parentheses (%q):\n%s", c.fix, got)
			}
		})
	}
}

// The control: the form the error recommends parses, and a correct header with a
// different error on its line is not told to add parentheses.
func TestRoutineHeaderWithParensNotHinted(t *testing.T) {
	ok := "create microflow Tmp.F () returns Boolean as $b\nbegin\n  declare $b Boolean = true;\n  return $b;\nend;"
	if _, errs := Build(ok); len(errs) != 0 {
		t.Fatalf("the recommended form does not parse: %v", errs)
	}
	bad := "create microflow Tmp.F ($l List of Tmp.Item) returns Boolean\nbegin\n  return true;\nend;"
	_, errs := Build(bad)
	if len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
	if got := errsText(errs); strings.Contains(got, routineParensHint) {
		t.Errorf("a header with parentheses got the parentheses error:\n%s", got)
	}
}
