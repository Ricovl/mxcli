// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#1697 (mendixlabs/mxcli#1650): `$Var/<member>` in an expression,
// for a member the variable's entity does not have, passed check -p
// --references — the type pass even said "Expression types OK" — and exec
// wrote it; mxbuild then reported CE0117 "Error(s) in expression.". The
// reported shapes are the system `id` and a misspelling, on System.User and on
// an entity the same script creates.
//
// Measured on 11.13.0, one microflow per shape, in an app with no other error
// (a CE1613 elsewhere suppresses every CE0117 — mx check then reports none):
//
//	CE0117   $O/id, $O/id = $S/id, $U/Bogus, $O/Bogus, $O/number (for Number),
//	         $O/CreatedDate, $O/changedDate on an entity that does not store it,
//	         $O/owner, $O/Ord_Cust/Name (a bare association),
//	         $O/Mod.Nope/Name, $O/Mod.Ord_Cust/Mod.Cust/Nam
//	clean    $O/Number, an inherited attribute (also from another module and
//	         from System.User), $O/Mod.Ord_Cust/Name, $O/createdDate where stored
const exprMemberHead = `mdl 1;
create module B1697;
create module O1697;
create persistent entity B1697.Cust (Name: String(50));
create persistent entity B1697.Ord (Number: String(20), createdDate: AutoCreatedDate);
create persistent entity B1697.SubOrd extends B1697.Ord (Extra: String(20));
create association B1697.Ord_Cust from B1697.Ord to B1697.Cust;
create persistent entity O1697.Base (BaseName: String(20));
create persistent entity B1697.Spec extends O1697.Base (SpecName: String(20));
`

const exprMemberFlow = `create or modify microflow B1697.F ($O: B1697.Ord, $S: B1697.SubOrd, $U: System.User, $A: Administration.Account, $Sp: B1697.Spec) returns %s as $r
begin
  declare $r %s = %s;
  return $r;
end;
`

func exprMemberScript(typ, expr string) string {
	return exprMemberHead + fmt.Sprintf(exprMemberFlow, typ, typ, expr)
}

// The reported shapes and their siblings: check -p reports each, naming the
// member and the entity, and the id with the way to get what it was for.
func TestExpressionMissingMember_Reported(t *testing.T) {
	cases := []struct{ typ, expr, want string }{
		{"String", "toString($U/id)", `$U/id: System.User has no attribute "id"`},
		{"String", "toString($O/id)", "object's id is not readable in an expression"},
		{"Boolean", "$O/id = $S/id", `$S/id: B1697.SubOrd has no attribute "id"`},
		{"String", "$U/Bogus", `$U/Bogus: System.User has no attribute "Bogus"`},
		{"String", "$O/Bogus", `$O/Bogus: B1697.Ord has no attribute "Bogus"`},
		{"String", "$O/number", `$O/number: B1697.Ord has no attribute "number"`},
		{"DateTime", "$O/CreatedDate", "the system member is spelled `createdDate`"},
		{"DateTime", "$O/changedDate", `$O/changedDate: B1697.Ord has no attribute "changedDate"`},
		{"String", "toString($O/owner)", "the system member is spelled `System.owner`"},
		{"String", "$O/Ord_Cust/Name", "Ord_Cust is an association: an expression names it qualified, `$O/B1697.Ord_Cust/…`"},
		{"String", "$O/B1697.Nope/Name", `$O/B1697.Nope/Name: there is no association B1697.Nope`},
		{"String", "$O/B1697.Ord_Cust/B1697.Cust/Nam", `$O/B1697.Ord_Cust/B1697.Cust/Nam: B1697.Cust has no attribute "Nam"`},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			got := strings.Join(agreeCheck(t, exec, dir, exprMemberScript(c.typ, c.expr)), "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("check -p must report %q, reported:\n%s", c.want, got)
			}
			if !strings.Contains(got, "CE0117") {
				t.Errorf("the report must name what mxbuild says (CE0117), reported:\n%s", got)
			}
		})
	}
}

// What mxbuild builds clean is not reported.
func TestExpressionMember_ControlsAccepted(t *testing.T) {
	cases := []struct{ typ, expr string }{
		{"String", "$O/Number"},
		{"String", "$S/Number"},
		{"String", "$Sp/BaseName"},
		{"String", "$U/Name"},
		{"String", "$A/Name"},
		{"String", "$A/FullName"},
		{"String", "$O/B1697.Ord_Cust/Name"},
		{"String", "$O/B1697.Ord_Cust/B1697.Cust/Name"},
		// The target entity by its bare name: the builder qualifies it (#829).
		{"String", "$O/B1697.Ord_Cust/Cust/Name"},
		{"DateTime", "$O/createdDate"},
		{"DateTime", "$S/createdDate"},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, exprMemberScript(c.typ, c.expr), "")
		})
	}
}

// The reported repro, against the project: System.User is read from it.
func TestExpressionMissingMember_SystemUserRepro(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	const src = `create module ModI;
create microflow ModI.MF_UserIdText ($User: System.User) returns String as $r
begin
  declare $r String = toString($User/id);
  return $r;
end;
`
	got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
	for _, want := range []string{`$User/id: System.User has no attribute "id"`, "CommunityCommons.getGUID"} {
		if !strings.Contains(got, want) {
			t.Errorf("check -p must report %q, reported:\n%s", want, got)
		}
	}
}

// An expression in a nested block and in a loop over a typed list is judged
// against the entity the variable holds there.
func TestExpressionMissingMember_NestedAndLoop(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	src := exprMemberHead + `create microflow B1697.G ($l: List of B1697.Ord) returns Boolean
begin
  loop $it in $l
  begin
    if $it/Numbr = 'x' then
      log info node 'n' 'found';
    end if;
  end loop;
  return true;
end;
`
	got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
	if want := `$it/Numbr: B1697.Ord has no attribute "Numbr"`; !strings.Contains(got, want) {
		t.Errorf("check -p must report %q, reported:\n%s", want, got)
	}
}
