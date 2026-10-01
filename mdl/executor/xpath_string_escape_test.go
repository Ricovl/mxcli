// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A stored XPath is spelled the Mendix way: a doubled apostrophe is the only
// escape and a backslash is itself. Every describer writes it so that the
// statement it belongs to, read in the describe language, stores it again:
// as it stands under mdl 1, and with each backslash in a string doubled under
// mdl 0, which reads a backslash as an escape (ako/mxcli#825).
var storedXPaths = []string{
	`[Name = 'C:\temp']`,
	`[Name = 'C:\' or Name = 'it''s']`,
	`[Name = 'a\nb'][Name != '\\']`,
}

type xpathDescriber struct {
	name     string
	describe func(ctx *ExecContext, stored string) string // the statement describe writes
	read     func(t *testing.T, prog *ast.Program) string // what that statement stores
}

var xpathDescribers = []xpathDescriber{
	{"access rule",
		func(ctx *ExecContext, x string) string {
			return entityGrantMDL(ctx, "read *", "M.E", []string{"M.R"}, x)
		},
		func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.GrantEntityAccessStmt).XPathConstraint
		}},
	{"workflow targeting",
		func(ctx *ExecContext, x string) string {
			return "alter workflow M.WF {\n  set (Targeting: xpath " + targetingXPathMDL(ctx, x) + ") on 'Review'; };"
		},
		func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.AlterWorkflowStmt).Operations[0].(*ast.SetActivityPropertyOp).Value
		}},
	{"navigation sync",
		func(ctx *ExecContext, x string) string {
			return "create or replace navigation TabletOffline sync ( sync M.E " + syncModeMDL(ctx, "Constrained", x) + "; );"
		},
		func(t *testing.T, prog *ast.Program) string {
			return prog.Statements[0].(*ast.AlterNavigationStmt).SyncEntries[0].Constraint
		}},
	{"page datasource",
		func(ctx *ExecContext, x string) string {
			return "create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {\n  gallery g (datasource: database from M.E where " +
				xpathConstraintClause(ctx, x) + ") {\n    dynamictext t (content: 'x')\n  }\n};"
		},
		func(t *testing.T, prog *ast.Program) string {
			page := prog.Statements[0].(*ast.CreatePageStmtV3)
			for _, w := range page.Widgets {
				if ds, ok := w.Properties["DataSource"].(*ast.DataSourceV3); ok {
					return ds.Where
				}
			}
			t.Fatal("no datasource")
			return ""
		}},
}

func TestDescribedXPathStoresTheStoredConstraint(t *testing.T) {
	for _, lang := range []struct {
		name   string
		ctx    *ExecContext
		header string
	}{
		{"mdl 0", mdl0Ctx(), ""},
		{"mdl 1", &ExecContext{LanguageVersion: langver.V1}, "mdl 1;\n"},
		{"default (mdl 1)", nil, "mdl 1;\n"},
	} {
		for _, d := range xpathDescribers {
			for _, stored := range storedXPaths {
				// A navigation sync constraint takes one predicate group.
				if d.name == "navigation sync" && strings.Contains(stored, "][") {
					continue
				}
				src := lang.header + d.describe(lang.ctx, stored)
				prog, errs := visitor.Build(src)
				if len(errs) > 0 {
					t.Errorf("%s, %s: %s does not parse: %v", lang.name, d.name, src, errs[0])
					continue
				}
				if got, want := d.read(t, prog), stored; got != want {
					t.Errorf("%s, %s: %s\nstores %q\nwant   %q", lang.name, d.name, src, got, want)
				}
			}
		}
	}
}

// Control: under mdl 0 the stored text as it stands reads as another value,
// so describe cannot write it unchanged.
func TestStoredXPathAsWrittenIsAnotherValueUnderMDL0(t *testing.T) {
	prog, errs := visitor.Build("grant read * on entity M.E to M.R where " + storedXPaths[0] + ";")
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if got := prog.Statements[0].(*ast.GrantEntityAccessStmt).XPathConstraint; got == storedXPaths[0] {
		t.Errorf("mdl 0 read %q as itself; the comparison above cannot fail", got)
	}
}

// The dynamic query of a database query action is a Mendix expression, and a
// retrieve's limit and offset are too: described the way describeExpr spells
// an expression, and stored as the value under mdl 0.
func TestDynamicQueryDescribesPerLanguage(t *testing.T) {
	stored := `'SELECT * FROM t WHERE p = ''C:\temp'''`
	a := &microflows.ExecuteDatabaseQueryAction{Query: "M.C.Q", DynamicQuery: stored}
	for _, lang := range []struct {
		ctx    *ExecContext
		header string
	}{{mdl0Ctx(), ""}, {&ExecContext{LanguageVersion: langver.V1}, "mdl 1;\n"}, {nil, "mdl 1;\n"}} {
		line := formatExecuteDatabaseQueryAction(lang.ctx, a)
		src := lang.header + "create microflow M.F () begin\n  " + line + "\nend;"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s: %v", src, errs[0])
		}
		q := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ExecuteDatabaseQueryStmt)
		if got := dynamicQueryExpression(q); got != stored {
			t.Errorf("%s\nstores %q\nwant   %q", src, got, stored)
		}
	}
}
