// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func parseFlowBody(t *testing.T, body string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build("create or modify microflow M.F ($In: String)\nbegin\n" + body + "\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	s, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

// The stored side is what describe prints: geometry on every statement.
const storedFlowBody = `  @position(200, 200)
  @curve(from: (30, 0), to: (-15, 0))
  @caption 'Say hello'
  log info node 'N' 'hello';
  @position(400, 200)
  if $In = 'x' then
    @position(400, 300)
    @anchor(from: bottom, to: top)
    log info node 'N' 'x';
  end if;`

func TestDeclaredMatches_OmittedGeometryIsNotADifference(t *testing.T) {
	stored := parseFlowBody(t, storedFlowBody)
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"identical", storedFlowBody, true},
		{"no geometry at all", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, true},
		{"a node moved", `  @position(210, 200)
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"a caption dropped", `  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"a nested statement changed", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'y';
  end if;`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared := parseFlowBody(t, tc.body)
			if got := declaredMatches(declared.Body, stored.Body); got != tc.want {
				t.Errorf("declaredMatches = %v, want %v", got, tc.want)
			}
		})
	}

	// The asymmetry: geometry the STORED side lacks is not a wildcard. A
	// declared position against a stored flow drawn elsewhere is a move.
	bare := parseFlowBody(t, `  @caption 'Say hello'
  log info node 'N' 'hello';`)
	placed := parseFlowBody(t, `  @position(200, 200)
  @caption 'Say hello'
  log info node 'N' 'hello';`)
	if declaredMatches(placed.Body, bare.Body) {
		t.Error("a declared @position matched a statement stored without one")
	}
}

func TestLCSStatements_PairsTheUnchangedRuns(t *testing.T) {
	stored := parseFlowBody(t, `  log info node 'N' 'a';
  log info node 'N' 'b';
  log info node 'N' 'c';
  log info node 'N' 'd';`)
	declared := parseFlowBody(t, `  log info node 'N' 'a';
  log info node 'N' 'new';
  log info node 'N' 'b';
  log info node 'N' 'C';
  log info node 'N' 'd';`)
	got := lcsStatements(declared.Body, stored.Body)
	want := [][2]int{{0, 0}, {2, 1}, {4, 3}}
	if len(got) != len(want) {
		t.Fatalf("pairs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pairs = %v, want %v", got, want)
		}
	}
}
