// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#1646: find, filter, sort or an aggregate by a member the list's
// entity does not have passed check -p and exec, and mxbuild then failed once
// per activity (measured on 11.12.5 and again on 11.13.0):
//
//	[CE1613] "The selected attribute 'Shop.Order.Numbr' no longer exists."  at list operation activity 'Find'
//	[CE1613] "The selected attribute 'Shop.Order.Totl' no longer exists."   at aggregate list activity …
//
// The builder resolved the bare name and, when it named nothing, fell back to
// writing `<entity>.<name>` anyway — the fabricated attribute mxbuild rejects —
// while the same misspelling in a retrieve constraint was refused (#1213). Now
// the builder and check ask one question, flowMemberVerdict, and refuse a name
// that is neither an attribute, an association nor a stored system member of
// the entity or anything it extends.
const listMemberHead = `mdl 1;
create module B1646;
create module O1646;
create persistent entity B1646.Cust (Name: String(50));
create persistent entity B1646.Ord (Number: String(20), Total: Decimal, owner: AutoOwner, createdDate: AutoCreatedDate);
create persistent entity B1646.SubOrd extends B1646.Ord (Extra: String(20));
create association B1646.Ord_Cust from B1646.Ord to B1646.Cust;
create persistent entity O1646.Base (BaseName: String(20));
create persistent entity O1646.Thing (ThingName: String(20));
create association O1646.Base_Thing from O1646.Base to O1646.Thing;
create persistent entity B1646.Spec extends O1646.Base (SpecName: String(20));
`

const listMemberFlow = `create or modify microflow B1646.F ($l: List of B1646.Ord, $sl: List of B1646.SubOrd, $spl: List of B1646.Spec, $c: B1646.Cust, $th: O1646.Thing, $u: System.User) returns Boolean
begin
  %s
  return true;
end;
`

// The reported misspellings: check -p and exec both refuse each, naming the
// entity and the member, rather than writing the fabricated attribute.
func TestListOperationMissingMember_Refused(t *testing.T) {
	cases := []struct{ stmt, want string }{
		{"$m = find $l by Numbr = '42';", `find $l by Numbr: B1646.Ord has no member "Numbr"`},
		{"$m = filter $l by Totl = 5;", `filter $l by Totl: B1646.Ord has no member "Totl"`},
		{"$m = sort $l by Totl asc;", `sort $l by Totl: B1646.Ord has no member "Totl"`},
		{"$m = sum $l by Totl;", `sum $l by Totl: B1646.Ord has no member "Totl"`},
		{"$m = maximum $sl by Extr;", `maximum $sl by Extr: B1646.SubOrd has no member "Extr"`},
		// `owner` is the association System.owner, which a bare name does not
		// reach: written as the attribute B1646.Ord.owner, CE1613 (11.13.0).
		{"$m = find $l by owner = $u;", "the system member is spelled `System.owner`"},
		// A sort reads a stored system date; an aggregate does not: CE0226,
		// on the entity that stores it and on a specialization (11.13.0).
		{"$m = maximum $l by createdDate;", "maximum $l by createdDate: an aggregate cannot read the system member createdDate"},
		{"$m = maximum $sl by createdDate;", "CE0226"},
	}
	for _, c := range cases {
		t.Run(c.stmt, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, listMemberHead+fmt.Sprintf(listMemberFlow, c.stmt), c.want)
		})
	}
}

// What mxbuild builds clean stays accepted (each measured with mx check on
// 11.13.0): an own and an inherited attribute, an own association by its bare
// name, one inherited from a generalization in another module (#1637), the
// System.owner association (#1629), and the system dates the root stores. A
// list whose entity is not known (a call's result) is not judged at all.
func TestListOperationMember_ControlsAccepted(t *testing.T) {
	stmts := []string{
		"$m = find $sl by Number = '1';",
		"$m = sort $sl by Number asc;",
		"$m = sum $sl by Total;",
		"$m = find $l by Ord_Cust = $c;",
		"$m = find $spl by Base_Thing = $th;",
		"$m = find $l by System.owner = $u;",
		"$m = sort $l by createdDate asc;",
		"$m = sort $sl by createdDate desc;",
		"$m = sort $spl by BaseName desc;",
	}
	for _, stmt := range stmts {
		t.Run(stmt, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, listMemberHead+fmt.Sprintf(listMemberFlow, stmt), "")
		})
	}
	t.Run("untyped list", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		src := listMemberHead + `create microflow B1646.Src () returns List of B1646.Ord
begin
  retrieve $all from B1646.Ord;
  return $all;
end;
create microflow B1646.G ($x: Integer) returns Boolean
begin
  $l = call microflow B1646.Src();
  $m = sort $l by Number asc;
  return true;
end;
`
		assertAgree(t, exec, dir, src, "")
	})
}

// An aggregate by an INHERITED attribute was written against the list's own
// entity — `B1646.SubOrd.Total` — which mxbuild reports as CE1613 "The selected
// attribute 'B1646.SubOrd.Total' no longer exists" (measured on 11.13.0). It is
// qualified against the entity that declares it, as a sort already was
// (mendixlabs/mxcli#1430).
func TestAggregateInheritedAttribute_StoresTheDeclaringEntity(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, listMemberHead+fmt.Sprintf(listMemberFlow, "$m = sum $sl by Total;"), "")
	unit := microflowUnit(t, exec, "B1646", "F")
	if bytes.Contains(unit, []byte("B1646.SubOrd.Total")) {
		t.Fatal("stored the inherited attribute as B1646.SubOrd.Total — CE1613")
	}
	if !bytes.Contains(unit, []byte("B1646.Ord.Total")) {
		t.Fatal("the aggregate does not name B1646.Ord.Total")
	}
}

// The builder's refusal and check's report are one message: an exec that
// skipped the preflight (exec -c, the REPL, --no-check) says what check says.
func TestListOperationMissingMember_ExecSaysWhatCheckSays(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	src := listMemberHead + fmt.Sprintf(listMemberFlow, "$m = find $l by Numbr = '42';")
	checkSays := strings.Join(agreeCheck(t, exec, dir, src), "\n")
	err := agreeExec(t, exec, src)
	if err == nil {
		t.Fatal("exec accepted find by a missing member")
	}
	const msg = `find $l by Numbr: B1646.Ord has no member "Numbr" — it has Number, Total`
	if !strings.Contains(checkSays, msg) || !strings.Contains(err.Error(), msg) {
		t.Errorf("check and exec must both say %q\ncheck: %s\nexec: %v", msg, checkSays, err)
	}
}

// A member the same script adds before the flow is not refused: check leaves
// an altered entity unjudged, and exec's builder runs after the alter.
// `create or modify` of a STORED flow is the shape where check's flow-verdict
// simulation builds the declared flow against the project as stored.
func TestListOperationMember_AddedByTheScriptIsAccepted(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, listMemberHead+fmt.Sprintf(listMemberFlow, "$m = find $l by Number = '1';")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	src := "mdl 1;\nalter entity B1646.Ord add attribute Code: String(10);\n" +
		fmt.Sprintf(listMemberFlow, "$m = find $l by Code = 'x';\n  $s = sort $l by Code asc;\n  $t = sum $l by Total;")
	assertAgree(t, exec, dir, src, "")
}
