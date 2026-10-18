// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// "GRANT EXECUTE ON NANOFLOW Module.NF TO Module.Role is stored, but
// permissions() only returns microflow EXECUTE rows" (ako/mxcli#1434). The
// granted microflow is the control: it had a row before the fix too.
func TestPermissionsIncludeNanoflowExecute(t *testing.T) {
	const mod = model.ID("mod-t")
	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "T"}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListRulesFunc:        func() ([]*microflows.Rule, error) { return nil, nil },
		GetNavigationFunc:    func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{{
				BaseElement: model.BaseElement{ID: "mf-1"}, ContainerID: mod, Name: "ACT_Save",
				AllowedModuleRoles: []model.ID{"T.User"},
			}}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{
				{BaseElement: model.BaseElement{ID: "nf-1"}, ContainerID: mod, Name: "NF_Open",
					AllowedModuleRoles: []model.ID{"T.User", "T.Admin"}},
				// Not granted to anyone: no row.
				{BaseElement: model.BaseElement{ID: "nf-2"}, ContainerID: mod, Name: "NF_Private"},
			}, nil
		},
	}
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}

	var got []string
	for _, row := range queryRows(t, cat, `SELECT ElementType, ElementName, AccessType, ModuleRoleName, ModuleName
		FROM permissions WHERE AccessType = 'EXECUTE'`) {
		got = append(got, row[0].(string)+" "+row[1].(string)+" "+row[2].(string)+" "+row[3].(string)+" "+row[4].(string))
	}
	sort.Strings(got)
	want := []string{
		"MICROFLOW T.ACT_Save EXECUTE T.User T",
		"NANOFLOW T.NF_Open EXECUTE T.Admin T",
		"NANOFLOW T.NF_Open EXECUTE T.User T",
	}
	if len(got) != len(want) {
		t.Fatalf("EXECUTE rows:\n%v\nwant:\n%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EXECUTE rows:\n%v\nwant:\n%v", got, want)
		}
	}
}
