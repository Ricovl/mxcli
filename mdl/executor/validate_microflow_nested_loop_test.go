// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL001 fired on every loop inside a loop, so an intentional region × plan
// iteration drew the "replace it with FIND" hint about five times per script.
// The hint is about one shape: an inner loop that looks for the outer item's
// match, which is the comparison between the two iterators. Iterating every
// pair without matching them is not a lookup.
func TestMDL001FiresOnlyOnTheListMatchingShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"lookup: inner if compares both iterators",
			"loop $R in $Rs begin loop $P in $Ps begin if $P/Name = $R/Name then set $n = $n + 1; end if; end loop; end loop;", true},
		{"lookup: comparison nested deeper in the inner body",
			"loop $R in $Rs begin loop $P in $Ps begin if $n > 0 then if $R/Name = $P/Name then set $n = 0; end if; end if; end loop; end loop;", true},
		{"cartesian product without matching (control)",
			"loop $R in $Rs begin loop $P in $Ps begin set $n = $n + 1; end loop; end loop;", false},
		{"inner if on the inner item only",
			"loop $R in $Rs begin loop $P in $Ps begin if $P/Name = 'x' then set $n = $n + 1; end if; end loop; end loop;", false},
		{"single loop",
			"loop $R in $Rs begin if $R/Name = 'x' then set $n = 1; end if; end loop;", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($Rs: list of M.R, $Ps: list of M.R)\nreturns Boolean\nbegin\n  declare $n Integer = 0;\n  " +
				tc.body + "\n  return true;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			got := false
			for _, vi := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
				if vi.RuleID == "MDL001" {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("MDL001 fired=%v, want %v", got, tc.want)
			}
		})
	}
}
