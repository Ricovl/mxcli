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

// ako/mxcli#818: a statement drawn elsewhere is the stored node moved — it
// matches once positions are ignored — but a redrawn connector is not.
func TestSameExceptPositions_PositionsOnly(t *testing.T) {
	stored := parseFlowBody(t, storedFlowBody)
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"a node and a nested node moved", `  @position(210, 260)
  @curve(from: (30, 0), to: (-15, 0))
  @caption 'Say hello'
  log info node 'N' 'hello';
  @position(400, 200)
  if $In = 'x' then
    @position(420, 330)
    @anchor(from: bottom, to: top)
    log info node 'N' 'x';
  end if;`, true},
		{"a start stated", `  @start(10, 10)
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, true},
		{"a curve redrawn", `  @curve(from: (30, 0), to: (-15, 20))
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"an anchor redrawn", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    @anchor(from: right, to: top)
    log info node 'N' 'x';
  end if;`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := parseFlowBody(t, c.body)
			if got := sameExceptPositions(d.Body, stored.Body); got != c.want {
				t.Errorf("sameExceptPositions = %v, want %v", got, c.want)
			}
		})
	}
}

// Of two alike stored statements, the one the script leaves in place keeps
// its node; the moved one pairs with the other.
func TestLCSStatements_ExactMatchWinsOverAMove(t *testing.T) {
	stored := parseFlowBody(t, `  @position(200, 200)
  log info node 'N' 'same';
  @position(400, 200)
  log info node 'N' 'same';`).Body
	declared := parseFlowBody(t, `  @position(400, 200)
  log info node 'N' 'same';`).Body
	pairs := lcsStatements(declared, stored)
	if len(pairs) != 1 || pairs[0] != [2]int{0, 1} {
		t.Fatalf("pairs %v, want the declared statement paired with the stored one it matches exactly", pairs)
	}
}
