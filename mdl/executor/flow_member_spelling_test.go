// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#859 (rehearsal S3): a member named in another spelling than the
// one describe prints never matched its own stored activity. The declared body
// is compared in describe's spelling — but only where the builder stores the
// same member for both, so a real change of member still shows.
func TestDescribedMemberSpellings(t *testing.T) {
	hr, other := model.ID("hr"), model.ID("other")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "HR":
				return &model.Module{BaseElement: model.BaseElement{ID: hr}, Name: name}, nil
			case "Other":
				return &model.Module{BaseElement: model.BaseElement{ID: other}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case hr:
				return &domainmodel.DomainModel{
					Entities: []*domainmodel.Entity{
						{Name: "Dept", Attributes: []*domainmodel.Attribute{{Name: "Name"}, {Name: "SortOrder"}}},
						{Name: "Emp", Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
					},
					Associations: []*domainmodel.Association{{Name: "Emp_Dept"}},
				}, nil
			case other:
				// A same-named association in another module.
				return &domainmodel.DomainModel{Associations: []*domainmodel.Association{{Name: "Emp_Dept"}}}, nil
			}
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))

	const src = `create microflow HR.F ($Emp: HR.Emp, $Emps: List of HR.Emp, $D: HR.Dept)
begin
  retrieve $Depts from HR.Dept sort by HR.Dept.SortOrder asc, HR.Emp.Name asc;
  $New = create HR.Emp (HR.Emp.Name = 'x', HR.Emp_Dept = $D, Other.Emp_Dept = $D);
  change $Emp (HR.Emp.Name = 'y', Emp_Dept = $D, Other.Emp_Dept = $D);
  $Found = find $Emps by HR.Emp_Dept = $D;
  $Other = find $Emps by Other.Emp_Dept = $D;
  loop $X in $Emps
  begin
    change $X (Emp_Dept = $D);
  end loop;
end;
`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	f := prog.Statements[0].(*ast.CreateMicroflowStmt)
	before := cloneAST(reflect.ValueOf(f.Body)).Interface().([]ast.MicroflowStatement)
	got := describedMemberSpellings(ctx, f.Parameters, f.Body, nil)

	if !declaredMatches(before, f.Body) {
		t.Fatal("the declared body was rewritten in place; only the copy may be")
	}

	retrieve := got[0].(*ast.RetrieveStmt)
	if a := retrieve.SortColumns[0].Attribute; a != "SortOrder" {
		t.Errorf("sort on the retrieved entity's attribute: %q, want SortOrder", a)
	}
	// Control: an attribute of another entity is not the retrieved entity's.
	if a := retrieve.SortColumns[1].Attribute; a != "HR.Emp.Name" {
		t.Errorf("sort on another entity's attribute: %q, want it as written", a)
	}

	create := got[1].(*ast.CreateObjectStmt)
	for i, want := range []string{"Name", "Emp_Dept", "Other.Emp_Dept"} {
		if a := create.Changes[i].Attribute; a != want {
			t.Errorf("create member %d: %q, want %q", i, a, want)
		}
	}
	change := got[2].(*ast.ChangeObjectStmt)
	for i, want := range []string{"Name", "HR.Emp_Dept", "Other.Emp_Dept"} {
		if a := change.Changes[i].Attribute; a != want {
			t.Errorf("change member %d: %q, want %q", i, a, want)
		}
	}
	if left := got[3].(*ast.ListOperationStmt).Condition.(*ast.BinaryExpr).Left; !declaredMatches(left, &ast.IdentifierExpr{Name: "Emp_Dept"}) {
		t.Errorf("find by a member of the list's module: %#v, want Emp_Dept", left)
	}
	// Control: the other module's association is another member; describe
	// cannot print it short without it meaning HR.Emp_Dept.
	if left := got[4].(*ast.ListOperationStmt).Condition.(*ast.BinaryExpr).Left; declaredMatches(left, &ast.IdentifierExpr{Name: "Emp_Dept"}) {
		t.Errorf("find by another module's same-named association was shortened: %#v", left)
	}
	inLoop := got[5].(*ast.LoopStmt).Body[0].(*ast.ChangeObjectStmt)
	if a := inLoop.Changes[0].Attribute; a != "HR.Emp_Dept" {
		t.Errorf("change of a loop variable: %q, want HR.Emp_Dept", a)
	}

	// A variable whose declaring statement names no entity (a call's result)
	// is typed by the builder's variable types.
	prog, errs = visitor.Build(`create microflow HR.G ()
begin
  $Made = call microflow HR.NewEmp();
  change $Made (HR.Emp.Name = 'y');
end;
`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	g := prog.Statements[0].(*ast.CreateMicroflowStmt)
	typed := describedMemberSpellings(ctx, g.Parameters, g.Body, map[string]string{"Made": "HR.Emp"})
	if a := typed[1].(*ast.ChangeObjectStmt).Changes[0].Attribute; a != "Name" {
		t.Errorf("change of a call's result typed by the builder: %q, want Name", a)
	}
	// Control: without the builder's types the entity is unknown, so the
	// member stays as written.
	untyped := describedMemberSpellings(ctx, g.Parameters, g.Body, nil)
	if a := untyped[1].(*ast.ChangeObjectStmt).Changes[0].Attribute; a != "HR.Emp.Name" {
		t.Errorf("change of an untyped variable: %q, want it as written", a)
	}
}
