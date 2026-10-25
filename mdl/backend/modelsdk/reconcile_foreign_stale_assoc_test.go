// SPDX-License-Identifier: Apache-2.0

// A MemberAccess for an association is qualified by the module that DECLARES the
// association. On the TO entity of a cross-module `OWNER Both` association that
// is another module, and ReconcileMemberAccesses preserved every such foreign
// reference it could not find among the entity's members — the "preserve what
// cannot be checked" rule written for associations inherited from a
// generalization in another module (#758). So once the association was gone,
// the entry stayed for good.
//
// ako/mxcli#1449: `drop entity` on the FROM end removes the association, and the
// TO entity's rule went on naming it — mx check CE1613 "The selected association
// 'Crm.Lead_Order' no longer exists". `drop association` on the same association
// left the same entry, and `update security` printed "All entity access rules
// are up to date" over both. Measured on mxbuild 11.13.0.
//
// That case is checkable after all: an association its declaring module no
// longer declares is a member of nothing.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The #1449 shape: the FROM end of a cross-module OWNER Both association goes
// away (drop entity), or the association itself does (drop association), and the
// TO entity — Administration.Account, a specialization of System.User — must
// lose the entry.
func TestReconcile_DropsAForeignAssociationThatNoLongerExists(t *testing.T) {
	const ref = "MyFirstModule.ZzNote_Account"
	for _, tc := range []struct {
		name   string
		remove func(t *testing.T, b *Backend, from *model.Module)
	}{
		{"drop entity on the FROM end", func(t *testing.T, b *Backend, from *model.Module) {
			dm, err := b.GetDomainModel(from.ID)
			if err != nil {
				t.Fatalf("GetDomainModel: %v", err)
			}
			if err := b.DeleteEntity(dm.ID, entityIDs(t, b, from.ID)["ZzNote"]); err != nil {
				t.Fatalf("DeleteEntity: %v", err)
			}
		}},
		{"drop association", func(t *testing.T, b *Backend, from *model.Module) {
			dm, err := b.GetDomainModel(from.ID)
			if err != nil {
				t.Fatalf("GetDomainModel: %v", err)
			}
			for _, ca := range dm.CrossAssociations {
				if ca.Name == "ZzNote_Account" {
					if err := b.DeleteCrossAssociation(dm.ID, ca.ID); err != nil {
						t.Fatalf("DeleteCrossAssociation: %v", err)
					}
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, from, to := crossBothFixture(t, "Both")
			reconcileModule(t, b, to)
			populated, naming := accountRulesNaming(t, b, to, ref)
			if populated == 0 || naming != populated {
				t.Fatalf("precondition: %d of %d populated rules name %s", naming, populated, ref)
			}

			tc.remove(t, b, from)

			n := reconcileModule(t, b, to)
			if _, naming := accountRulesNaming(t, b, to, ref); naming != 0 {
				t.Errorf("%d rule(s) on the TO entity still name %s after it was removed — CE1613", naming, ref)
			}
			// The count drives `update security`'s "All entity access rules are
			// up to date", so it is part of the contract.
			if n != populated {
				t.Errorf("reconcile reported %d modified rule(s), want %d", n, populated)
			}
		})
	}
}

// targetFixture adds, beside crossBothFixture's ZzNote, an entity
// Administration.ZzTarget with no generalization and a populated rule, plus the
// cross-module association MyFirstModule.ZzNote_Target FROM ZzNote TO it, owned
// by both ends.
func targetFixture(t *testing.T) (b *Backend, from, to *model.Module) {
	t.Helper()
	b, from, to = crossBothFixture(t, "Default")
	toDM, err := b.GetDomainModel(to.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := b.CreateEntity(toDM.ID, &domainmodel.Entity{Name: "ZzTarget", Persistable: true,
		Attributes: []*domainmodel.Attribute{{Name: "Code", Type: &domainmodel.StringAttributeType{}}}}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := b.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID: toDM.ID, EntityName: "ZzTarget",
		RoleNames:           []string{"Administration.Administrator"},
		DefaultMemberAccess: "ReadWrite",
		MemberAccesses: []types.EntityMemberAccess{
			{AttributeRef: "Administration.ZzTarget.Code", AccessRights: "ReadWrite"},
		},
	}); err != nil {
		t.Fatalf("AddEntityAccessRule: %v", err)
	}
	fromDM, err := b.GetDomainModel(from.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := b.CreateCrossAssociation(fromDM.ID, &domainmodel.CrossModuleAssociation{
		Name: "ZzNote_Target", ParentID: entityIDs(t, b, from.ID)["ZzNote"], ChildRef: "Administration.ZzTarget",
		Type: "Reference", Owner: "Both",
	}); err != nil {
		t.Fatalf("CreateCrossAssociation: %v", err)
	}
	return b, from, to
}

// entityNamesRef reports whether any of an entity's rules carries a
// MemberAccess for the association ref.
func entityNamesRef(t *testing.T, b *Backend, mod *model.Module, entity, ref string) bool {
	t.Helper()
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name != entity {
			continue
		}
		for _, r := range e.AccessRules {
			for _, ma := range r.MemberAccesses {
				if ma.AssociationName == ref {
					return true
				}
			}
		}
		return false
	}
	t.Fatalf("entity %s not found in %s", entity, mod.Name)
	return false
}

// Control: an association declared in another module that this entity INHERITS
// — from a generalization in that module — still exists and is a member, and
// the reconcile cannot see that from here. Judging foreign references must not
// reach it: it stays.
func TestReconcile_KeepsAForeignAssociationInheritedAcrossModules(t *testing.T) {
	const ref = "MyFirstModule.ZzNote_Target"
	b, from, to := targetFixture(t)
	toDM, err := b.GetDomainModel(to.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := b.CreateEntity(toDM.ID, &domainmodel.Entity{Name: "ZzSpecial", Persistable: true,
		GeneralizationRef: from.Name + ".ZzNote"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := b.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID: toDM.ID, EntityName: "ZzSpecial",
		RoleNames:           []string{"Administration.Administrator"},
		DefaultMemberAccess: "ReadWrite",
		MemberAccesses: []types.EntityMemberAccess{
			{AttributeRef: "MyFirstModule.ZzNote.Title", AccessRights: "ReadWrite"},
			{AssociationRef: ref, AccessRights: "ReadWrite"},
		},
	}); err != nil {
		t.Fatalf("AddEntityAccessRule: %v", err)
	}

	reconcileModule(t, b, to)
	if !entityNamesRef(t, b, to, "ZzSpecial", ref) {
		t.Fatalf("an association inherited from a generalization in another module was removed — CE0066")
	}
}
