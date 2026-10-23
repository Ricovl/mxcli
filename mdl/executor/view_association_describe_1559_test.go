// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// viewAssociationSetup gives a module a view entity whose `t.ID as VE_Target`
// column declares an association, and — the control — a plain association
// between two persistent entities.
const viewAssociationSetup = `mdl 1;
create module VEA;
create persistent entity VEA.Target (Code: String(20));
create persistent entity VEA.Other (Code: String(20));
create association VEA.Other_Target from VEA.Other to VEA.Target;
create view entity VEA.VE (
  Code: String(20)
) as (
  from VEA.Target as t
  select t.ID as VE_Target,
         t.Code as Code
);`

// mendixlabs/mxcli#1627 (ako/mxcli#1559): `describe module` printed a view
// entity's association as `create or modify association M.VE_Target from M.VE
// to M.Target`, which check and exec then refuse as CE6771 — so the module's
// own description did not re-execute. The association is declared by the OQL
// column, which the view entity's description already carries.
func TestDescribeModule_ViewAssociationIsItsOQLColumn(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, viewAssociationSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: "VEA"}, WithAll: true}); err != nil {
		t.Fatalf("describe module: %v\n%s", err, out.String())
	}
	described := out.String()

	if strings.Contains(described, "association VEA.VE_Target") {
		t.Errorf("describe module printed the view entity's association as a statement:\n%s", described)
	}
	// The column that declares it is still there…
	if !strings.Contains(described, "select t.ID as VE_Target") {
		t.Errorf("describe module lost the OQL column that declares the association:\n%s", described)
	}
	// …and a plain association is still described (the control).
	if !strings.Contains(described, "create or modify association VEA.Other_Target") {
		t.Errorf("describe module dropped a plain association:\n%s", described)
	}

	// The description re-executes: check and exec both accept it.
	assertAgree(t, exec, dir, described, "")

	// And the association survives the replay, still owned by the view.
	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: "VEA"}, WithAll: true}); err != nil {
		t.Fatalf("second describe: %v", err)
	}
	if again := out.String(); again != described {
		t.Errorf("describe → exec → describe is not a fixed point\nfirst:\n%s\nsecond:\n%s", described, again)
	}
}

// `describe association` on a view entity's association names where it is
// declared rather than printing a statement every check refuses. The output
// holds no statement at all, so a script built from it has nothing to refuse;
// the plain association, the control, still describes as one.
func TestDescribeAssociation_ViewAssociationPointsAtTheView(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, viewAssociationSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeAssociation, Name: ast.QualifiedName{Module: "VEA", Name: "VE_Target"}}); err != nil {
		t.Fatalf("describe association: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "create or modify association") {
		t.Errorf("describe association printed a statement for a view entity's association:\n%s", got)
	}
	for _, want := range []string{"VEA.VE", "VE_Target", "describe entity VEA.VE"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe association should name %q, got:\n%s", want, got)
		}
	}
	prog, errs := visitor.Build(got)
	if len(errs) > 0 {
		t.Fatalf("describe association output does not parse: %v\n%s", errs[0], got)
	}
	if n := len(prog.Statements); n != 0 {
		t.Errorf("describe association output holds %d statement(s), want none:\n%s", n, got)
	}

	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeAssociation, Name: ast.QualifiedName{Module: "VEA", Name: "Other_Target"}}); err != nil {
		t.Fatalf("describe association (control): %v", err)
	}
	if !strings.Contains(out.String(), "create or modify association VEA.Other_Target") {
		t.Errorf("control: a plain association must still describe as a statement:\n%s", out.String())
	}
}
