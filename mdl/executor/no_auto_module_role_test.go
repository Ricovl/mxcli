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

// mendixlabs/mxcli#1474: a new microflow, nanoflow or page in a module with no
// module roles used to create <Module>.User and grant the document to it — a
// security role nobody asked for, opened to every user role later mapped to it.
// A role is now always the author's choice: a new document starts with no
// allowed roles whatever its module holds, and nothing is created.

type noAutoRoleProbe struct {
	added      []string
	microflows []*microflows.Microflow
	nanoflows  []*microflows.Nanoflow
}

func noAutoRoleCtx(t *testing.T, roles []*security.ModuleRole) (*ExecContext, *noAutoRoleProbe, func() string) {
	t.Helper()
	p := &noAutoRoleProbe{}
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
			p.added = append(p.added, name)
			ms.ModuleRoles = append(ms.ModuleRoles, &security.ModuleRole{Name: name, Description: desc})
			return nil
		},
		CreateMicroflowFunc: func(mf *microflows.Microflow) error { p.microflows = append(p.microflows, mf); return nil },
		CreateNanoflowFunc:  func(nf *microflows.Nanoflow) error { p.nanoflows = append(p.nanoflows, nf); return nil },
		ListNanoflowsFunc:   func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, p, buf.String
}

func assertNoAutoRole(t *testing.T, ctx *ExecContext, p *noAutoRoleProbe, out func() string) {
	t.Helper()
	if err := execCreateMicroflow(ctx, &ast.CreateMicroflowStmt{Name: ast.QualifiedName{Module: "Shop", Name: "MF_New"}}); err != nil {
		t.Fatalf("exec microflow: %v", err)
	}
	if err := execCreateNanoflow(ctx, &ast.CreateNanoflowStmt{Name: ast.QualifiedName{Module: "Shop", Name: "NF_New"}}); err != nil {
		t.Fatalf("exec nanoflow: %v", err)
	}
	if len(p.added) != 0 {
		t.Errorf("creating a document created module role(s) %v", p.added)
	}
	if len(p.microflows) != 1 || len(p.nanoflows) != 1 {
		t.Fatalf("documents not written: %d microflow(s), %d nanoflow(s)\n%s", len(p.microflows), len(p.nanoflows), out())
	}
	if r := p.microflows[0].AllowedModuleRoles; len(r) != 0 {
		t.Errorf("new microflow granted to %v, want no allowed roles", r)
	}
	if r := p.nanoflows[0].AllowedModuleRoles; len(r) != 0 {
		t.Errorf("new nanoflow granted to %v, want no allowed roles", r)
	}
	if strings.Contains(out(), "auto-created role") {
		t.Errorf("exec still reports an auto-created role grant:\n%s", out())
	}
}

func TestCreateFlow_ModuleWithoutRoles_CreatesNoRole(t *testing.T) {
	ctx, p, out := noAutoRoleCtx(t, nil)
	assertNoAutoRole(t, ctx, p, out)
}

// A module still holding the role an earlier mxcli auto-created: that role was
// not the author's choice either, so a new document is not granted to it. The
// role itself is left alone — removing a stored role is not this fix's call.
func TestCreateFlow_ModuleWithLegacyAutoRole_GrantsNothing(t *testing.T) {
	ctx, p, out := noAutoRoleCtx(t, []*security.ModuleRole{{Name: "User", Description: autoDocumentRoleDescription}})
	assertNoAutoRole(t, ctx, p, out)
}

// Control: a module that manages its own roles never got a default grant.
func TestCreateFlow_ModuleWithOwnRoles_GrantsNothing(t *testing.T) {
	ctx, p, out := noAutoRoleCtx(t, []*security.ModuleRole{{Name: "Admin"}})
	assertNoAutoRole(t, ctx, p, out)
}
