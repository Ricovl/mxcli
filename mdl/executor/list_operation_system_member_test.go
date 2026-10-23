// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#1629: `find $l by System.owner = $u` was refused at exec with
//
//	member "System.owner" on entity Shop.Owned is not a known association (and a
//	one-qualifier name cannot be an attribute) — create the association first
//	(`create or modify association System.owner from ... to ...`) or fix the name
//
// although mxbuild builds it, and describe printed a stored one as `by owner`,
// which re-executes as an attribute. Measured with mx check on 11.12.5:
//
//	find/filter by System.owner | System.changedBy, the root stores it    0 problems
//	  (own entity, a System.FileDocument specialization, a specialization of an owning root)
//	the same on an entity that does not store it, or its specialization   CE0091 "No member selected."
//	find by System.Owner                                                  CE1613
//	find over Administration.Account by System.User_Language              0 problems, and its
//	  describe `by User_Language` re-executed as an attribute: CE1613
const listSystemMemberHead = `mdl 1;
create module B1629;
create persistent entity B1629.Owned (Number: String(50), owner: AutoOwner, changedBy: AutoChangedBy);
create persistent entity B1629.Plain (Number: String(50));
create persistent entity B1629.Doc extends System.FileDocument (Title: String(50));
create persistent entity B1629.SubOwned extends B1629.Owned (Extra: String(50));
create persistent entity B1629.SubPlain extends B1629.Plain (Extra: String(50));
`

const listSystemMemberFlow = `create or modify microflow B1629.F ($l: List of %s, $User: System.User) returns Boolean
begin
  $m = %s $l by %s = $User;
  return true;
end;
`

// The forms mxbuild builds: check and exec accept them, the member is stored
// as the System association, and describe prints it back so that running the
// description again changes nothing.
func TestListOperationBySystemMember_Builds(t *testing.T) {
	cases := []struct{ entity, verb, member string }{
		{"B1629.Owned", "find", "System.owner"},
		{"B1629.Owned", "filter", "System.changedBy"},
		{"B1629.Doc", "find", "System.owner"},
		{"B1629.SubOwned", "find", "System.owner"},
	}
	for _, c := range cases {
		t.Run(c.entity+" "+c.verb+" "+c.member, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			flow := fmt.Sprintf(listSystemMemberFlow, c.entity, c.verb, c.member)
			assertAgree(t, exec, dir, listSystemMemberHead+flow, "")

			out.Reset()
			if err := agreeExec(t, exec, "describe microflow B1629.F;"); err != nil {
				t.Fatalf("describe: %v", err)
			}
			want := fmt.Sprintf("$m = %s $l by %s = $User;", c.verb, c.member)
			if !strings.Contains(out.String(), want) {
				t.Fatalf("describe must print %q, got:\n%s", want, out.String())
			}

			// Run the script again: the declared find and the stored one's
			// description must agree, or the flow is rewritten every run.
			out.Reset()
			if err := agreeExec(t, exec, "mdl 1;\n"+flow); err != nil {
				t.Fatalf("re-run: %v", err)
			}
			if !strings.Contains(out.String(), "Unchanged microflow") {
				t.Errorf("re-running the same find must change nothing, got:\n%s", out.String())
			}
		})
	}
}

// An entity that does not store the member: mxbuild reports CE0091 "No member
// selected.", so check -p and exec's pre-flight refuse it and name the fix.
func TestListOperationBySystemMember_EntityStoresNone(t *testing.T) {
	cases := []struct {
		entity, verb, member string
		want                 []string
	}{
		{"B1629.Plain", "find", "System.owner", []string{
			"B1629.Plain does not store System.owner", "CE0091", "alter entity B1629.Plain add attribute owner: AutoOwner"}},
		{"B1629.SubPlain", "filter", "System.changedBy", []string{
			"root B1629.Plain does not store System.changedBy", "alter entity B1629.Plain add attribute changedBy: AutoChangedBy"}},
	}
	for _, c := range cases {
		exec, _, dir := openPedAppCopy(t)
		src := listSystemMemberHead + fmt.Sprintf(listSystemMemberFlow, c.entity, c.verb, c.member)
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s %s by %s: check -p must report %q, reported:\n%s", c.entity, c.verb, c.member, w, got)
			}
		}
	}
}

// `System.Owner` is a different name (CE1613); the refusal says how it is spelled
// instead of advising to create a System association.
func TestListOperationBySystemMember_Spelling(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	src := listSystemMemberHead + fmt.Sprintf(listSystemMemberFlow, "B1629.Owned", "find", "System.Owner")
	assertAgree(t, exec, dir, src, "spelled System.owner")
	if got := strings.Join(agreeCheck(t, exec, dir, src), "\n"); strings.Contains(got, "create the association first") {
		t.Errorf("check still advises creating a System association:\n%s", got)
	}
}

// A System association by its qualified name. Over a specialization of
// System.User, describe printed it bare and the bare name re-executed as an
// attribute of the specialization (CE1613). Over System.User itself the bare
// name does resolve, so the member speller respelled the declared name bare
// while describe prints it qualified — and the flow was rewritten every run.
func TestListOperationBySystemAssociation_RoundTrips(t *testing.T) {
	for _, entity := range []string{"Administration.Account", "System.User"} {
		t.Run(entity, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			flow := fmt.Sprintf(`create or modify microflow B1629.G ($l: List of %s, $Lang: System.Language) returns Boolean
begin
  $m = find $l by System.User_Language = $Lang;
  return true;
end;
`, entity)
			assertAgree(t, exec, dir, "mdl 1;\ncreate module B1629;\n"+flow, "")
			out.Reset()
			if err := agreeExec(t, exec, "describe microflow B1629.G;"); err != nil {
				t.Fatalf("describe: %v", err)
			}
			if want := "$m = find $l by System.User_Language = $Lang;"; !strings.Contains(out.String(), want) {
				t.Fatalf("describe must print %q, got:\n%s", want, out.String())
			}
			out.Reset()
			if err := agreeExec(t, exec, "mdl 1;\n"+flow); err != nil {
				t.Fatalf("re-run: %v", err)
			}
			if !strings.Contains(out.String(), "Unchanged microflow") {
				t.Errorf("re-running the same find must change nothing, got:\n%s", out.String())
			}
		})
	}
}

func TestExtractFieldName_KeepsSystemAssociationsQualified(t *testing.T) {
	cases := []struct{ attr, assoc, want string }{
		{"", "System.owner", "System.owner"},
		{"", "System.User_Language", "System.User_Language"},
		{"", "M.Emp_Dept", "Emp_Dept"},
		{"M.Emp.Name", "", "Name"},
	}
	for _, c := range cases {
		if got := extractFieldName(c.attr, c.assoc); got != c.want {
			t.Errorf("extractFieldName(%q, %q) = %q, want %q", c.attr, c.assoc, got, c.want)
		}
	}
}

// The member speller puts a declared find in describe's spelling before it is
// compared with the stored one. Over a list of System.User the bare
// `User_Language` resolves to the same association, so the speller shortened
// the declared name while describe prints it qualified: the two never matched.
// The speller now spells a find's member exactly as describe does.
func TestDescribedMemberSpellings_SystemAssociationStaysQualified(t *testing.T) {
	sys, hr := model.ID("sys"), model.ID("hr")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "System":
				return &model.Module{BaseElement: model.BaseElement{ID: sys}, Name: name}, nil
			case "HR":
				return &model.Module{BaseElement: model.BaseElement{ID: hr}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case sys:
				return &domainmodel.DomainModel{
					Entities:     []*domainmodel.Entity{{Name: "User"}, {Name: "Language"}},
					Associations: []*domainmodel.Association{{Name: "User_Language"}},
				}, nil
			case hr:
				return &domainmodel.DomainModel{Entities: []*domainmodel.Entity{{Name: "Emp"}}}, nil
			}
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))
	prog, errs := visitor.Build(`create microflow HR.F ($Users: List of System.User, $Emps: List of HR.Emp, $Lang: System.Language, $U: System.User)
begin
  $A = find $Users by System.User_Language = $Lang;
  $B = find $Emps by System.owner = $U;
end;
`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	f := prog.Statements[0].(*ast.CreateMicroflowStmt)
	got := describedMemberSpellings(ctx, f.Parameters, f.Body, nil)
	for i, want := range []string{"System.User_Language", "System.owner"} {
		left := got[i].(*ast.ListOperationStmt).Condition.(*ast.BinaryExpr).Left
		name, _ := listOperationFieldName(left)
		if name != want {
			t.Errorf("find %d: spelled %q, want %q — describe prints a System association qualified", i, name, want)
		}
	}
}
