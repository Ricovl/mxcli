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

// ako/mxcli#1637: describe prints a find's association member short — `by
// Person_Dept` for Other.Person_Dept — and the builder resolved a bare name
// only in the list entity's own module. An association inherited from a
// generalization in another module was therefore not found, and the bare name
// was written as an attribute of the entity. Measured on 11.12.5: the
// description of a clean find, executed as a new microflow, passed check -p
// and exec and then failed
//
//	[CE1613] "The selected attribute 'HR.Emp.Person_Dept' no longer exists."
//
// Re-running it over the stored flow said "Unchanged", so only a rebuild (a
// copy, a fresh project) showed it.
const inheritedAssocHead = `mdl 1;
create module Other;
create module HR;
create persistent entity Other.Person (Name: String(50));
create persistent entity Other.Dept (Name: String(50));
create association Other.Person_Dept from Other.Person to Other.Dept;
create persistent entity HR.Emp extends Other.Person (Code: String(10));
create microflow HR.FindByDept ($l: List of HR.Emp, $d: Other.Dept) returns Boolean
begin
  $m = find $l by Other.Person_Dept = $d;
  return true;
end;
`

// microflowUnit returns a stored microflow's unit bytes.
func microflowUnit(t *testing.T, exec *Executor, module, name string) []byte {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == name && h.GetModuleName(h.FindModuleID(m.ContainerID)) == module {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte(nil), raw...)
		}
	}
	t.Fatalf("%s.%s not found", module, name)
	return nil
}

// assertStoresInheritedAssociation fails when the unit references the member
// as an attribute of HR.Emp instead of the association Other.Person_Dept.
func assertStoresInheritedAssociation(t *testing.T, unit []byte, what string) {
	t.Helper()
	if bytes.Contains(unit, []byte("HR.Emp.Person_Dept")) {
		t.Errorf("%s: stored as the attribute HR.Emp.Person_Dept — mxbuild reports CE1613", what)
	}
	if !bytes.Contains(unit, []byte("Other.Person_Dept")) {
		t.Errorf("%s: does not reference the association Other.Person_Dept", what)
	}
}

func TestInheritedAssociation_DescribeExecutesAsTheSameMember(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, inheritedAssocHead, "")

	out.Reset()
	if err := agreeExec(t, exec, "describe microflow HR.FindByDept;"); err != nil {
		t.Fatalf("describe: %v", err)
	}
	described := out.String()
	if !strings.Contains(described, "$m = find $l by Person_Dept = $d;") {
		t.Fatalf("describe output changed shape; this test runs it back as written:\n%s", described)
	}

	// Execute the description as a new microflow: a fresh build, with nothing
	// stored to splice against.
	i := strings.Index(described, "create or modify microflow HR.FindByDept")
	if i < 0 {
		t.Fatalf("no create statement in the description:\n%s", described)
	}
	copySrc := "mdl 1;\n" + strings.Replace(described[i:], "microflow HR.FindByDept ", "microflow HR.FindByDeptCopy ", 1)
	assertAgree(t, exec, dir, copySrc, "")
	assertStoresInheritedAssociation(t, microflowUnit(t, exec, "HR", "FindByDeptCopy"), "the executed description")
}

// The same bare name written by hand, in a find and in a create's members.
func TestInheritedAssociation_BareNameResolves(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	const flows = `create microflow HR.Bare ($l: List of HR.Emp, $d: Other.Dept) returns Boolean
begin
  $m = find $l by Person_Dept = $d;
  $e = create HR.Emp (Code = 'x', Person_Dept = $d);
  return true;
end;
`
	assertAgree(t, exec, dir, inheritedAssocHead+flows, "")
	unit := microflowUnit(t, exec, "HR", "Bare")
	assertStoresInheritedAssociation(t, unit, "a hand-written bare name")
}

// diff-then-patch compares a declared flow in describe's spelling. describe
// prints `Person_Dept`; the speller shortens the declared Other.Person_Dept only
// when the short name resolves to the same member, which it did not — so the
// declared find and its own stored activity were spelled differently.
func TestInheritedAssociation_SpellerAgreesWithDescribe(t *testing.T) {
	exec, _, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, inheritedAssocHead); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prog, errs := visitor.Build(`create microflow HR.F ($l: List of HR.Emp, $d: Other.Dept)
begin
  $m = find $l by Other.Person_Dept = $d;
end;
`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	f := prog.Statements[0].(*ast.CreateMicroflowStmt)
	ctx := exec.newExecContext(context.Background())
	got := describedMemberSpellings(ctx, f.Parameters, f.Body, nil)
	name, _ := listOperationFieldName(got[0].(*ast.ListOperationStmt).Condition.(*ast.BinaryExpr).Left)
	if name != "Person_Dept" {
		t.Errorf("declared find spelled %q; describe prints Person_Dept", name)
	}
}

// Control: an inherited ATTRIBUTE of the same bare name is still the attribute —
// the association lookup comes after it.
func TestInheritedAssociation_AttributeStillWins(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	const src = `mdl 1;
create module Other;
create module HR;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Emp extends Other.Person (Code: String(10));
create microflow HR.ByName ($l: List of HR.Emp) returns Boolean
begin
  $m = find $l by Name = 'x';
  return true;
end;
`
	assertAgree(t, exec, dir, src, "")
	if unit := microflowUnit(t, exec, "HR", "ByName"); !bytes.Contains(unit, []byte("Other.Person.Name")) {
		t.Errorf("find by an inherited attribute no longer stores Other.Person.Name")
	}
}
