// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func parseFlowBodyUnder(t *testing.T, header, body string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build(header + "create or modify microflow M.F ($In: String)\nbegin\n" + body + "\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	for _, st := range prog.Statements {
		if s, ok := st.(*ast.CreateMicroflowStmt); ok {
			return s
		}
	}
	t.Fatal("no create microflow statement")
	return nil
}

// ako/mxcli#839: a retrieve written with the bracketed XPath form, as authors
// write it, never matched the stored retrieve describe prints without the
// brackets. The two are read by different grammar paths — the bracketed one as
// XPath, keeping its source text and lower-case operators; the bare one as an
// expression — so the same constraint compared as two different ASTs, and
// diff-then-patch saw a dropped retrieve and a new one. Under mdl 1 that is a
// refusal when the retrieve carries a note, and otherwise a replace that
// writes the node anew; under mdl 0 the whole flow was rebuilt.
//
// A where clause is compared as the constraint it stores.
func TestDeclaredMatches_RetrieveWhereComparesTheStoredConstraint(t *testing.T) {
	// What describe prints for the stored retrieve.
	const stored = `  @position(520, 595)
  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where MonthKey >= $FromKey and MonthKey <= $ToKey and ($CatName = '' or CategoryName = $CatName) and not(CategoryName = 'Savings transfer')
    sort by CategoryName asc;`
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"as described", stored, true},
		{"bracketed, one line", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where [MonthKey >= $FromKey and MonthKey <= $ToKey and ($CatName = '' or CategoryName = $CatName) and not(CategoryName = 'Savings transfer')]
    sort by CategoryName asc;`, true},
		{"bracketed, over several lines", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where [MonthKey >= $FromKey and MonthKey <= $ToKey
      and ($CatName = '' or CategoryName = $CatName)
      and not(CategoryName = 'Savings transfer')]
    sort by CategoryName asc;`, true},
		{"upper-case operators", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where MonthKey >= $FromKey AND MonthKey <= $ToKey AND ($CatName = '' OR CategoryName = $CatName) AND not(CategoryName = 'Savings transfer')
    sort by CategoryName asc;`, true},
		// Controls: a constraint that stores something else is a difference.
		{"a changed comparison", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where [MonthKey > $FromKey and MonthKey <= $ToKey
      and ($CatName = '' or CategoryName = $CatName)
      and not(CategoryName = 'Savings transfer')]
    sort by CategoryName asc;`, false},
		{"whitespace inside a string", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    where [MonthKey >= $FromKey and MonthKey <= $ToKey
      and ($CatName = '' or CategoryName = $CatName)
      and not(CategoryName = 'Savings  transfer')]
    sort by CategoryName asc;`, false},
		{"no constraint", `  @annotation 'Every filter is in the retrieve.'
  retrieve $Months from M.VMonth
    sort by CategoryName asc;`, false},
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		storedFlow := parseFlowBodyUnder(t, header, stored)
		for _, tc := range cases {
			t.Run(map[string]string{"": "mdl0", "mdl 1;\n": "mdl1"}[header]+"/"+tc.name, func(t *testing.T) {
				declared := parseFlowBodyUnder(t, header, tc.body)
				if got := declaredMatches(declared.Body, storedFlow.Body); got != tc.want {
					t.Errorf("declaredMatches = %v, want %v", got, tc.want)
				}
			})
		}
	}
}

// ako/mxcli#839, the second half: a member value whose closing parenthesis is
// on a line of its own keeps the line break in its source text (the writer
// stores what was typed), and describe drops the whitespace around a stored
// expression (describeExpr), so the stored side never has it. It states
// nothing, and must not make the change a replaced activity.
func TestDeclaredMatches_WhitespaceAroundAnExpressionIsNotADifference(t *testing.T) {
	const stored = `  change $In (Name = 'a' + 'b'
      + 'c') refresh;`
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"as described", stored, true},
		{"closing parenthesis on its own line", `  change $In (
    Name = 'a' + 'b'
      + 'c'
  ) refresh;`, true},
		// An interior line break is the author's formatting too: stored as
		// written, but the same expression wherever it falls, so moving one
		// is not a change to write (ako/mxcli#886 — it was, on every run,
		// wherever describe's layout of a stored value differed from the
		// script's).
		{"an interior line break moved", `  change $In (
    Name = 'a'
      + 'b' + 'c'
  ) refresh;`, true},
		{"a changed value", `  change $In (
    Name = 'a' + 'b'
      + 'd'
  ) refresh;`, false},
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		storedFlow := parseFlowBodyUnder(t, header, stored)
		for _, tc := range cases {
			t.Run(map[string]string{"": "mdl0", "mdl 1;\n": "mdl1"}[header]+"/"+tc.name, func(t *testing.T) {
				declared := parseFlowBodyUnder(t, header, tc.body)
				if got := declaredMatches(declared.Body, storedFlow.Body); got != tc.want {
					t.Errorf("declaredMatches = %v, want %v", got, tc.want)
				}
			})
		}
	}
}
