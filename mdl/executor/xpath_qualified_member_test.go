// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
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
		if got, want := accessRuleXPathConstraint("[M.Emp.Name = 'y']", "M.Emp"), "[Name = 'y']"; got != want {
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
