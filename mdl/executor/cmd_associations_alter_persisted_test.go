// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#792: ALTER ASSOCIATION on a cross-module association printed
// "Altered association" while the backend wrote nothing. The backend is fixed
// separately (UpdateDomainModel now patches CrossAssociations); this is the
// executor's half — a statement that reports success must have written. After
// the write, execAlterAssociation reads the association back and refuses to
// report success if the altered property is not what it asked for.
//
// The mock plays a store: GetDomainModel hands out a COPY of what is stored, and
// UpdateDomainModel either persists its argument (the control) or drops it (the
// #792 backend). With the shared-pointer mocks the rest of the suite uses, a
// read-back check could never fail.
func TestAlterAssociation_RefusesToReportAWriteThatDidNotLand(t *testing.T) {
	ops := []struct {
		name string
		stmt ast.AlterAssociationStmt
	}{
		{"owner", ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetOwner, Owner: ast.OwnerBoth}},
		{"storage", ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetStorage, Storage: ast.StorageTable}},
		{"comment", ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetComment, Comment: "why"}},
		{"delete", ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetDeleteBehavior,
			DeleteBehavior: ast.DeleteIfNoReferences, DeleteErrorMessage: "in use"}},
	}
	for _, cross := range []bool{false, true} {
		kind := "same-module"
		if cross {
			kind = "cross-module"
		}
		for _, op := range ops {
			for _, persist := range []bool{true, false} {
				label := "persisting"
				if !persist {
					label = "dropping"
				}
				t.Run(kind+"/"+op.name+"/"+label, func(t *testing.T) {
					ctx := storeLikeAssocFixture(t, cross, persist)
					s := op.stmt
					s.Name = ast.QualifiedName{Module: "M", Name: "Child_Parent"}
					err := execAlterAssociation(ctx, &s)
					if persist {
						assertNoError(t, err)
						return
					}
					if err == nil {
						t.Fatalf("the backend wrote nothing and the statement reported success")
					}
					if !strings.Contains(err.Error(), "not persisted") {
						t.Errorf("error does not say the change was not persisted: %v", err)
					}
				})
			}
		}
	}

	// A no-op anchor alter on a same-module association is still verified, and
	// the cross-module one keeps its refusal (no anchors are stored for those).
	t.Run("same-module/anchor/dropping", func(t *testing.T) {
		ctx := storeLikeAssocFixture(t, false, false)
		err := execAlterAssociation(ctx, &ast.AlterAssociationStmt{
			Name:       ast.QualifiedName{Module: "M", Name: "Child_Parent"},
			Operation:  ast.AlterAssociationSetAnchor,
			FromAnchor: &ast.Position{X: 10, Y: 0},
			ToAnchor:   &ast.Position{X: 90, Y: 100},
		})
		assertError(t, err)
	})
}

// storeLikeAssocFixture builds module M holding Child_Parent — a regular
// association, or a cross-module one to Other.Parent — behind a mock that copies
// on read and persists (or drops) on write.
func storeLikeAssocFixture(t *testing.T, cross, persist bool) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	child := mkEntity(mod.ID, "Child")
	parent := mkEntity(mod.ID, "Parent")
	stored := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{child, parent},
	}
	if cross {
		ca := &domainmodel.CrossModuleAssociation{
			Name: "Child_Parent", ParentID: child.ID, ChildRef: "Other.Parent",
			Type: "Reference", Owner: "Default", StorageFormat: "Column",
			ChildDeleteBehavior: &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences},
		}
		ca.ID = nextID("xassoc")
		stored.CrossAssociations = []*domainmodel.CrossModuleAssociation{ca}
	} else {
		a := mkAssociation(mod.ID, "Child_Parent", child.ID, parent.ID)
		a.Owner, a.StorageFormat = "Default", "Column"
		a.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences}
		stored.Associations = []*domainmodel.Association{a}
	}

	h := mkHierarchy(mod)
	withContainer(h, stored.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
			return copyDomainModelForTest(stored), nil
		},
		UpdateDomainModelFunc: func(dm *domainmodel.DomainModel) error {
			if persist {
				stored = copyDomainModelForTest(dm)
			}
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// copyDomainModelForTest copies the association lists deeply enough that a
// mutation of the copy never reaches the original.
func copyDomainModelForTest(dm *domainmodel.DomainModel) *domainmodel.DomainModel {
	out := *dm
	out.Associations = nil
	for _, a := range dm.Associations {
		c := *a
		if a.ChildDeleteBehavior != nil {
			db := *a.ChildDeleteBehavior
			c.ChildDeleteBehavior = &db
		}
		if a.ParentConnection != nil {
			p := *a.ParentConnection
			c.ParentConnection = &p
		}
		if a.ChildConnection != nil {
			p := *a.ChildConnection
			c.ChildConnection = &p
		}
		out.Associations = append(out.Associations, &c)
	}
	out.CrossAssociations = nil
	for _, ca := range dm.CrossAssociations {
		c := *ca
		if ca.ChildDeleteBehavior != nil {
			db := *ca.ChildDeleteBehavior
			c.ChildDeleteBehavior = &db
		}
		out.CrossAssociations = append(out.CrossAssociations, &c)
	}
	return &out
}
