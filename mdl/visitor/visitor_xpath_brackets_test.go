// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): XPath is always written in [ ], never in a string. The
// entity grant takes the word order of every other grant, and workflow
// targeting takes its XPath bracketed. The old spellings are deprecated
// aliases that build the same statement.

func grantStmt(t *testing.T, src string) (*ast.GrantEntityAccessStmt, *ast.Program) {
	t.Helper()
	prog := mustBuild(t, src)
	if len(prog.Statements) != 1 {
		t.Fatalf("%q: %d statements, want 1", src, len(prog.Statements))
	}
	g, ok := prog.Statements[0].(*ast.GrantEntityAccessStmt)
	if !ok {
		t.Fatalf("%q: built %T, want *ast.GrantEntityAccessStmt", src, prog.Statements[0])
	}
	return g, prog
}

func TestGrantEntity_CanonicalForm(t *testing.T) {
	g, prog := grantStmt(t, `grant read *, write (Email, "Status"), create on entity Shop.Order to Shop.User, Shop.Admin where [Status = 'Open'];`)
	want := &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "Shop", Name: "Order"},
		Roles:  []ast.QualifiedName{{Module: "Shop", Name: "User"}, {Module: "Shop", Name: "Admin"}},
		Rights: []ast.EntityAccessRight{
			{Type: ast.EntityAccessReadAll},
			{Type: ast.EntityAccessWriteMembers, Members: []string{"Email", "Status"}},
			{Type: ast.EntityAccessCreate},
		},
		XPathConstraint: "[Status = 'Open']",
	}
	if !reflect.DeepEqual(g, want) {
		t.Errorf("built %#v\nwant  %#v", g, want)
	}
	if got := deprecationCodes(prog); len(got) != 0 {
		t.Errorf("canonical grant recorded %v, want none", got)
	}
}

// The reversed word order and the quoted XPath build exactly what the canonical
// form builds, and record the alias.
func TestGrantEntity_ReversedFormIsAnAlias(t *testing.T) {
	old, prog := grantStmt(t, `grant Shop.User, Shop.Admin on Shop.Order (read *, write (Email, "Status"), create) where '[Status = ''Open'']';`)
	canon, _ := grantStmt(t, `grant read *, write (Email, "Status"), create on entity Shop.Order to Shop.User, Shop.Admin where [Status = 'Open'];`)
	if !reflect.DeepEqual(old, canon) {
		t.Errorf("old form built %#v\ncanonical   %#v", old, canon)
	}
	if got := deprecationCodes(prog); !reflect.DeepEqual(got, []string{deprecation.ReversedEntityGrant}) {
		t.Errorf("recorded %v, want [%s]", got, deprecation.ReversedEntityGrant)
	}
}

// Mendix stores sibling predicate groups concatenated; every group is kept.
func TestGrantEntity_SeveralPredicateGroups(t *testing.T) {
	g, _ := grantStmt(t, `grant read * on entity M.E to M.R where [a = 1][System.owner = '[%CurrentUser%]'];`)
	if g.XPathConstraint != "[a = 1][System.owner = '[%CurrentUser%]']" {
		t.Errorf("XPathConstraint = %q", g.XPathConstraint)
	}
}

// Without a where clause, and in upper case.
func TestGrantEntity_NoWhereUpperCase(t *testing.T) {
	g, _ := grantStmt(t, `GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY M.E TO M.R;`)
	if g.XPathConstraint != "" || len(g.Rights) != 4 || len(g.Roles) != 1 {
		t.Errorf("built %#v", g)
	}
}

func userTaskTargeting(t *testing.T, clause string) (ast.WorkflowTargetingNode, *ast.Program) {
	t.Helper()
	src := "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n  user task Review 'Review'\n    " +
		clause + "\n    outcomes 'Done' { };\nend workflow;"
	prog := mustBuild(t, src)
	wf, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("built %T", prog.Statements[0])
	}
	ut, ok := wf.Activities[0].(*ast.WorkflowUserTaskNode)
	if !ok {
		t.Fatalf("first activity is %T", wf.Activities[0])
	}
	return ut.Targeting, prog
}

func TestWorkflowTargeting_BracketedXPath(t *testing.T) {
	for _, tc := range []struct{ clause, kind string }{
		{`targeting users xpath [System.UserRoles = '[%UserRole_Banker%]']`, "xpath"},
		{`targeting xpath [System.UserRoles = '[%UserRole_Banker%]']`, "xpath"},
		{`targeting groups xpath [System.UserRoles = '[%UserRole_Banker%]']`, "group_xpath"},
	} {
		got, prog := userTaskTargeting(t, tc.clause)
		if got.Kind != tc.kind || got.XPath != `[System.UserRoles = '[%UserRole_Banker%]']` {
			t.Errorf("%s: targeting = %q %q", tc.clause, got.Kind, got.XPath)
		}
		if codes := deprecationCodes(prog); len(codes) != 0 {
			t.Errorf("%s: recorded %v, want none", tc.clause, codes)
		}
	}
}

func TestWorkflowTargeting_QuotedXPathIsAnAlias(t *testing.T) {
	got, prog := userTaskTargeting(t, `targeting users xpath '[System.UserRoles = ''[%UserRole_Banker%]'']'`)
	if got.Kind != "xpath" || got.XPath != `[System.UserRoles = '[%UserRole_Banker%]']` {
		t.Errorf("targeting = %q %q", got.Kind, got.XPath)
	}
	if codes := deprecationCodes(prog); !reflect.DeepEqual(codes, []string{deprecation.QuotedTargetingXPath}) {
		t.Errorf("recorded %v, want [%s]", codes, deprecation.QuotedTargetingXPath)
	}
}

func TestAlterWorkflowTargeting_BracketedXPath(t *testing.T) {
	for _, tc := range []struct {
		src   string
		codes []string
	}{
		{"alter workflow M.WF {\n  set (Targeting: xpath [Role = 'Manager']) on 'Review'; };", nil},
		{"alter workflow M.WF {\n  set (Targeting: xpath '[Role = ''Manager'']') on 'Review'; };", []string{deprecation.QuotedTargetingXPath}},
		// The old action form records its own alias as well (ako/mxcli#712).
		{"alter workflow M.WF\n  set activity 'Review' targeting xpath [Role = 'Manager'];",
			[]string{deprecation.AlterWorkflowSetActivity}},
		{"alter workflow M.WF\n  set activity 'Review' targeting xpath '[Role = ''Manager'']';",
			[]string{deprecation.QuotedTargetingXPath, deprecation.AlterWorkflowSetActivity}},
	} {
		prog := mustBuild(t, tc.src)
		alt, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
		if !ok || len(alt.Operations) != 1 {
			t.Fatalf("%q built %#v", tc.src, prog.Statements[0])
		}
		op, ok := alt.Operations[0].(*ast.SetActivityPropertyOp)
		if !ok || op.Property != "targeting_xpath" || op.Value != "[Role = 'Manager']" {
			t.Errorf("%q: op = %#v", tc.src, alt.Operations[0])
		}
		if got := deprecationCodes(prog); !reflect.DeepEqual(got, tc.codes) {
			t.Errorf("%q: recorded %v, want %v", tc.src, got, tc.codes)
		}
	}
}

// The bracketed XPath is lifted from the source text, which carries whatever
// the lexer sent to a hidden channel. An MDL comment is not XPath: stored, it
// fails the build with CE0161 "Error(s) in XPath constraint" (measured with
// mx check on 11.13 under production security). Every other bracketed XPath
// (retrieve, navigation) strips them.
func TestGrantEntity_CommentsAreNotStored(t *testing.T) {
	for _, src := range []string{
		"grant read * on entity M.E to M.R where [A = 1] /* why */ [B = 2];",
		"grant read * on entity M.E to M.R where [A = 1 -- why\n][B = 2];",
	} {
		g, _ := grantStmt(t, src)
		if strings.Contains(g.XPathConstraint, "why") || !strings.HasPrefix(g.XPathConstraint, "[A = 1") ||
			!strings.HasSuffix(g.XPathConstraint, "[B = 2]") {
			t.Errorf("%q: XPathConstraint = %q, want both groups and no comment", src, g.XPathConstraint)
		}
	}
	got, _ := userTaskTargeting(t, "targeting xpath [Name = 'x'] /* why */")
	if got.XPath != "[Name = 'x']" {
		t.Errorf("targeting XPath = %q, want the comment dropped", got.XPath)
	}
}

// A bare [%Token%] used as a value must be quoted or Studio Pro rejects the
// constraint with CE0161 (#641; measured on an entity access rule, 11.13,
// production security). Every other bracketed XPath quotes it on the way in.
func TestBracketedXPath_BareTokenIsQuoted(t *testing.T) {
	g, _ := grantStmt(t, "grant read * on entity M.E to M.R where [D < [%CurrentDateTime%]];")
	if g.XPathConstraint != "[D < '[%CurrentDateTime%]']" {
		t.Errorf("grant XPathConstraint = %q", g.XPathConstraint)
	}
	got, _ := userTaskTargeting(t, "targeting xpath [System.UserRoles = [%UserRole_Banker%]]")
	if got.XPath != "[System.UserRoles = '[%UserRole_Banker%]']" {
		t.Errorf("targeting XPath = %q", got.XPath)
	}
}

// IsBracketedXPath answers "would `where <s>` store s": a value the bracketed
// form would change on the way in (a comment, a bare token) is not one, so
// describe falls back to the quoted form and fmt --upgrade reports it.
func TestIsBracketedXPath_OnlyWhatStoresVerbatim(t *testing.T) {
	for s, want := range map[string]bool{
		"[a = 1][b = '[%CurrentUser%]']": true,
		"[a = 1] /* c */ [b = 2]":        false,
		"[D < [%CurrentDateTime%]]":      false,
		"[Name = 'a -- b']":              true,
	} {
		if got := IsBracketedXPath(s); got != want {
			t.Errorf("IsBracketedXPath(%q) = %v, want %v", s, got, want)
		}
	}
}
