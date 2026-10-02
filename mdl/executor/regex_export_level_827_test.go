// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#827: a regular expression's export level is Projects$ExportLevel,
// Hidden | API — what Studio Pro stores and describe prints. The validator
// allowed Hidden | Public, so re-running describe's `ExportLevel: API` was
// refused, and `Public` was written verbatim, a value the metamodel does not
// have. API is accepted; Public is its deprecated spelling and stores API.
func TestRegularExpressionExportLevel_DescribeRoundTripsAndPublicStoresAPI(t *testing.T) {
	for _, c := range []struct{ name, stored, want string }{
		{"API as Studio Pro stores it", "API", "API"},
		{"Public as an earlier mxcli stored it", "Public", "API"},
	} {
		t.Run(c.name, func(t *testing.T) {
			mod := mkModule("Val")
			re := mkRegex(mod.ID, "R", `^a$`)
			re.ExportLevel = c.stored
			h := mkHierarchy(mod)
			withContainer(h, re.ContainerID, mod.ID)
			var written *model.RegularExpression
			mb := &mock.MockBackend{
				IsConnectedFunc:            func() bool { return true },
				ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) { return []*model.RegularExpression{re}, nil },
				UpdateRegularExpressionFunc: func(r *model.RegularExpression) error {
					written = r
					return nil
				},
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, execDescribeRegularExpression(ctx, &ast.DescribeRegularExpressionStmt{
				Name: ast.QualifiedName{Module: "Val", Name: "R"},
			}))
			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("describe output does not parse: %v\n%s", errs, buf.String())
			}
			if len(prog.Deprecations) != 0 {
				t.Errorf("describe output uses a deprecated spelling: %v\n%s", prog.Deprecations, buf.String())
			}
			stmt := prog.Statements[0].(*ast.CreateRegularExpressionStmt)
			if err := execCreateRegularExpression(ctx, stmt); err != nil {
				t.Fatalf("re-running describe's output: %v\n%s", err, buf.String())
			}
			if written == nil || written.ExportLevel != c.want {
				t.Errorf("written export level = %v, want %q", written, c.want)
			}
		})
	}
}

// `ExportLevel: Public` keeps working without the header, warns MDL-DEPR161,
// and builds what `ExportLevel: API` builds; an unknown value is still refused.
func TestRegularExpressionExportLevel_PublicIsADeprecatedAlias(t *testing.T) {
	prog, errs := visitor.Build(`create regular expression Val.R ( Expression: '^a$', ExportLevel: Public );`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	stmt := prog.Statements[0].(*ast.CreateRegularExpressionStmt)
	if stmt.ExportLevel != "API" {
		t.Errorf("ExportLevel = %q, want API", stmt.ExportLevel)
	}
	if len(prog.Deprecations) != 1 || prog.Deprecations[0].Code != deprecation.RegexExportLevelPublic {
		t.Errorf("deprecations = %v, want one %s", prog.Deprecations, deprecation.RegexExportLevelPublic)
	}
	if err := validateRegularExpressionStmt(stmt); err != nil {
		t.Errorf("validate: %v", err)
	}
	// Control: the validator still refuses a value outside the metamodel.
	if err := validateRegularExpressionStmt(&ast.CreateRegularExpressionStmt{
		Name: ast.QualifiedName{Module: "Val", Name: "R"}, Expression: "^a$", ExportLevel: "Protected",
	}); err == nil {
		t.Error("ExportLevel: Protected was accepted")
	}
}
