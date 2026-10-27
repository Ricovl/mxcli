// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1690: `retrieve $C from $O/Ord_Cust;` — an association path with
// the association's SHORT name — passed check -p --references and exec, and
// was stored as the association ".Ord_Cust", with an empty module. Measured on
// 11.13.0:
//
//	[CE1613] "The selected association '.Ord_Cust' no longer exists."
//	         at Retrieve object(s) activity 'Retrieve from association'
//
// and describe printed `retrieve $C from $O/.Ord_Cust;`, which does not parse.
// The builder joined the AST's empty Module to the name. A bare name now
// resolves against the start variable's entity — through its generalization
// chain, from either end — when exactly one association fits, and is refused
// otherwise; it is never written with an empty module.
const bareAssocHead = `mdl 1;
create module B1690;
create module O1690;
create persistent entity B1690.Cust (Name: String(50));
create persistent entity B1690.Ord (Number: String(20));
create persistent entity B1690.SubOrd extends B1690.Ord (Extra: String(20));
create association B1690.Ord_Cust from B1690.Ord to B1690.Cust;
create persistent entity O1690.Base (BaseName: String(20));
create persistent entity O1690.Thing (ThingName: String(20));
create association O1690.Base_Thing from O1690.Base to O1690.Thing;
create persistent entity B1690.Spec extends O1690.Base (SpecName: String(20));
`

// The reported shape, and the controls a resolution must keep accepting: the
// TO end, a specialization, an association inherited from another module, and
// a System association reached from a System.User specialization. Each is
// stored qualified, and describe prints that.
func TestBareRetrieveAssociation_ResolvesToTheQualifiedName(t *testing.T) {
	cases := []struct {
		name, params, retrieve, stored string
	}{
		{"from the owner", "$O: B1690.Ord", "retrieve $C from $O/Ord_Cust;", "B1690.Ord_Cust"},
		{"from the TO end", "$C: B1690.Cust", "retrieve $Os from $C/Ord_Cust;", "B1690.Ord_Cust"},
		{"from a specialization", "$S: B1690.SubOrd", "retrieve $C from $S/Ord_Cust;", "B1690.Ord_Cust"},
		{"inherited from another module", "$Sp: B1690.Spec", "retrieve $T from $Sp/Base_Thing;", "O1690.Base_Thing"},
		{"System association of a user entity", "$A: Administration.Account", "retrieve $R from $A/UserRoles;", "System.UserRoles"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			flow := "create or modify microflow B1690.F (" + c.params + ")\nbegin\n  " + c.retrieve + "\nend;\n"
			assertAgree(t, exec, dir, bareAssocHead+flow, "")

			unit := microflowUnit(t, exec, "B1690", "F")
			short := c.stored[strings.Index(c.stored, ".")+1:]
			if bytes.Contains(unit, []byte("\x00."+short+"\x00")) {
				t.Fatalf("stored the association as %q — an empty module, CE1613", "."+short)
			}
			if !bytes.Contains(unit, []byte(c.stored)) {
				t.Fatalf("the stored retrieve does not name %s", c.stored)
			}

			out.Reset()
			if err := agreeExec(t, exec, "describe microflow B1690.F;"); err != nil {
				t.Fatalf("describe: %v", err)
			}
			want := strings.Replace(c.retrieve, "/"+short+";", "/"+c.stored+";", 1)
			if !strings.Contains(out.String(), want) {
				t.Fatalf("describe must print %q, got:\n%s", want, out.String())
			}

			// Run the script again: the declared bare name and the stored
			// qualified one are the same member, so nothing changes.
			out.Reset()
			if err := agreeExec(t, exec, "mdl 1;\n"+flow); err != nil {
				t.Fatalf("re-run: %v", err)
			}
			if !strings.Contains(out.String(), "Unchanged microflow") {
				t.Errorf("re-running the same retrieve must change nothing, got:\n%s", out.String())
			}
		})
	}
}

// The entity and the association can come from the script itself, which is
// the reported shape: check resolves them from the declarations.
func TestBareRetrieveAssociation_ScriptDeclared(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	const src = `mdl 1;
create module S1690;
create persistent entity S1690.Cust (Name: String(50));
create persistent entity S1690.Ord (Number: String(20));
create association S1690.Ord_Cust from S1690.Ord to S1690.Cust;
create microflow S1690.GetCust ($O: S1690.Ord)
begin
  retrieve $C from $O/Ord_Cust;
end;
`
	assertAgree(t, exec, dir, src, "")
	if unit := microflowUnit(t, exec, "S1690", "GetCust"); !bytes.Contains(unit, []byte("S1690.Ord_Cust")) {
		t.Fatal("the stored retrieve does not name S1690.Ord_Cust")
	}
}

// What cannot be resolved is refused by check -p and by exec alike, with the
// qualified spelling to write instead.
func TestBareRetrieveAssociation_Refused(t *testing.T) {
	t.Run("no such association", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		src := bareAssocHead + `create microflow B1690.F ($O: B1690.Ord)
begin
  retrieve $C from $O/Ord_Kust;
end;
`
		assertAgree(t, exec, dir, src, `no association called "Ord_Kust" has B1690.Ord`)
	})
	t.Run("an association the entity is no end of", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		src := bareAssocHead + `create microflow B1690.F ($Sp: B1690.Spec)
begin
  retrieve $C from $Sp/Ord_Cust;
end;
`
		assertAgree(t, exec, dir, src, `no association called "Ord_Cust" has B1690.Spec`)
	})
	t.Run("two modules each have one", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		src := bareAssocHead + `create association O1690.Ord_Cust from O1690.Thing to B1690.Ord;
create microflow B1690.F ($O: B1690.Ord)
begin
  retrieve $C from $O/Ord_Cust;
end;
`
		assertAgree(t, exec, dir, src, `"Ord_Cust" could be any of B1690.Ord_Cust, O1690.Ord_Cust`)
	})
}

// A start variable whose entity the builder does not know cannot be resolved,
// and the bare name cannot be written — refused rather than stored as ".Name".
func TestBareRetrieveAssociation_UnknownStartEntityIsRefusedByExec(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{}, declaredVars: map[string]string{}}
	fb.addRetrieveAction(&ast.RetrieveStmt{Variable: "C", StartVariable: "X",
		Source: ast.QualifiedName{Name: "Ord_Cust"}})
	got := strings.Join(fb.GetErrors(), "\n")
	if !strings.Contains(got, "the association is not qualified") {
		t.Fatalf("a bare association from an unknown entity must be refused, got: %q", got)
	}
}

// diff-then-patch compares a declared flow with describe's spelling, which is
// the stored, qualified one. The speller qualifies a bare retrieve association
// the way the builder resolves it, or every run rewrote the flow.
func TestBareRetrieveAssociation_SpellerQualifies(t *testing.T) {
	exec, _, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, bareAssocHead); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prog, errs := visitor.Build(`create microflow B1690.F ($S: B1690.SubOrd)
begin
  retrieve $C from $S/Ord_Cust;
end;
`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	f := prog.Statements[0].(*ast.CreateMicroflowStmt)
	got := describedMemberSpellings(exec.newExecContext(context.Background()), f.Parameters, f.Body, nil)
	if src := got[0].(*ast.RetrieveStmt).Source.String(); src != "B1690.Ord_Cust" {
		t.Errorf("declared retrieve spelled %q; describe prints B1690.Ord_Cust", src)
	}
}
