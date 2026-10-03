// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

const seededDomainModelDoc = "Orders, lines and the customers who place them."

// seedDomainModelDocumentation writes Documentation onto a stored domain model
// unit the way Studio Pro stores it -- a string property on the
// DomainModels$DomainModel unit -- and returns that unit's ID. TestApp has no
// domain model with documentation, so the fixture sets one through the raw unit
// rather than through any mxcli write path (none writes it).
func seedDomainModelDocumentation(t *testing.T, b *Backend) model.ID {
	t.Helper()
	dmID, _ := domainModelWithAPIEntity(t, b)
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	gdm.SetDocumentation(seededDomainModelDoc)
	contents, err := (&codec.Encoder{}).Encode(gdm)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(dmID), contents); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
	stored, err := b.loadDomainModelGen(dmID)
	if err != nil || stored.Documentation() != seededDomainModelDoc {
		t.Fatalf("fixture: stored documentation = %q (%v), want the seeded text", stored.Documentation(), err)
	}
	return dmID
}

// mendixlabs/mxcli#1269: DomainModels$DomainModel.Documentation was read
// nowhere. Reading it into the semantic model must not make any write path
// blank it: the writers load the stored unit and mutate it in place, so the
// value is carried by construction -- including when the semantic model handed
// to UpdateDomainModel has it empty, which is what a statement-built model has.
//
// CONTROL for the blanked case: adding gdm.SetDocumentation(dm.Documentation)
// to UpdateDomainModel makes "BlankedInSemanticModel" fail, so the test detects
// a writer that starts writing the field from the semantic model.
func TestDomainModelDocumentationReadAndCarriedByWrites(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}

	for _, tc := range []struct {
		name  string
		write func(t *testing.T, b *Backend, dmID model.ID)
	}{
		{"ReadOnly", func(*testing.T, *Backend, model.ID) {}},
		{"UpdateDomainModelNoChange", func(t *testing.T, b *Backend, dmID model.ID) {
			if err := b.UpdateDomainModel(reloadDomainModel(t, b, dmID)); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}
		}},
		{"UpdateDomainModelRenameAssociation", func(t *testing.T, b *Backend, dmID model.ID) {
			dm := reloadDomainModel(t, b, dmID)
			dm.Associations[0].Name = "Doc1269_" + dm.Associations[0].Name
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}
		}},
		{"BlankedInSemanticModel", func(t *testing.T, b *Backend, dmID model.ID) {
			dm := reloadDomainModel(t, b, dmID)
			dm.Documentation = ""
			dm.Associations[0].Documentation = "issue 1269"
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}
		}},
		{"CreateEntity", func(t *testing.T, b *Backend, dmID model.ID) {
			e := &domainmodel.Entity{Name: "Doc1269Probe", Persistable: true}
			if err := b.CreateEntity(dmID, e); err != nil {
				t.Fatalf("CreateEntity: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyTestAppFixture(t, src)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			dmID := seedDomainModelDocumentation(t, b)

			tc.write(t, b, dmID)

			if got := reloadDomainModel(t, b, dmID).Documentation; got != seededDomainModelDoc {
				t.Errorf("after %s: read Documentation = %q, want %q", tc.name, got, seededDomainModelDoc)
			}
			stored, err := b.loadDomainModelGen(dmID)
			if err != nil {
				t.Fatal(err)
			}
			if got := stored.Documentation(); got != seededDomainModelDoc {
				t.Errorf("after %s: stored Documentation = %q, want %q -- the write blanked it",
					tc.name, got, seededDomainModelDoc)
			}
		})
	}
}

// mendixlabs/mxcli#1269: the reader read the child end's refusal message but
// not the parent end's, so the catalog could not report it.
func TestAssocFromGenReadsBothDeleteErrorMessages(t *testing.T) {
	db := genDm.NewAssociationDeleteBehavior()
	db.SetChildDeleteBehavior(string(domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences))
	db.SetParentDeleteBehavior(string(domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences))
	db.SetChildErrorMessage(textToGen(&model.Text{Translations: map[string]string{"en_US": "child says no"}}))
	db.SetParentErrorMessage(textToGen(&model.Text{Translations: map[string]string{"en_US": "parent says no"}}))

	a := genDm.NewAssociation()
	a.SetName("Order_Customer")
	a.SetDeleteBehavior(db)
	ca := genDm.NewCrossAssociation()
	ca.SetName("Order_Remote")
	ca.SetDeleteBehavior(db)

	for name, got := range map[string][2]*domainmodel.DeleteBehavior{
		"association":       {assocFromGen(a).ParentDeleteBehavior, assocFromGen(a).ChildDeleteBehavior},
		"cross-association": {crossAssocFromGen(ca).ParentDeleteBehavior, crossAssocFromGen(ca).ChildDeleteBehavior},
	} {
		if got[0] == nil || got[0].ErrorMessage != "parent says no" {
			t.Errorf("%s: parent ErrorMessage = %+v, want %q", name, got[0], "parent says no")
		}
		// Control: the child side was already read.
		if got[1] == nil || got[1].ErrorMessage != "child says no" {
			t.Errorf("%s: child ErrorMessage = %+v, want %q", name, got[1], "child says no")
		}
	}
}
