// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1516: a retrieve over a Reference from its TO entity (the
// "one" side) binds a LIST, and using it as an object passed check --references
// and failed the build. Measured on 11.6.6 and 11.15.0 alike, with
// Tmp.Line_Header from Tmp.Line to Tmp.Header:
//
//	retrieve $L from $h/Tmp.Line_Header;            clean (an unused list)
//	… $n = count($L);                                clean
//	… declare $n Integer = $L/Qty;                   CE0117 "Error(s) in expression."
//	… change $L (Qty = 5);                           CE7247 "Variable 'L' is of type List of Tmp.Line, but should be of type Object."
//	… call microflow Tmp.Sub(line = $L);             CE0117 (Sub takes one Tmp.Line)
//	retrieve $H from $l/Tmp.Line_Header; $H/Name     clean (the FROM side is one object)
package executor

import (
	"strings"
	"testing"
)

const assocListSetup = `
create module Tmp;
create persistent entity Tmp.Header ( Name: String(100) );
create persistent entity Tmp.Line ( Qty: Integer );
create association Tmp.Line_Header from Tmp.Line to Tmp.Header;
create association Tmp.Line_Tags from Tmp.Line to Tmp.Header type ReferenceSet;
create association Tmp.Line_Main from Tmp.Line to Tmp.Header owner Both;
create microflow Tmp.Sub ($line: Tmp.Line) returns Boolean as $b
begin
  declare $b Boolean = true;
  return $b;
end;
create microflow Tmp.SubList ($lines: list of Tmp.Line) returns Boolean as $b
begin
  declare $b Boolean = true;
  return $b;
end;
`

func assocListErrors(t *testing.T, flow string) []string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	prog := parseMDL(t, assocListSetup+flow)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, err := range validateAssociationRetrieveListUse(ctx, prog, sc) {
		out = append(out, err.Error())
	}
	return out
}

func TestAssociationRetrieveListUsedAsObject(t *testing.T) {
	for _, tc := range []struct {
		name string
		flow string
		want string // "" = no error
	}{
		{"member path on the list (CE0117)", `
create microflow Tmp.H ($h: Tmp.Header) returns Integer as $n
begin
  retrieve $L from $h/Tmp.Line_Header;
  declare $n Integer = $L/Qty;
  return $n;
end;`, "member path `$L/…`"},
		{"change over the list (CE7247)", `
create microflow Tmp.C ($h: Tmp.Header)
begin
  retrieve $L from $h/Tmp.Line_Header;
  change $L (Qty = 5);
end;`, "`change $L`"},
		{"the list passed to an object parameter (CE0117)", `
create microflow Tmp.P ($h: Tmp.Header) returns Boolean as $b
begin
  retrieve $L from $h/Tmp.Line_Header;
  $b = call microflow Tmp.Sub(line = $L);
  return $b;
end;`, "parameter line of Tmp.Sub"},
		{"a ReferenceSet is a list from either end", `
create microflow Tmp.R ($l: Tmp.Line) returns Boolean as $b
begin
  retrieve $T from $l/Tmp.Line_Tags;
  declare $b Boolean = $T/Name != empty;
  return $b;
end;`, "member path `$T/…`"},
		{"the start variable is a loop iterator", `
create microflow Tmp.LoopIt ($hs: list of Tmp.Header)
begin
  loop $h in $hs begin
    retrieve $L from $h/Tmp.Line_Header;
    change $L (Qty = 1);
  end loop;
end;`, "`change $L`"},

		// Controls: the shapes that build.
		{"control: the bare list retrieve (clean on 11.6.6 and 11.15)", `
create microflow Tmp.F ($h: Tmp.Header) returns Boolean as $b
begin
  retrieve $L from $h/Tmp.Line_Header;
  declare $b Boolean = true;
  return $b;
end;`, ""},
		{"control: the list used as a list", `
create microflow Tmp.G ($h: Tmp.Header) returns Integer as $n
begin
  retrieve $L from $h/Tmp.Line_Header;
  $n = count($L);
  $b = call microflow Tmp.SubList(lines = $L);
  loop $x in $L begin
    change $x (Qty = 1);
  end loop;
  return $n;
end;`, ""},
		{"control: from the FROM side is one object", `
create microflow Tmp.K ($l: Tmp.Line) returns Boolean as $b
begin
  retrieve $H from $l/Tmp.Line_Header;
  declare $b Boolean = $H/Name != empty;
  return $b;
end;`, ""},
		{"control: owner Both is one-to-one, one object from the TO side", `
create microflow Tmp.B ($h: Tmp.Header) returns Integer as $n
begin
  retrieve $L from $h/Tmp.Line_Main;
  declare $n Integer = $L/Qty;
  return $n;
end;`, ""},
		{"control: a rebinding clears the list", `
create microflow Tmp.Re ($h: Tmp.Header) returns Integer as $n
begin
  retrieve $L from $h/Tmp.Line_Header;
  retrieve $L from Tmp.Line first;
  declare $n Integer = $L/Qty;
  return $n;
end;`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := assocListErrors(t, tc.flow)
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("want no error, got %v", errs)
				}
				return
			}
			if len(errs) != 1 {
				t.Fatalf("want 1 error containing %q, got %d: %v", tc.want, len(errs), errs)
			}
			for _, w := range []string{tc.want, "MDL-RETRIEVE03", "list of Tmp."} {
				if !strings.Contains(errs[0], w) {
					t.Errorf("error should mention %q:\n%s", w, errs[0])
				}
			}
		})
	}
}

// The builder and the check must agree on which retrieves are lists: the check
// exists to describe what exec writes.
func TestAssociationRetrieveYieldsList(t *testing.T) {
	for _, tc := range []struct {
		refSet, ownerBoth, fromTo, want bool
	}{
		{false, false, false, false}, // Reference from FROM: object
		{false, false, true, true},   // Reference from TO: list
		{false, true, false, false},  // owner Both: one-to-one
		{false, true, true, false},
		{true, false, false, true}, // ReferenceSet: list either way
		{true, false, true, true},
		{true, true, false, true},
		{true, true, true, true},
	} {
		if got := associationRetrieveYieldsList(tc.refSet, tc.ownerBoth, tc.fromTo); got != tc.want {
			t.Errorf("refSet=%v ownerBoth=%v fromTo=%v: got %v, want %v", tc.refSet, tc.ownerBoth, tc.fromTo, got, tc.want)
		}
	}
}
