// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#874. A module-qualified attribute inside an XPath constraint,
//
//	retrieve $L from M.Emp where [M.Emp.Name = 'y'];
//
// was stored as ['Name' = 'y']: every XPath writer read a three-part name as an
// enumeration value (M.Enum.Value -> 'Value') without knowing which entity the
// constraint is evaluated on. The stored constraint compares two constants, so
// the retrieve silently returns every row or none, and mxcli check and mx check
// both pass it. Measured on mx check 11.14.0: [Name = 'y'] is clean,
// [M.Emp.Name = 'y'] is CE0161, and so is [Kind = M.Enum.Value] — so the
// attribute is stored bare and an enumeration value stays a string literal.
//
// A silent wrong write, fixed under mdl 0 and mdl 1 alike (ADR-0011).

// retrieveConstraintOf builds the first retrieve of a microflow from MDL source
// and returns the XPath constraint the writer stores for it.
func retrieveConstraintOf(t *testing.T, src string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q: %v", src, errs)
	}
	for _, s := range prog.Statements {
		mf, ok := s.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		r := mf.Body[0].(*ast.RetrieveStmt)
		fb := &flowBuilder{varTypes: map[string]string{}}
		fb.addRetrieveAction(r)
		if len(fb.errors) > 0 {
			t.Fatalf("%q: builder errors: %v", src, fb.errors)
		}
		act := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.RetrieveAction)
		return act.Source.(*microflows.DatabaseRetrieveSource).XPathConstraint
	}
	t.Fatalf("%q: no microflow", src)
	return ""
}

func TestRetrieveXPath_QualifiedAttributeIsStoredAsTheAttribute(t *testing.T) {
	cases := []struct{ name, where, want string }{
		{"bracketed", "[M.Emp.Name = 'y']", "[Name = 'y']"},
		{"expression form", "M.Emp.Name = 'y'", "[Name = 'y']"},
		{"beside an enum value", "M.Emp.Name = 'y' and M.Emp.Kind = M.EmpKind.Staff",
			"[Name = 'y' and Kind = 'Staff']"},
		{"bracketed, beside an enum value", "[M.Emp.Kind = M.EmpKind.Staff or M.Emp.Name = 'y']",
			"[Kind = 'Staff' or Name = 'y']"},
		// Inside a predicate on a path step the constraint is evaluated on that
		// step's entity, so its attributes are qualified with it.
		{"nested predicate", "[M.Emp_Dept/M.Dept[M.Dept.Code = 'x']]", "[M.Emp_Dept/M.Dept[Code = 'x']]"},
		{"inside a function", "[contains(M.Emp.Name, 'y')]", "[contains(Name, 'y')]"},
		// CONTROLS: what the fix must not touch.
		{"enum value", "[Kind = M.EmpKind.Staff]", "[Kind = 'Staff']"},
		{"enum value, expression form", "Kind = M.EmpKind.Staff", "[Kind = 'Staff']"},
		{"string literal naming the attribute", "[Name = 'M.Emp.Name']", "[Name = 'M.Emp.Name']"},
		{"bare attribute", "[Name = 'y']", "[Name = 'y']"},
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		for _, tc := range cases {
			t.Run(strings.TrimSpace(header+" "+tc.name), func(t *testing.T) {
				src := header + "create microflow M.F () begin\n  retrieve $L from M.Emp where " + tc.where + ";\nend;"
				if got := retrieveConstraintOf(t, src); got != tc.want {
					t.Errorf("stored constraint\n got  %q\n want %q", got, tc.want)
				}
			})
		}
	}
}

// The re-run: describe prints the stored constraint with the bare attribute, so
// a script that spells it qualified must still be the same retrieve — otherwise
// create or modify re-splices it on every run (the twice-exec rule).
func TestRetrieveXPath_QualifiedAttributeMatchesItsDescribeForm(t *testing.T) {
	parse := func(src string) *ast.RetrieveStmt {
		t.Helper()
		prog, errs := visitor.Build("mdl 1;\ncreate microflow M.F () begin\n  " + src + "\nend;")
		if len(errs) > 0 {
			t.Fatalf("%q: %v", src, errs)
		}
		for _, s := range prog.Statements {
			if mf, ok := s.(*ast.CreateMicroflowStmt); ok {
				return mf.Body[0].(*ast.RetrieveStmt)
			}
		}
		t.Fatalf("%q: no microflow", src)
		return nil
	}
	stored := parse("retrieve $L from M.Emp\n    where Name = 'y';")
	for _, declared := range []string{
		"retrieve $L from M.Emp where [M.Emp.Name = 'y'];",
		"retrieve $L from M.Emp where M.Emp.Name = 'y';",
	} {
		if !declaredMatches(parse(declared), stored) {
			t.Errorf("%s\n does not match its describe form `where Name = 'y'`", declared)
		}
	}
	// CONTROL: a different attribute is a different retrieve.
	if declaredMatches(parse("retrieve $L from M.Emp where [M.Emp.Code = 'y'];"), stored) {
		t.Error("[M.Emp.Code = 'y'] matched `where Name = 'y'`")
	}
}

// The other three writers take the constraint as text.
func TestStoredXPath_EveryWriterResolvesQualifiedAttributes(t *testing.T) {
	t.Run("page datasource", func(t *testing.T) {
		pb := &pageBuilder{}
		ds := &ast.DataSourceV3{Type: "database", Reference: "M.Emp", Where: "[M.Emp.Name = 'y' and Kind = M.EmpKind.Staff]"}
		src := &pages.DatabaseSource{}
		if err := pb.applyDatabaseClausesV3(src, ds, "M.Emp"); err != nil {
			t.Fatal(err)
		}
		if want := "[Name = 'y' and Kind = 'Staff']"; src.XPathConstraint != want {
			t.Errorf("page datasource constraint\n got  %q\n want %q", src.XPathConstraint, want)
		}
	})
	t.Run("access rule", func(t *testing.T) {
		if got, want := accessRuleXPathConstraint(nil, "[M.Emp.Name = 'y']", "M.Emp"), "[Name = 'y']"; got != want {
			t.Errorf("access rule constraint\n got  %q\n want %q", got, want)
		}
	})
	t.Run("workflow targeting", func(t *testing.T) {
		users := buildUserTask(&ast.WorkflowUserTaskNode{Name: "t",
			Targeting: ast.WorkflowTargetingNode{Kind: "xpath", XPath: "[System.User.Name = 'admin']"}})
		if xp, ok := users.UserSource.(*workflows.XPathBasedUserSource); !ok || xp.XPath != "[Name = 'admin']" {
			t.Errorf("user targeting = %#v, want XPath [Name = 'admin']", users.UserSource)
		}
		groups := buildUserTask(&ast.WorkflowUserTaskNode{Name: "t",
			Targeting: ast.WorkflowTargetingNode{Kind: "group_xpath", XPath: "[System.WorkflowGroup.Name = 'G']"}})
		if xp, ok := groups.UserSource.(*workflows.XPathGroupSource); !ok || xp.XPath != "[Name = 'G']" {
			t.Errorf("group targeting = %#v, want XPath [Name = 'G']", groups.UserSource)
		}
	})
}

// The formatter re-reads a constraint that does not fit on one line and prints
// it again. It must not decide what a three-part name is: reading it as an enum
// value turned a qualified attribute into a string literal on the way (the same
// defect, for long constraints only).
func TestFormatXPathConstraint_KeepsThreePartNames(t *testing.T) {
	in := "[M.Emp.Name = 'a long enough value to force a line break' and Kind = M.EmpKind.Staff and Code != empty]"
	got := visitor.FormatXPathConstraint(in)
	for _, name := range []string{"M.Emp.Name", "M.EmpKind.Staff"} {
		if !strings.Contains(got, name) {
			t.Errorf("FormatXPathConstraint dropped %s:\n%s", name, got)
		}
	}
}

// An access rule's constraint used to be stored verbatim when it was short, so a
// three-part name the writer cannot place — an attribute qualified with a
// generalization of the entity, or with an unrelated entity — failed loudly
// (CE0161). Reading every such name as an enumeration value instead would turn
// that error into ['Name' = 'x']: a constraint that compares two constants and
// passes mx check (measured, 11.14.0, grant on Administration.Account where
// [System.User.Name = 'x']). The writer has the project, so it resolves the
// generalization chain and leaves a name qualified with any other entity as
// written.
func TestAccessRuleXPath_GeneralizationAndForeignEntityNames(t *testing.T) {
	sales, admin, system := mkModule("Sales"), mkModule("Administration"), mkModule("System")
	account := mkEntity(admin.ID, "Account")
	account.GeneralizationRef = "System.User"
	dms := map[model.ID]*domainmodel.DomainModel{
		sales.ID:  mkDomainModel(sales.ID, mkEntity(sales.ID, "Order")),
		admin.ID:  mkDomainModel(admin.ID, account),
		system.ID: mkDomainModel(system.ID, mkEntity(system.ID, "User")),
	}
	mods := map[string]*model.Module{"Sales": sales, "Administration": admin, "System": system}
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		GetModuleByNameFunc: func(n string) (*model.Module, error) { return mods[n], nil },
		GetDomainModelFunc:  func(id model.ID) (*domainmodel.DomainModel, error) { return dms[id], nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	for _, tc := range []struct{ in, want string }{
		// Inherited attribute, qualified with the generalization: the attribute.
		{"[System.User.Name = 'x']", "[Name = 'x']"},
		// The entity itself, as before.
		{"[Administration.Account.Email = 'x']", "[Email = 'x']"},
		// Qualified with an entity that is neither: left as written (CE0161),
		// never the literal 'Total'.
		{"[Sales.Order.Total = 'x']", "[Sales.Order.Total = 'x']"},
		// Not an entity: an enumeration value, stored as a string literal.
		{"[Kind = Sales.Kind.Gold]", "[Kind = 'Gold']"},
	} {
		if got := accessRuleXPathConstraint(ctx, tc.in, "Administration.Account"); got != tc.want {
			t.Errorf("access rule on Administration.Account, %s\n got  %q\n want %q", tc.in, got, tc.want)
		}
	}
}

// Workflow targeting has no project in hand when it is built, so it cannot tell
// an enumeration value from an attribute of another entity (the configured
// workflow user entity may be a specialization, e.g. Administration.Account).
// It resolves the names it can place and leaves every other three-part name as
// written — as it always stored them — rather than guessing a string literal.
func TestWorkflowTargetingXPath_LeavesUnplacedNamesAsWritten(t *testing.T) {
	task := buildUserTask(&ast.WorkflowUserTaskNode{Name: "t",
		Targeting: ast.WorkflowTargetingNode{Kind: "xpath", XPath: "[Administration.Account.IsLocalUser = true]"}})
	xp, ok := task.UserSource.(*workflows.XPathBasedUserSource)
	if !ok || xp.XPath != "[Administration.Account.IsLocalUser = true]" {
		t.Errorf("user targeting = %#v, want the constraint as written", task.UserSource)
	}
}
