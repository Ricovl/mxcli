// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// A new document in a module with no module roles is granted to an
// auto-created <Module>.User role (defaultDocumentAccessRoles). That grant was
// silent, and GRANT is additive — so a stub page later granted to an admin role
// still opened for every role mapped to <Module>.User, and the people who hit it
// had no way to know the first grant existed. exec now says so.

const autoRoleNoteText = "access: granted to auto-created role Shop.User (the module has no other roles"

func autoRoleCtx(t *testing.T, roles []*security.ModuleRole) (*ExecContext, func() string) {
	t.Helper()
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-shop"}, Name: "Shop"}
	ms := &security.ModuleSecurity{BaseElement: model.BaseElement{ID: "ms-shop"}, ContainerID: mod.ID, ModuleRoles: roles}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Shop" {
				return mod, nil
			}
			return nil, nil
		},
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) { return ms, nil },
		AddModuleRoleFunc: func(_ model.ID, name, desc string) error {
			ms.ModuleRoles = append(ms.ModuleRoles, &security.ModuleRole{Name: name, Description: desc})
			return nil
		},
		CreateMicroflowFunc: func(*microflows.Microflow) error { return nil },
		CreateNanoflowFunc:  func(*microflows.Nanoflow) error { return nil },
		ListNanoflowsFunc:   func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, buf.String
}

func TestCreateMicroflow_SaysWhenAutoRoleGranted(t *testing.T) {
	ctx, out := autoRoleCtx(t, nil)
	s := &ast.CreateMicroflowStmt{Name: ast.QualifiedName{Module: "Shop", Name: "MF_New"}}
	if err := execCreateMicroflow(ctx, s); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out(), autoRoleNoteText) {
		t.Fatalf("exec output does not mention the auto-created role grant:\n%s", out())
	}

	// A second document in the same module is granted to the same role.
	s2 := &ast.CreateNanoflowStmt{Name: ast.QualifiedName{Module: "Shop", Name: "NF_New"}}
	before := len(out())
	if err := execCreateNanoflow(ctx, s2); err != nil {
		t.Fatalf("exec nanoflow: %v", err)
	}
	if !strings.Contains(out()[before:], autoRoleNoteText) {
		t.Fatalf("nanoflow output does not mention the grant:\n%s", out()[before:])
	}
}

// Control: a module that manages its own roles gets no default grant, and
// nothing is said about one.
func TestCreateMicroflow_NoAutoRoleNoteWhenModuleHasRoles(t *testing.T) {
	ctx, out := autoRoleCtx(t, []*security.ModuleRole{{Name: "Admin"}})
	s := &ast.CreateMicroflowStmt{Name: ast.QualifiedName{Module: "Shop", Name: "MF_New"}}
	if err := execCreateMicroflow(ctx, s); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if strings.Contains(out(), "auto-created role") {
		t.Fatalf("note printed for a module with its own roles:\n%s", out())
	}
	if !strings.Contains(out(), "Created microflow: Shop.MF_New") {
		t.Fatalf("microflow was not created:\n%s", out())
	}
}
