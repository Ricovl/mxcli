// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#802: `alter association … set owner both` wrote the new owner and
// never reconciled the access rules. `OWNER Both` makes the association a member
// of the TO entity too, so that entity's rules were left without the entry and
// mx check reported CE0066 "Entity access is out of date" — on the module of the
// TO entity, which for a cross-module association is not the module altered.
//
// The alter has to reconcile every module an end of the association lives in:
// the declaring module, and for a cross-module association the TO entity's.
func TestAlterAssociation_OwnerReconcilesBothEnds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cross bool
		op    ast.AlterAssociationStmt
		want  []string // modules reconciled
	}{
		{"same-module/owner", false, ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetOwner, Owner: ast.OwnerBoth}, []string{"M"}},
		{"cross-module/owner", true, ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetOwner, Owner: ast.OwnerBoth}, []string{"M", "Other"}},
		{"cross-module/owner-back-to-default", true, ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetOwner, Owner: ast.OwnerDefault}, []string{"M", "Other"}},
		// Control: an alter that does not change membership leaves the rules alone.
		{"same-module/comment", false, ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetComment, Comment: "why"}, nil},
		{"cross-module/comment", true, ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetComment, Comment: "why"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reconciled := reconcileRecordingAssocFixture(t, tc.cross)
			s := tc.op
			s.Name = ast.QualifiedName{Module: "M", Name: "Child_Parent"}
			assertNoError(t, execAlterAssociation(ctx, &s))
			got := append([]string(nil), *reconciled...)
			sort.Strings(got)
			if len(got) != len(tc.want) {
				t.Fatalf("reconciled modules %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("reconciled modules %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// reconcileRecordingAssocFixture is storeLikeAssocFixture with a second module,
// Other, holding the cross-module association's TO entity, and a backend that
// records which modules' member accesses were reconciled.
func reconcileRecordingAssocFixture(t *testing.T, cross bool) (*ExecContext, *[]string) {
	t.Helper()
	ctx := storeLikeAssocFixture(t, cross, true)
	mb := ctx.Backend.(*mock.MockBackend)

	mod := mb.ListModulesFunc
	mods, _ := mod()
	other := mkModule("Other")
	otherDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: other.ID,
		Entities:    []*domainmodel.Entity{mkEntity(other.ID, "Parent")},
	}
	all := append(append([]*model.Module(nil), mods...), other)
	mb.ListModulesFunc = func() ([]*model.Module, error) { return all, nil }
	inner := mb.GetDomainModelFunc
	mb.GetDomainModelFunc = func(id model.ID) (*domainmodel.DomainModel, error) {
		if id == other.ID {
			return otherDM, nil
		}
		return inner(id)
	}
	h := mkHierarchy(all...)
	ctx.Cache = nil
	withHierarchy(h)(ctx)

	var reconciled []string
	mb.ReconcileMemberAccessesFunc = func(_ model.ID, moduleName string) (int, error) {
		reconciled = append(reconciled, moduleName)
		return 0, nil
	}
	return ctx, &reconciled
}
