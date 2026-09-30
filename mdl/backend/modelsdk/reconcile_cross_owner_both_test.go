// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#802: `OWNER Both` makes an association a member of its TO entity as
// well, and for a CROSS-module association that entity lives in another module.
// ReconcileMemberAccesses only ever looked at the domain model it was handed, so
// the TO entity's rules never got the entry — mx check: CE0066 "Entity access is
// out of date" at the TO module's domain model, and `update security` reported
// "All entity access rules are up to date" over it. Measured on PedApp
// (mxbuild 11.13): MyFirstModule.Note_Account → Administration.Account.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// crossBothFixture adds MyFirstModule.ZzNote with a populated rule and a
// cross-module association ZzNote_Account FROM it TO Administration.Account
// (a Studio Pro-authored entity with populated rules) with the given owner.
func crossBothFixture(t *testing.T, owner domainmodel.AssociationOwner) (b *Backend, from, to *model.Module) {
	t.Helper()
	proj := copyFixture(t)
	b = New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	from, _ = b.GetModuleByName("MyFirstModule")
	to, _ = b.GetModuleByName("Administration")
	if from == nil || to == nil {
		t.Fatalf("fixture modules missing")
	}
	dm, err := b.GetDomainModel(from.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := b.CreateEntity(dm.ID, &domainmodel.Entity{Name: "ZzNote", Persistable: true,
		Attributes: []*domainmodel.Attribute{{Name: "Title", Type: &domainmodel.StringAttributeType{}}}}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	ids := entityIDs(t, b, from.ID)
	if err := b.CreateCrossAssociation(dm.ID, &domainmodel.CrossModuleAssociation{
		Name: "ZzNote_Account", ParentID: ids["ZzNote"], ChildRef: "Administration.Account",
		Type: "Reference", Owner: owner,
	}); err != nil {
		t.Fatalf("CreateCrossAssociation: %v", err)
	}
	return b, from, to
}

// accountRulesNaming counts Administration.Account's populated rules and how
// many of them carry a MemberAccess for ref.
func accountRulesNaming(t *testing.T, b *Backend, to *model.Module, ref string) (populated, naming int) {
	t.Helper()
	dm, err := b.GetDomainModel(to.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name != "Account" {
			continue
		}
		for _, r := range e.AccessRules {
			if len(r.MemberAccesses) == 0 {
				continue
			}
			populated++
			for _, ma := range r.MemberAccesses {
				if ma.AssociationName == ref {
					naming++
				}
			}
		}
	}
	return populated, naming
}

func reconcileModule(t *testing.T, b *Backend, mod *model.Module) int {
	t.Helper()
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	n, err := b.ReconcileMemberAccesses(dm.ID, mod.Name)
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	return n
}

func TestReconcile_CrossModuleOwnerBothAddsTheToEntitysEntry(t *testing.T) {
	const ref = "MyFirstModule.ZzNote_Account"

	t.Run("owner Both", func(t *testing.T) {
		b, _, to := crossBothFixture(t, "Both")
		n := reconcileModule(t, b, to)
		populated, naming := accountRulesNaming(t, b, to, ref)
		if populated == 0 {
			t.Fatalf("fixture: Administration.Account has no populated rule")
		}
		if naming != populated {
			t.Errorf("%d of %d populated rules on the TO entity name %s — CE0066", naming, populated, ref)
		}
		if n != populated {
			t.Errorf("reconcile reported %d modified rule(s), want %d", n, populated)
		}
		// Idempotent: a second pass has nothing to do.
		if n := reconcileModule(t, b, to); n != 0 {
			t.Errorf("second reconcile modified %d rule(s), want 0", n)
		}
	})

	// Control: with owner Default the association is a member of the FROM
	// entity only, and the TO entity's rules must not name it.
	t.Run("owner Default", func(t *testing.T) {
		b, _, to := crossBothFixture(t, "Default")
		if n := reconcileModule(t, b, to); n != 0 {
			t.Errorf("reconcile modified %d rule(s) of the TO module, want 0", n)
		}
		if _, naming := accountRulesNaming(t, b, to, ref); naming != 0 {
			t.Errorf("%d rule(s) on the TO entity name %s under owner Default", naming, ref)
		}
	})

	// Both → Default: the entry the TO entity got is stale again and must go.
	t.Run("owner Both then Default", func(t *testing.T) {
		b, from, to := crossBothFixture(t, "Both")
		reconcileModule(t, b, to)
		if _, naming := accountRulesNaming(t, b, to, ref); naming == 0 {
			t.Fatalf("precondition: owner Both did not add the entry")
		}
		dm, err := b.GetDomainModel(from.ID)
		if err != nil {
			t.Fatalf("GetDomainModel: %v", err)
		}
		for _, ca := range dm.CrossAssociations {
			if ca.Name == "ZzNote_Account" {
				ca.Owner = "Default"
			}
		}
		if err := b.UpdateDomainModel(dm); err != nil {
			t.Fatalf("UpdateDomainModel: %v", err)
		}
		reconcileModule(t, b, to)
		if _, naming := accountRulesNaming(t, b, to, ref); naming != 0 {
			t.Errorf("%d rule(s) on the TO entity still name %s after owner Default", naming, ref)
		}
	})
}
