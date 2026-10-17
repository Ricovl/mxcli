// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// unreachableViolations parses src and returns the MDL-FLOW05 messages per
// microflow or nanoflow name.
func unreachableViolations(t *testing.T, src string) map[string][]string {
	t.Helper()
	got := map[string][]string{}
	for _, stmt := range parseMDL(t, src).Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			for _, v := range ValidateMicroflow(s) {
				if v.RuleID == "MDL-FLOW05" {
					got[s.Name.Name] = append(got[s.Name.Name], v.Message)
				}
			}
		case *ast.CreateNanoflowStmt:
			for _, v := range ValidateNanoflow(s) {
				if v.RuleID == "MDL-FLOW05" {
					got[s.Name.Name] = append(got[s.Name.Name], v.Message)
				}
			}
		}
	}
	return got
}

// The shapes of mendixlabs/mxcli#1491 plus the siblings measured with it, each
// one microflow. Every "dead" one fails mxbuild 11.13.0 (CE0104 / CE0105 /
// CE0709); every "live" one builds with 0 errors.
const unreachableShapes = `
create module Repro;
create microflow Repro.T1 ($N: Integer) begin
  declare $I Integer = 0;
  while true begin
    set $I = $I + 1;
    if $I >= $N then return; end if;
  end while;
  return;
end;
create microflow Repro.B ($N: Integer) begin
  declare $I Integer = 0;
  while true begin
    set $I = $I + 1;
    if $I >= $N then return; end if;
  end while;
  log info node 'Repro' 'after';
end;
create microflow Repro.A ($V: Integer) begin
  if $V < 0 then
    return;
    log info node 'Repro' 'negative';
  end if;
  return;
end;
create microflow Repro.D ($N: Integer) begin
  declare $I Integer = 0;
  while $I < $N begin
    set $I = $I + 1;
    break;
    log info node 'Repro' 'after break';
  end while;
end;
create microflow Repro.E ($N: Integer) begin
  declare $I Integer = 0;
  while $I < $N begin
    set $I = $I + 1;
    continue;
    log info node 'Repro' 'after continue';
  end while;
end;
create microflow Repro.F ($V: Integer) begin
  if $V < 0 then return; else return; end if;
  log info node 'Repro' 'after';
end;
create microflow Repro.H () returns Boolean begin
  $X = call microflow Repro.Sub(In = 1) on error without rollback begin
    join fail;
    log info node 'Repro' 'after join';
  end error;
  return true;
  merge fail;
  return false;
end;
create microflow Repro.I () returns Boolean begin
  $X = call microflow Repro.Sub(In = 1) on error begin
    raise error;
    log info node 'Repro' 'after raise';
  end error;
  return true;
end;
create microflow Repro.J () returns Boolean begin
  $X = call microflow Repro.Sub(In = 1) on error without rollback begin
    return false;
    log info node 'Repro' 'after return';
  end error;
  return true;
end;
create microflow Repro.C1 ($N: Integer) begin
  declare $I Integer = 0;
  while true begin
    set $I = $I + 1;
    if $I >= $N then return; end if;
  end while;
end;
create microflow Repro.C2 ($N: Integer) begin
  declare $I Integer = 0;
  while true begin
    set $I = $I + 1;
    if $I >= $N then break; end if;
  end while;
  return;
end;
create microflow Repro.G () returns Boolean begin
  $X = call microflow Repro.Sub(In = 1) on error without rollback begin
    join fail;
  end error;
  return true;
  merge fail;
  log error node 'Repro' 'failed';
  return false;
end;
create microflow Repro.K ($V: Integer) begin
  if $V < 0 then return; end if;
  log info node 'Repro' 'reachable';
end;
`

func TestUnreachableStatementsAreRefused(t *testing.T) {
	got := unreachableViolations(t, unreachableShapes)
	for _, name := range []string{"T1", "B", "A", "D", "E", "F", "H", "I", "J"} {
		if len(got[name]) != 1 {
			t.Errorf("%s: got %d MDL-FLOW05 findings, want 1 (mxbuild refuses it): %v", name, len(got[name]), got[name])
		}
	}
	// CONTROLS: each builds with 0 errors on mxbuild 11.13.0.
	for _, name := range []string{"C1", "C2", "G", "K"} {
		if len(got[name]) != 0 {
			t.Errorf("%s builds clean but was refused: %v", name, got[name])
		}
	}
	if msg := strings.Join(got["T1"], ""); !strings.Contains(msg, "while true") {
		t.Errorf("T1's message should name the loop that ends the path:\n%s", msg)
	}
}

// A break that leaves the loop from inside a split or an error handler still
// makes the code after a `while true` reachable. Missing it would refuse a
// script that builds.
func TestUnreachableSeesBreakInsideSplitAndHandler(t *testing.T) {
	got := unreachableViolations(t, `
create module Repro;
create enumeration Repro.Color (Red 'Red', Blue 'Blue');
create microflow Repro.InCase ($C: Repro.Color) begin
  while true begin
    case $C
      when Red then break;
      when Blue then return;
    end case;
  end while;
  log info node 'Repro' 'after';
end;
create microflow Repro.InHandler () returns Boolean begin
  while true begin
    $X = call microflow Repro.Sub(In = 1) on error without rollback begin
      break;
    end error;
  end while;
  return true;
end;
`)
	if len(got) != 0 {
		t.Errorf("a break out of the loop was missed: %v", got)
	}
}

// A break in a NESTED loop leaves that loop, not the `while true` around it.
func TestUnreachableIgnoresBreakOfNestedLoop(t *testing.T) {
	got := unreachableViolations(t, `
create module Repro;
create microflow Repro.Nested ($N: Integer) begin
  declare $I Integer = 0;
  while true begin
    while $I < $N begin
      set $I = $I + 1;
      break;
    end while;
    if $I >= $N then return; end if;
  end while;
  return;
end;
`)
	if len(got["Nested"]) != 1 {
		t.Errorf("got %v, want one finding: the inner break does not leave the outer loop", got["Nested"])
	}
}

// A nanoflow is built by the same flow builder.
func TestUnreachableInNanoflow(t *testing.T) {
	got := unreachableViolations(t, `
create module Repro;
create nanoflow Repro.NF ($V: Integer) begin
  if $V < 0 then
    return;
    log info node 'Repro' 'negative';
  end if;
end;
`)
	if len(got["NF"]) != 1 {
		t.Errorf("got %v, want one finding", got["NF"])
	}
}
