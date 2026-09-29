// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue792_UpdateDomainModelPersistsCrossAssociations guards ako/mxcli#792:
// ALTER ASSOCIATION on a cross-module association printed "Altered association"
// and wrote nothing.
//
// The executor mutates dm.CrossAssociations and hands the whole semantic model to
// UpdateDomainModel — which rebuilt Entities and Associations and left the
// CrossAssociations list as raw passthrough, "not represented in the semantic
// model". It had been represented for a long time (crossAssocFromGen fills it on
// every read), so every edit to it was dropped on the floor while the statement
// reported success. CREATE OR MODIFY on a cross-module association and the view
// entity's re-targeting of a cross-module view association ride the same path.
//
// The subject is a Studio Pro-authored association turned cross-module by MOVE
// ENTITY (whose raw transform keeps the stored GUID, #503), so GUID != $ID and a
// rebuild that re-minted it would be visible — an mxcli-created cross
// association has GUID == $ID from birth and could not fail that assertion.
func TestIssue792_UpdateDomainModelPersistsCrossAssociations(t *testing.T) {
	for _, tc := range []struct {
		name string
		// mutate edits the cross association as the executor would and returns
		// the check that the edit reached disk.
		mutate func(ca *domainmodel.CrossModuleAssociation) func(t *testing.T, got *domainmodel.CrossModuleAssociation)
		// unchanged: the statement restates what is stored, so the element's bytes
		// must not move at all (the other cases are the control for this one).
		unchanged bool
	}{
		{
			name: "SetOwner",
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				want := domainmodel.AssociationOwner("Both")
				if ca.Owner == want {
					want = "Default"
				}
				ca.Owner = want
				return func(t *testing.T, got *domainmodel.CrossModuleAssociation) {
					if got.Owner != want {
						t.Errorf("owner not persisted: got %q, want %q", got.Owner, want)
					}
				}
			},
		},
		{
			name: "SetStorage",
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				want := domainmodel.AssociationStorageFormat("Table")
				if ca.StorageFormat == want {
					want = "Column"
				}
				ca.StorageFormat = want
				return func(t *testing.T, got *domainmodel.CrossModuleAssociation) {
					if got.StorageFormat != want {
						t.Errorf("storage not persisted: got %q, want %q", got.StorageFormat, want)
					}
				}
			},
		},
		{
			name: "SetComment",
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				ca.Documentation = "issue 792 comment"
				return func(t *testing.T, got *domainmodel.CrossModuleAssociation) {
					if got.Documentation != "issue 792 comment" {
						t.Errorf("comment not persisted: got %q", got.Documentation)
					}
				}
			},
		},
		{
			name: "SetDeleteBehaviorCascade",
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences}
				return func(t *testing.T, got *domainmodel.CrossModuleAssociation) {
					if got.ChildDeleteBehavior == nil || got.ChildDeleteBehavior.Type != domainmodel.DeleteBehaviorTypeDeleteMeAndReferences {
						t.Errorf("delete behavior not persisted: got %+v", got.ChildDeleteBehavior)
					}
				}
			},
		},
		{
			// The restrict side needs its refusal message on disk, or the runtime
			// does not start (CapTrackV2 §1) — the same rule assocToGen follows.
			name: "SetDeleteBehaviorPreventWithMessage",
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
					Type:         domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
					ErrorMessage: "still referenced",
				}
				return func(t *testing.T, got *domainmodel.CrossModuleAssociation) {
					if got.ChildDeleteBehavior == nil ||
						got.ChildDeleteBehavior.Type != domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences ||
						got.ChildDeleteBehavior.ErrorMessage != "still referenced" {
						t.Errorf("prevent + message not persisted: got %+v", got.ChildDeleteBehavior)
					}
				}
			},
		},
		{
			name:      "NoChange",
			unchanged: true,
			mutate: func(ca *domainmodel.CrossModuleAssociation) func(*testing.T, *domainmodel.CrossModuleAssociation) {
				return func(*testing.T, *domainmodel.CrossModuleAssociation) {}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj, dmID, name := crossAssocSubject(t)

			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			guidBefore := crossAssocGUID(t, b, dmID, name)
			idBefore, rawBefore := crossAssocRaw(t, b, dmID, name)
			if guidBefore == "" || guidBefore == idBefore {
				t.Fatalf("precondition: cross association %s has GUID %q and $ID %q; need them to differ", name, guidBefore, idBefore)
			}

			dm, err := b.GetDomainModelByID(dmID)
			if err != nil {
				t.Fatalf("GetDomainModelByID: %v", err)
			}
			ca := findCrossAssoc(dm, name)
			if ca == nil {
				t.Fatalf("cross association %s not in the semantic model", name)
			}
			before := *ca
			check := tc.mutate(ca)
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })

			after, err := b2.GetDomainModelByID(dmID)
			if err != nil {
				t.Fatalf("GetDomainModelByID after: %v", err)
			}
			got := findCrossAssoc(after, name)
			if got == nil {
				t.Fatalf("cross association %s disappeared", name)
			}
			check(t, got)

			if g := crossAssocGUID(t, b2, dmID, name); g != guidBefore {
				t.Errorf("storage GUID changed: before=%s after=%s", guidBefore, g)
			}
			idAfter, rawAfter := crossAssocRaw(t, b2, dmID, name)
			if idAfter != idBefore {
				t.Errorf("$ID changed: before=%s after=%s", idBefore, idAfter)
			}
			if got.ParentID != before.ParentID || got.ChildRef != before.ChildRef || got.Name != before.Name {
				t.Errorf("endpoints moved: before %s %s->%s, after %s %s->%s",
					before.Name, before.ParentID, before.ChildRef, got.Name, got.ParentID, got.ChildRef)
			}
			if tc.unchanged && !bytes.Equal(rawBefore, rawAfter) {
				t.Errorf("an update that changed nothing rewrote the cross association's bytes")
			}
			if !tc.unchanged && bytes.Equal(rawBefore, rawAfter) {
				t.Errorf("the update reported success and the stored cross association is byte-identical — nothing was written")
			}
		})
	}
}

// crossAssocSubject copies the fixture and moves the child end of a Studio
// Pro-authored association to another module, which converts it in place into a
// cross-module association that keeps its stored GUID. It returns the project,
// the domain model now holding the cross association, and its name.
func crossAssocSubject(t *testing.T) (string, model.ID, string) {
	t.Helper()
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	srcID, srcMod, assocName, _, childID, _ := associationSubject(t, b)
	dstID, dstMod := otherDomainModel(t, b, srcID)
	if dstID == "" {
		t.Skip("fixture has no second loadable domain model to move into")
	}
	ent := entityByID(t, b, srcID, childID)
	if _, err := b.MoveEntity(ent, srcID, dstID, srcMod, dstMod); err != nil {
		t.Fatalf("MoveEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	return proj, srcID, assocName
}

func findCrossAssoc(dm *domainmodel.DomainModel, name string) *domainmodel.CrossModuleAssociation {
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name {
			return ca
		}
	}
	return nil
}

func crossAssocRaw(t *testing.T, b *Backend, dmID model.ID, name string) (string, []byte) {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	for _, el := range gdm.CrossAssociationsItems() {
		ca, ok := el.(*genDm.CrossAssociation)
		if !ok || ca.Name() != name {
			continue
		}
		return rawKeyHex(t, ca.Raw(), "$ID"), append([]byte(nil), ca.Raw()...)
	}
	t.Fatalf("cross association %s not stored", name)
	return "", nil
}

// A restrict side carries a refusal message and no other side does (census in
// assocToGen). Leaving the message behind when an association goes from restrict
// to cascade kept a stale text that DESCRIBE then printed back as
// `on delete cascade error message '…'`.
func TestIssue792_CrossAssociationLeavingRestrictClearsMessage(t *testing.T) {
	proj, dmID, name := crossAssocSubject(t)
	set := func(db *domainmodel.DeleteBehavior) {
		b := New()
		if err := b.Connect(proj); err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer func() { _ = b.Disconnect() }()
		dm, err := b.GetDomainModelByID(dmID)
		if err != nil {
			t.Fatalf("GetDomainModelByID: %v", err)
		}
		findCrossAssoc(dm, name).ChildDeleteBehavior = db
		if err := b.UpdateDomainModel(dm); err != nil {
			t.Fatalf("UpdateDomainModel: %v", err)
		}
	}
	storedMessage := func() (bool, bool) {
		b := New()
		if err := b.Connect(proj); err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer func() { _ = b.Disconnect() }()
		gdm, err := b.loadDomainModelGen(dmID)
		if err != nil {
			t.Fatalf("loadDomainModelGen: %v", err)
		}
		for _, el := range gdm.CrossAssociationsItems() {
			if ca, ok := el.(*genDm.CrossAssociation); ok && ca.Name() == name {
				v, err := ca.Raw().LookupErr("DeleteBehavior", "ChildErrorMessage")
				return err == nil, err == nil && v.Type != bson.TypeNull
			}
		}
		t.Fatalf("cross association %s not stored", name)
		return false, false
	}

	set(&domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences, ErrorMessage: "in use"})
	if _, has := storedMessage(); !has {
		t.Fatalf("control: restrict did not store its message")
	}
	set(&domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences})
	present, has := storedMessage()
	if has {
		t.Errorf("cascade kept the restrict side's refusal message")
	}
	if !present {
		t.Errorf("ChildErrorMessage key missing; Studio Pro writes it as null")
	}
}
