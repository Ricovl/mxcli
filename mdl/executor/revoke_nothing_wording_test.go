// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// A revoke whose right is already gone is the idempotent re-run of a script, and
// said "No access rules found matching …" — which reads as a failed lookup. It
// is the revoke's "already in the state you asked for", worded like the grant's.
func TestRevokeWithNothingToRevokeSaysUnchanged(t *testing.T) {
	mod := mkModule("MyModule")
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Customer", Persistable: true}},
	}
	ms := &security.ModuleSecurity{ContainerID: mod.ID, ModuleRoles: []*security.ModuleRole{{Name: "User"}}}
	mb := &mock.MockBackend{
		IsConnectedFunc:            func() bool { return true },
		ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:         func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleSecurityFunc:      func(model.ID) (*security.ModuleSecurity, error) { return ms, nil },
		ListModuleSecurityFunc:     func() ([]*security.ModuleSecurity, error) { return []*security.ModuleSecurity{ms}, nil },
		RemoveEntityAccessRuleFunc: func(model.ID, string, []string) (int, error) { return 0, nil },
	}
	ctx, out := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	assertNoError(t, execRevokeEntityAccess(ctx, &ast.RevokeEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "MyModule", Name: "Customer"},
		Roles:  []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
	}))
	got := out.String()
	if strings.Contains(got, "No access rules found") {
		t.Errorf("reads as a failed lookup: %q", got)
	}
	if !strings.Contains(got, "Unchanged entity access: MyModule.Customer (MyModule.User)") {
		t.Errorf("output %q, want the Unchanged form", got)
	}
}
