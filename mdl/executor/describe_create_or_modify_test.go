// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// DESCRIBE describes a document that exists, so its output has to re-execute
// against the project it came from. A plain `create` fails there with "already
// exists" — the round trip dies at the first statement instead of reporting
// what changed (ako/mxcli#705 item 5). #731 moved the rest of the describers
// over (JavaScript action, user role, navigation, layout, REST and OData
// clients, OData service, external entity, database connection, data
// transformer, agent-editor documents, workflow, validation rule, module); their
// mock tests assert the spelling, and the three below also parse it.
//
// Each test parses the output rather than grepping it: what matters is the
// statement the parser builds, not how the verb is spelled.

func TestDescribeAssociation_EmitsCreateOrModify(t *testing.T) {
	ctx, _ := assocFixture(t)
	var buf bytes.Buffer
	ctx.Output = &buf
	assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))

	stmt := firstStatement[*ast.CreateAssociationStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe association emitted a plain create, which fails on the association it describes:\n%s", buf.String())
	}
}

func TestDescribeJavaAction_EmitsCreateOrModify(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(qn string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				BaseElement: model.BaseElement{ID: nextID("ja")},
				ContainerID: mod.ID,
				Name:        "DoSomething",
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "DoSomething"}))

	stmt := firstStatement[*ast.CreateJavaActionStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe java action emitted a plain create, which fails on the action it describes:\n%s", buf.String())
	}
}

func TestDescribeModuleRole_EmitsCreateOrModify(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{{
				ContainerID: mod.ID,
				ModuleRoles: []*security.ModuleRole{{Name: "Admin", Description: "Full access"}},
			}}, nil
		},
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	assertNoError(t, describeModuleRole(ctx, ast.QualifiedName{Module: "MyModule", Name: "Admin"}))

	stmt := firstStatement[*ast.CreateModuleRoleStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe module role emitted a plain create, which fails on the role it describes:\n%s", buf.String())
	}
}

// describe module prints its roles in the same shape as describe module role,
// so the two must change together.
func TestDescribeModule_RolesEmitCreateOrModify(t *testing.T) {
	mod := mkModule("Administration")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(id model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Administration", false))

	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("describe module emitted MDL the parser rejects: %v\n%s", errs, buf.String())
	}
	found := false
	for _, s := range prog.Statements {
		if r, ok := s.(*ast.CreateModuleRoleStmt); ok {
			found = true
			if !r.CreateOrModify {
				t.Errorf("describe module emitted a plain create module role:\n%s", buf.String())
			}
		}
	}
	if !found {
		t.Fatalf("no module role statement in:\n%s", buf.String())
	}
}

func TestDescribeJavaScriptAction_EmitsCreateOrModify(t *testing.T) {
	mod := mkModule("WebMod")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{
				BaseElement: model.BaseElement{ID: nextID("jsa")},
				ContainerID: mod.ID,
				Name:        "ShowAlert",
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "WebMod", Name: "ShowAlert"}))

	stmt := firstStatement[*ast.CreateJavaScriptActionStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe javascript action emitted a plain create, which fails on the action it describes:\n%s", buf.String())
	}
}

func TestDescribeUserRole_EmitsCreateOrModify(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{UserRoles: []*security.UserRole{
				{Name: "Administrator", ModuleRoles: []string{"MyModule.Admin"}},
			}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeUserRole(ctx, ast.QualifiedName{Name: "Administrator"}))

	stmt := firstStatement[*ast.CreateUserRoleStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe user role emitted a plain create, which fails on the role it describes:\n%s", buf.String())
	}
}

// Navigation printed `create or replace`, the deprecated spelling (MDL-DEPR001):
// describe output must be canonical, so it warns on nothing.
func TestDescribeNavigation_EmitsCreateOrModify(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
				Name: "Responsive", Kind: "Responsive", HomePage: &types.NavHomePage{Page: "MyModule.Home"},
			}}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeNavigation(ctx, ast.QualifiedName{Name: "Responsive"}))

	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("describe navigation emitted MDL the parser rejects: %v\n%s", errs, buf.String())
	}
	if len(prog.Deprecations) != 0 {
		t.Errorf("describe navigation emitted a deprecated spelling %+v:\n%s", prog.Deprecations, buf.String())
	}
	stmt := firstStatement[*ast.AlterNavigationStmt](t, buf.String())
	if !stmt.CreateOrModify {
		t.Errorf("describe navigation is not an idempotent create:\n%s", buf.String())
	}
}

func firstStatement[T ast.Statement](t *testing.T, mdl string) T {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n--- output ---\n%s", errs, mdl)
	}
	for _, s := range prog.Statements {
		if v, ok := s.(T); ok {
			return v
		}
	}
	var zero T
	t.Fatalf("no %T in DESCRIBE output:\n%s", zero, mdl)
	return zero
}
