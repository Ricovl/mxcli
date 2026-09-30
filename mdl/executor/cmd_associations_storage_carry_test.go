// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `describe association` omits the storage clause for table storage, and now
// prints `create or modify` (ako/mxcli#705 item 5). Re-executing that output
// must not decide the storage on the author's behalf: a statement that does not
// state it keeps what is stored. Before, the omitted clause defaulted to column
// and flipped a Studio Pro table-storage association (ako/mxcli#704), which
// changes the database schema — reachable from unchanged describe output as
// soon as it said `create or modify`.
func TestCreateOrModifyAssociation_UnstatedStorageKeepsTheStoredOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stated ast.StorageType
		stored domainmodel.AssociationStorageFormat
		want   domainmodel.AssociationStorageFormat
	}{
		{"unstated keeps table", ast.StorageDefault, domainmodel.StorageFormatTable, domainmodel.StorageFormatTable},
		{"unstated keeps column", ast.StorageDefault, domainmodel.StorageFormatColumn, domainmodel.StorageFormatColumn},
		// Controls: a stated storage is applied.
		{"stated column over table", ast.StorageColumn, domainmodel.StorageFormatTable, domainmodel.StorageFormatColumn},
		{"stated table over column", ast.StorageTable, domainmodel.StorageFormatColumn, domainmodel.StorageFormatTable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			ent1 := mkEntity(mod.ID, "Order")
			ent2 := mkEntity(mod.ID, "Customer")
			assoc := mkAssociation(mod.ID, "Order_Customer", ent1.ID, ent2.ID)
			assoc.StorageFormat = tc.stored
			dm := &domainmodel.DomainModel{
				BaseElement:  model.BaseElement{ID: nextID("dm")},
				ContainerID:  mod.ID,
				Entities:     []*domainmodel.Entity{ent1, ent2},
				Associations: []*domainmodel.Association{assoc},
			}
			h := mkHierarchy(mod)
			withContainer(h, dm.ID, mod.ID)
			withContainer(h, ent1.ContainerID, dm.ID)
			withContainer(h, ent2.ContainerID, dm.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc:             func() bool { return true },
				ListModulesFunc:             func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListDomainModelsFunc:        func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
				GetDomainModelFunc:          func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
				UpdateDomainModelFunc:       func(d *domainmodel.DomainModel) error { return nil },
				ReconcileMemberAccessesFunc: func(dmID model.ID, moduleName string) (int, error) { return 0, nil },
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
			err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
				Name:           ast.QualifiedName{Module: "MyModule", Name: "Order_Customer"},
				Parent:         ast.QualifiedName{Module: "MyModule", Name: "Order"},
				Child:          ast.QualifiedName{Module: "MyModule", Name: "Customer"},
				Type:           ast.AssocReference,
				Storage:        tc.stated,
				CreateOrModify: true,
			})
			assertNoError(t, err)
			if assoc.StorageFormat != tc.want {
				t.Errorf("storage = %q, want %q", assoc.StorageFormat, tc.want)
			}
		})
	}
}

// The cross-module arm of the same rewrite.
func TestCreateOrModifyCrossAssociation_UnstatedStorageKeepsTheStoredOne(t *testing.T) {
	mod1 := mkModule("ModA")
	mod2 := mkModule("ModB")
	ent1 := mkEntity(mod1.ID, "Order")
	ent2 := mkEntity(mod2.ID, "Product")
	ca := &domainmodel.CrossModuleAssociation{
		BaseElement:   model.BaseElement{ID: nextID("ca")},
		ContainerID:   nextID("dm1"),
		Name:          "Order_Product",
		ChildRef:      "ModB.Product",
		StorageFormat: domainmodel.StorageFormatTable,
	}
	dm1 := &domainmodel.DomainModel{
		BaseElement:       model.BaseElement{ID: nextID("dm1")},
		ContainerID:       mod1.ID,
		Entities:          []*domainmodel.Entity{ent1},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{ca},
	}
	dm2 := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm2")},
		ContainerID: mod2.ID,
		Entities:    []*domainmodel.Entity{ent2},
	}
	h := mkHierarchy(mod1, mod2)
	withContainer(h, dm1.ID, mod1.ID)
	withContainer(h, dm2.ID, mod2.ID)
	withContainer(h, ent1.ContainerID, dm1.ID)
	withContainer(h, ent2.ContainerID, dm2.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm1, dm2}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == mod1.ID {
				return dm1, nil
			}
			return dm2, nil
		},
		UpdateDomainModelFunc:       func(d *domainmodel.DomainModel) error { return nil },
		ReconcileMemberAccessesFunc: func(dmID model.ID, moduleName string) (int, error) { return 0, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:           ast.QualifiedName{Module: "ModA", Name: "Order_Product"},
		Parent:         ast.QualifiedName{Module: "ModA", Name: "Order"},
		Child:          ast.QualifiedName{Module: "ModB", Name: "Product"},
		Type:           ast.AssocReference,
		CreateOrModify: true,
	})
	assertNoError(t, err)
	if ca.StorageFormat != domainmodel.StorageFormatTable {
		t.Errorf("storage = %q, want Table", ca.StorageFormat)
	}
}
