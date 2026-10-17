// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#1442: "the catalog has no user roles table with it" — a user
// role's documentation could not be queried from the catalog. The
// undocumented, anonymous role is the control for both columns.
func TestCatalogUserRolesCarryDescription(t *testing.T) {
	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc:        func() ([]*model.Module, error) { return nil, nil },
		ListDomainModelsFunc:   func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListRulesFunc:          func() ([]*microflows.Rule, error) { return nil, nil },
		ListMicroflowsFunc:     func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return nil, nil },
		GetNavigationFunc:      func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				GuestUserRole: "Guest",
				UserRoles: []*security.UserRole{
					{Name: "Planner", Description: "Plans the routes", ModuleRoles: []string{"Ops.Planner", "Ops.User"}},
					{Name: "Guest", ModuleRoles: []string{"Ops.Anonymous"}},
				},
			}, nil
		},
	}
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}

	got := map[string]string{}
	for _, row := range queryRows(t, cat, `SELECT Name, Description, IsAnonymous, ModuleRoleCount FROM user_roles`) {
		got[row[0].(string)] = fmt.Sprintf("%v|%v|%v", row[1], row[2], row[3])
	}
	want := map[string]string{
		"Planner": "Plans the routes|0|2",
		"Guest":   "|1|1",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("user_roles rows = %v, want %v", got, want)
	}
}
