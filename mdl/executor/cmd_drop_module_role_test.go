// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// `drop module role if exists` is a no-op for a role that is not there, the
// same as `drop user role if exists` (#731). The control is the same statement
// without IF EXISTS, which still refuses.
func TestDropModuleRole_IfExistsSkipsMissingRole(t *testing.T) {
	mod := mkModule("M")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	name := ast.QualifiedName{Module: "M", Name: "Gone"}

	assertNoError(t, execDropModuleRole(ctx, &ast.DropModuleRoleStmt{Name: name, IfExists: true}))
	assertContainsStr(t, buf.String(), "does not exist, skipping")

	err := execDropModuleRole(ctx, &ast.DropModuleRoleStmt{Name: name})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}
