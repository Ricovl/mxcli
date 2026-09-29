// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// xpathEscapeCase is one string literal as a script of one language spells it
// inside an XPath, and the literal the constraint must store: the value the
// literal has in that language, spelled the way Mendix spells it — an
// apostrophe doubled, a backslash and a control character as themselves. The
// rule is the expression rule (ako/mxcli#810, #820): Mendix XPath has no
// backslash escape either (ako/mxcli#825).
type xpathEscapeCase struct {
	name   string
	lit    string // the literal in the script
	stored string // the literal in the stored constraint
}

var xpathEscapeCases = map[string][]xpathEscapeCase{
	"mdl 0": {
		{"backslash", `'C:\\temp'`, `'C:\temp'`},
		{"backslash before another character", `'C:\data'`, `'C:\data'`},
		{"trailing backslash", `'C:\\'`, `'C:\'`},
		{"escaped apostrophe", `'it\'s'`, `'it''s'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"escaped line break", `'a\nb'`, "'a\nb'"},
	},
	"mdl 1": {
		{"backslash", `'C:\temp'`, `'C:\temp'`},
		{"doubled backslash", `'C:\\temp'`, `'C:\\temp'`},
		{"trailing backslash", `'C:\'`, `'C:\'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"line break", "'a\nb'", "'a\nb'"},
		{"backslash n", `'a\nb'`, `'a\nb'`},
	},
}

// xpathKinds are the places a script writes a bracketed XPath. Each writes
// `Name = LIT` into its constraint and reads back what the statement stores
// for it; want is the stored constraint with LIT replaced.
var xpathKinds = []struct {
	name   string
	script string // with LIT for the literal
	want   string // the constraint the statement carries, with LIT
	read   func(t *testing.T, prog *ast.Program) string
}{
	{"retrieve", "create microflow M.F () begin\n  retrieve $l from M.E where [Name = LIT];\nend;",
		"Name = LIT", retrieveWhereSource},
	{"retrieve spanning lines", "create microflow M.F () begin\n  retrieve $l from M.E where [Name = LIT or\n    Name = 'x'];\nend;",
		"Name = LIT or\n    Name = 'x'", retrieveWhereSource},
	{"retrieve two groups", "create microflow M.F () begin\n  retrieve $l from M.E where [Name = LIT][Name != 'x'];\nend;",
		"[Name = LIT][Name != 'x']", retrieveWhereSource},
	{"access rule", "grant read * on entity M.E to M.R where [Name = LIT];",
		"[Name = LIT]", func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.GrantEntityAccessStmt).XPathConstraint
		}},
	{"page datasource", "create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {\n  gallery g (datasource: database from M.E where [Name = LIT]) {\n    dynamictext t (content: 'x')\n  }\n};",
		"[Name = LIT]", func(t *testing.T, prog *ast.Program) string {
			page := prog.Statements[0].(*ast.CreatePageStmtV3)
			return findWidget(page.Widgets, "g").Properties["DataSource"].(*ast.DataSourceV3).Where
		}},
	{"workflow targeting", "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n  user task Review 'Review'\n    targeting users xpath [Name = LIT]\n    outcomes 'Done' { };\nend workflow;",
		"[Name = LIT]", func(t *testing.T, prog *ast.Program) string {
			wf := prog.Statements[0].(*ast.CreateWorkflowStmt)
			return wf.Activities[0].(*ast.WorkflowUserTaskNode).Targeting.XPath
		}},
	{"alter workflow targeting", "alter workflow M.WF {\n  set (Targeting: xpath [Name = LIT]) on 'Review'; };",
		"[Name = LIT]", func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.AlterWorkflowStmt).Operations[0].(*ast.SetActivityPropertyOp).Value
		}},
	{"navigation sync", "create or replace navigation TabletOffline sync ( sync M.E where [Name = LIT]; );",
		"[Name = LIT]", func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.AlterNavigationStmt).SyncEntries[0].Constraint
		}},
}

func retrieveWhereSource(t *testing.T, prog *ast.Program) string {
	t.Helper()
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	r := mf.Body[0].(*ast.RetrieveStmt)
	src, ok := r.Where.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("retrieve where is %T, want the source as written", r.Where)
	}
	return src.Source
}

// A string in any bracketed XPath is read by the string rule of the script's
// language and stores its value in Mendix's spelling, as a string in an
// expression does. Under mdl 0 the access rule, workflow targeting and
// navigation constraints used to be stored as written, so `'C:\\temp'` stored
// two backslashes and `'it\'s'` an escape Mendix does not have, while a retrieve
// and a page datasource in the same script stored the value (#825).
func TestXPathStringsStoreTheirValue(t *testing.T) {
	for _, lang := range []string{"mdl 0", "mdl 1"} {
		header := ""
		if lang == "mdl 1" {
			header = "mdl 1;\n"
		}
		for _, k := range xpathKinds {
			for _, c := range xpathEscapeCases[lang] {
				t.Run(fmt.Sprintf("%s/%s/%s", lang, k.name, c.name), func(t *testing.T) {
					src := header + strings.ReplaceAll(k.script, "LIT", c.lit)
					prog := mustBuild(t, src)
					want := strings.ReplaceAll(k.want, "LIT", c.stored)
					if got := k.read(t, prog); got != want {
						t.Errorf("%s\nstored %q\nwant   %q", src, got, want)
					}
				})
			}
		}
	}
}

// ParseXPathConstraint and IsBracketedXPath read a STORED constraint, which
// is spelled the Mendix way whatever language the script is in. Re-reading it
// with the mdl 0 lexer turned `'C:\temp'` into a tab and ran `'C:\'` into the
// rest of the constraint — the multi-line retrieve of #825, whose layout the
// executor re-derives from this parse on every write.
func TestStoredXPathIsReadInMendixSpelling(t *testing.T) {
	for _, tc := range []struct{ stored, value string }{
		{`[Name = 'C:\temp']`, `C:\temp`},
		{`[Name = 'C:\']`, `C:\`},
		{`[Name = 'it''s']`, `it's`},
		{`[Name = 'a\nb']`, `a\nb`},
		{"[Name = 'a\nb']", "a\nb"},
		{`[Name = 'C:\\']`, `C:\\`},
	} {
		expr, ok := ParseXPathConstraint(tc.stored)
		if !ok {
			t.Errorf("%q does not parse", tc.stored)
			continue
		}
		b, ok := expr.(*ast.BinaryExpr)
		if !ok {
			t.Errorf("%q parsed as %T", tc.stored, expr)
			continue
		}
		lit, ok := b.Right.(*ast.LiteralExpr)
		if !ok || lit.Value != tc.value {
			t.Errorf("%q: value %#v, want %q", tc.stored, b.Right, tc.value)
		}
		if !IsBracketedXPath(tc.stored) {
			t.Errorf("IsBracketedXPath(%q) = false", tc.stored)
		}
	}
}

// The layout a long or multi-line constraint is stored with comes from that
// parse; the strings in it must come out as they went in.
func TestFormatXPathConstraintKeepsStoredStrings(t *testing.T) {
	for _, in := range []string{
		"[Name = 'C:\\temp' or\n  Name = 'x']",
		"[Name = 'C:\\' or\n  Name = 'y']",
		"[Name = 'it''s' or\n  Name = 'a\nb']",
	} {
		got := FormatXPathConstraint(in)
		if FlattenXPathConstraint(got) != FlattenXPathConstraint(in) {
			t.Errorf("FormatXPathConstraint(%q) = %q", in, got)
		}
	}
}

// A database query's dynamic expression and a retrieve's limit and offset are
// Mendix expressions stored as written; a string in them stores its value, as
// in every other expression stored as written (#822, #825).
func TestExpressionsStoredAsWrittenStoreStringValues(t *testing.T) {
	for _, tc := range []struct {
		header, lit, stored string
	}{
		{"", `'C:\\temp'`, `'C:\temp'`},
		{"", `'it\'s'`, `'it''s'`},
		{"mdl 1;\n", `'C:\temp'`, `'C:\temp'`},
		{"mdl 1;\n", `'C:\'`, `'C:\'`},
	} {
		src := tc.header + "create microflow M.F () begin\n" +
			"  $r = execute database query M.C.Q dynamic $S + " + tc.lit + ";\n" +
			"  retrieve $l from M.E limit length(" + tc.lit + ") offset length(" + tc.lit + ");\nend;"
		body := mustBuild(t, src).Statements[0].(*ast.CreateMicroflowStmt).Body
		if got, want := body[0].(*ast.ExecuteDatabaseQueryStmt).DynamicQuery, "$S + "+tc.stored; got != want {
			t.Errorf("%s\ndynamic stores %q, want %q", src, got, want)
		}
		r := body[1].(*ast.RetrieveStmt)
		if want := "length(" + tc.stored + ")"; strings.TrimSpace(r.Limit) != want || strings.TrimSpace(r.Offset) != want {
			t.Errorf("%s\nlimit %q offset %q, want %q", src, r.Limit, r.Offset, want)
		}
	}
}
