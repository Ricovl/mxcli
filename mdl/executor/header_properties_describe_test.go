// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// buildCanonical parses describe output and requires it to use no deprecated
// spelling: describe is what people and agents copy MDL from.
func buildCanonical(t *testing.T, out string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(out)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, out)
	}
	for _, d := range prog.Deprecations {
		t.Errorf("describe output uses the deprecated %s at line %d:\n%s", d.Code, d.Line, out)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("describe output built %d statements, want 1:\n%s", len(prog.Statements), out)
	}
	return prog
}

// Phase 3.6 (ako/mxcli#755): describe writes a constant's header as its
// property list, and it reads back to the same values.
func TestDescribeConstantWritesThePropertyList(t *testing.T) {
	for _, exposed := range []bool{false, true} {
		buf := &bytes.Buffer{}
		c := &model.Constant{
			Name:            "Url",
			Type:            model.ConstantDataType{Kind: "String"},
			DefaultValue:    "https://x",
			ExposedToClient: exposed,
		}
		if err := New(buf).outputConstantMDL(c, "M"); err != nil {
			t.Fatalf("outputConstantMDL: %v", err)
		}
		got := buildCanonical(t, buf.String()).Statements[0].(*ast.CreateConstantStmt)
		if got.DataType.Kind != ast.TypeString || got.DefaultValue != "https://x" || got.ExposedToClient != exposed || !got.CreateOrModify {
			t.Errorf("exposed=%v read back as %+v from:\n%s", exposed, got, buf.String())
		}
	}
}

// Describe writes a demo user's header as its property list. A demo user
// without roles now describes into a statement that parses: the clause form
// required at least one role.
func TestDescribeDemoUserWritesThePropertyList(t *testing.T) {
	for _, du := range []*security.DemoUser{
		{UserName: "demo_admin", Entity: "Administration.Account", UserRoles: []string{"Administrator", "User"}},
		{UserName: "demo_none"},
	} {
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
				return &security.ProjectSecurity{EnableDemoUsers: true, DemoUsers: []*security.DemoUser{du}}, nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb))
		assertNoError(t, describeDemoUser(ctx, du.UserName))
		got := buildCanonical(t, buf.String()).Statements[0].(*ast.CreateDemoUserStmt)
		if got.UserName != du.UserName || got.Entity != du.Entity || len(got.UserRoles) != len(du.UserRoles) ||
			(len(du.UserRoles) > 0 && !reflect.DeepEqual(got.UserRoles, du.UserRoles)) ||
			got.Password != demoUserPasswordPlaceholder || !got.CreateOrModify {
			t.Errorf("%s read back as %+v from:\n%s", du.UserName, got, buf.String())
		}
	}
}
