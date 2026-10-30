// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1793 for check --references: what the full build now types a
// self-reference retrieve as, the check says too. The project is
// selfReferenceBackend (cmd_microflows_builder_retrieve_self_reference_test.go),
// whose header holds the 11.13.0 measurements.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// selfRefCheckErrors runs check --references' association-retrieve pass over
// flow, against the selfReferenceBackend project.
func selfRefCheckErrors(t *testing.T, flow string) []string {
	t.Helper()
	ctx, _ := newMockCtx(t, withBackend(selfReferenceBackend()))
	prog := parseMDL(t, flow)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, err := range validateAssociationRetrieveListUse(ctx, prog, sc) {
		out = append(out, err.Error())
	}
	return out
}

const selfRefSubFlows = `
create microflow Sample.SubList ($Nodes: list of Sample.Node)
begin
  log info node 'S' 'sub';
end;
create microflow Sample.SubOne ($Node: Sample.Node)
begin
  log info node 'S' 'sub';
end;
`

// TestAssociationRetrieveObjectUsedAsList: check --references refuses a list
// use of a retrieve that binds one object — a Reference followed from its FROM
// end, which a self-reference always is — naming the CE code the build
// reports. The controls are the shapes that build.
func TestAssociationRetrieveObjectUsedAsList(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string // nil = no error
	}{
		{"self-reference, count (the report)", `retrieve $P from $Node/Sample.Node_Parent;
  $n = count $P;`, []string{"count", "CE0013", "Input variable 'P' must be of type 'List'."}},
		{"self-reference, sum", `retrieve $P from $Node/Sample.Node_Parent;
  $n = sum $P by Amount;`, []string{"sum", "CE0013"}},
		{"self-reference, head", `retrieve $P from $Node/Sample.Node_Parent;
  $h = head $P;`, []string{"head", "CE0097", "The selected 'P' variable must be of type List."}},
		{"self-reference, filter", `retrieve $P from $Node/Sample.Node_Parent;
  $f = filter $P by Amount = 1;`, []string{"filter", "CE0097"}},
		{"self-reference, the first operand of contains", `retrieve $P from $Node/Sample.Node_Parent;
  $b = contains $Node in $P;`, []string{"contains", "CE0097"}},
		{"self-reference, reduce", `retrieve $P from $Node/Sample.Node_Parent;
  $r = reduce $P from 0 as Integer using $currentResult + 1;`, []string{"reduce", "CE0013"}},
		{"self-reference, loop", `retrieve $P from $Node/Sample.Node_Parent;
  loop $x in $P begin
    log info node 'S' 'x';
  end loop;`, []string{"a loop", "CE0100", "'P' is of type Sample.Node, but should be of type List."}},
		{"self-reference, a list parameter", `retrieve $P from $Node/Sample.Node_Parent;
  call microflow Sample.SubList(Nodes = $P);`, []string{"parameter Nodes of Sample.SubList", "CE0117"}},
		{"self-reference from a specialization, count", `retrieve $P from $Special/Sample.Node_Parent;
  $n = count $P;`, []string{"CE0013", "a Sample.SpecialNode"}},
		{"self-reference in a loop body, count", `loop $it in $Nodes begin
    retrieve $P from $it/Sample.Node_Parent;
    $n = count $P;
  end loop;`, []string{"CE0013"}},
		{"a forward Reference, count", `retrieve $C from $Line/Sample.Line_Node;
  $n = count $C;`, []string{"Sample.Line_Node", "CE0013", "Input variable 'C' must be of type 'List'."}},

		// Controls: the shapes that build (measured on 11.13.0), and the ones
		// whose shape is not established.
		{"control: self-reference, change", `retrieve $P from $Node/Sample.Node_Parent;
  change $P (Amount = 1);`, nil},
		{"control: self-reference, a member path", `retrieve $P from $Node/Sample.Node_Parent;
  declare $a Integer = $P/Amount;`, nil},
		{"control: self-reference, an object parameter", `retrieve $P from $Node/Sample.Node_Parent;
  call microflow Sample.SubOne(Node = $P);`, nil},
		{"control: self-reference set, count and loop", `retrieve $R from $Node/Sample.Node_Related;
  $n = count $R;
  loop $x in $R begin
    change $x (Amount = 1);
  end loop;`, nil},
		{"control: add, remove, clear (Change list activities mx check does not type)", `retrieve $P from $Node/Sample.Node_Parent;
  add $Node to $P;
  remove $Node from $P;
  clear $P;`, nil},
		{"control: range (mx check reports nothing on it)", `retrieve $P from $Node/Sample.Node_Parent;
  $r = range $P limit 1;`, nil},
		{"control: the object as a list operation's second operand", `retrieve $P from $Node/Sample.Node_Parent;
  $u = union $Nodes with $P;
  $b = contains $P in $Nodes;`, nil},
		{"control: a Reference from its TO end is the list", `retrieve $L from $Node/Sample.Line_Node;
  $n = count $L;`, nil},
		{"control: a rebinding clears the object", `retrieve $P from $Node/Sample.Node_Parent;
  retrieve $P from Sample.Node;
  $n = count $P;`, nil},
		{"control: a specialization of both ends of two entities is not judged", `retrieve $A from $Dog/Sample.Dog_Animal;
  $n = count $A;
  change $A (Name = 'x');
  retrieve $B from $Dog/Sample.Animal_Dog;
  $m = count $B;
  change $B (Name = 'x');`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := selfRefCheckErrors(t, selfRefSubFlows+`
create microflow Sample.F ($Node: Sample.Node, $Special: Sample.SpecialNode, $Line: Sample.Line,
  $Dog: Sample.Dog, $Nodes: list of Sample.Node)
begin
  `+tc.body+`
end;`)
			if tc.want == nil {
				if len(errs) != 0 {
					t.Fatalf("want no error, got %v", errs)
				}
				return
			}
			if len(errs) != 1 {
				t.Fatalf("want 1 error, got %d: %v", len(errs), errs)
			}
			for _, w := range append(tc.want, "MDL-RETRIEVE01", "one Sample.") {
				if !strings.Contains(errs[0], w) {
					t.Errorf("error should mention %q:\n%s", w, errs[0])
				}
			}
		})
	}
}

// TestAssociationRetrieveObjectUsedAsList_ScriptDeclaredSpecialization: the
// specialization and the self-reference are created by the script being
// checked, so neither is in the project yet; the start end is decided through
// the script's declarations, as exec decides it after creating them.
func TestAssociationRetrieveObjectUsedAsList_ScriptDeclaredSpecialization(t *testing.T) {
	ctx, _ := newMockCtx(t)
	prog := parseMDL(t, `create module Tmp;
create persistent entity Tmp.Order ( Code: String(50) );
create persistent entity Tmp.SpecialOrder extends Tmp.Order ( Extra: String(20) );
create association Tmp.Order_Parent from Tmp.Order to Tmp.Order;
create microflow Tmp.F ($S: Tmp.SpecialOrder) returns Integer as $n
begin
  retrieve $P from $S/Tmp.Order_Parent;
  $n = count $P;
  return $n;
end;
create microflow Tmp.G ($S: Tmp.SpecialOrder)
begin
  retrieve $P from $S/Tmp.Order_Parent;
  change $P (Code = 'x');
end;`)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	errs := validateAssociationRetrieveListUse(ctx, prog, sc)
	if len(errs) != 1 {
		t.Fatalf("want 1 error (the count in Tmp.F), got %d: %v", len(errs), errs)
	}
	for _, w := range []string{"statement 5", "a Tmp.SpecialOrder", "one Tmp.Order", "CE0013", "MDL-RETRIEVE01"} {
		if !strings.Contains(errs[0].Error(), w) {
			t.Errorf("error should mention %q:\n%s", w, errs[0])
		}
	}
}

// TestAssociationRetrieveSelfReferenceSetUsedAsObject: a self-reference is no
// longer outside MDL-RETRIEVE03. Over a reference set it is a list from the
// FROM end as from any other, and a member path through it is CE0117 (measured
// on 11.13.0) — reported, with a suggestion that does not guess which way a
// database constraint over the self-reference runs.
func TestAssociationRetrieveSelfReferenceSetUsedAsObject(t *testing.T) {
	errs := selfRefCheckErrors(t, `create microflow Sample.F ($Node: Sample.Node) returns Integer as $a
begin
  retrieve $R from $Node/Sample.Node_Related;
  declare $a Integer = $R/Amount;
  return $a;
end;`)
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %v", len(errs), errs)
	}
	for _, w := range []string{"MDL-RETRIEVE03", "member path `$R/…`", "is a list of Sample.Node", "CE0117"} {
		if !strings.Contains(errs[0], w) {
			t.Errorf("error should mention %q:\n%s", w, errs[0])
		}
	}
	if strings.Contains(errs[0], "where [") {
		t.Errorf("the suggestion should not spell a database constraint over a self-reference:\n%s", errs[0])
	}
}

// TestCheckSetOnSelfReferenceObject: check --references' flow validator types
// a self-reference retrieve as the object the full build now writes, so `set`
// on it is refused as it is for every object (MDL-SET01's CE7247), where it
// used to pass as a Change list Replace; over a self reference set it is still
// a list, and `set` is still a Change list Replace.
func TestCheckSetOnSelfReferenceObject(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(selfReferenceBackend()))
	shapes := checkAssociationShapes(ctx, nil)
	flow := func(assoc string) *ast.CreateMicroflowStmt {
		prog := parseMDL(t, `create microflow Sample.F ($A: Sample.Node, $B: Sample.Node)
begin
  retrieve $Cursor from $A/Sample.`+assoc+`;
  retrieve $Next from $B/Sample.`+assoc+`;
  set $Cursor = $Next;
end;`)
		return prog.Statements[0].(*ast.CreateMicroflowStmt)
	}
	ref := flow("Node_Parent")
	if got := strings.Join(validateFlowBody(ref.Parameters, nil, ref.Body, true, shapes), "\n"); !strings.Contains(got, "CE7247") ||
		!strings.Contains(got, "$Cursor") {
		t.Errorf("set on a self-reference retrieve: want the CE7247 refusal, got %q", got)
	}
	set := flow("Node_Related")
	if got := validateFlowBody(set.Parameters, nil, set.Body, true, shapes); len(got) != 0 {
		t.Errorf("set on a self reference set retrieve (a list): want no error, got %v", got)
	}
}
