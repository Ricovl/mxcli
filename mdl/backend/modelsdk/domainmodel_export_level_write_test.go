// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1498 (follow-up, point 2): `check` reports CE5400 for a
// Hidden entity a Usable microflow exposes in a Protected module, and nothing in
// mxcli could write any other level — entityToGen, attributeToGen, assocToGen and
// crossAssocToGen wrote the "Hidden" constant, and the rebuild carry wrote the
// stored level back over whatever the statement said.
//
// Each update case runs both directions: an authored Hidden over a stored API
// alone cannot tell a writer that honours the level from one that writes the
// constant (the #816 lesson), and an authored API over a stored Hidden alone
// cannot tell one that honours it from one that carries the stored level.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func TestCreateDomainElements_WriteAuthoredExportLevel(t *testing.T) {
	b, dmID := exportLevelFixture(t)
	api := &domainmodel.Entity{Name: "ZzApi", Persistable: true, ExportLevel: "API", Attributes: []*domainmodel.Attribute{
		{Name: "Usable", Type: &domainmodel.StringAttributeType{Length: 10}, ExportLevel: "API"},
		{Name: "Plain", Type: &domainmodel.StringAttributeType{Length: 10}},
	}}
	plain := &domainmodel.Entity{Name: "ZzPlain", Persistable: true}
	for _, e := range []*domainmodel.Entity{api, plain} {
		if err := b.CreateEntity(dmID, e); err != nil {
			t.Fatalf("CreateEntity %s: %v", e.Name, err)
		}
	}
	assoc := &domainmodel.Association{Name: "ZzApi_ZzPlain", ParentID: api.ID, ChildID: plain.ID,
		Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault, ExportLevel: "API"}
	if err := b.CreateAssociation(dmID, assoc); err != nil {
		t.Fatalf("CreateAssociation: %v", err)
	}
	cross := &domainmodel.CrossModuleAssociation{Name: "ZzApi_User", ParentID: api.ID, ChildRef: "System.User",
		Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault, ExportLevel: "API"}
	if err := b.CreateCrossAssociation(dmID, cross); err != nil {
		t.Fatalf("CreateCrossAssociation: %v", err)
	}

	got := storedDomainLevels(t, b, dmID)
	for key, want := range map[string]string{
		"entity ZzApi":              "API",
		"entity ZzPlain":            "Hidden",
		"attribute ZzApi.Usable":    "API",
		"attribute ZzApi.Plain":     "Hidden",
		"association ZzApi_ZzPlain": "API",
		"cross ZzApi_User":          "API",
	} {
		if got[key] != want {
			t.Errorf("%s: stored ExportLevel %q, want %q", key, got[key], want)
		}
	}

	// The read fills the semantic model, which describe prints and the
	// Protected-module check consults.
	dm := reloadDomainModel(t, b, dmID)
	for _, e := range dm.Entities {
		if e.Name == "ZzApi" && (e.ExportLevel != "API" || e.Attributes[0].ExportLevel != "API" || e.Attributes[1].ExportLevel != "Hidden") {
			t.Errorf("read back entity %q attributes %q/%q", e.ExportLevel, e.Attributes[0].ExportLevel, e.Attributes[1].ExportLevel)
		}
	}
	for _, a := range dm.Associations {
		if a.Name == "ZzApi_ZzPlain" && a.ExportLevel != "API" {
			t.Errorf("read back association ExportLevel %q", a.ExportLevel)
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == "ZzApi_User" && ca.ExportLevel != "API" {
			t.Errorf("read back cross association ExportLevel %q", ca.ExportLevel)
		}
	}
}

func TestUpdateDomainElements_AuthoredExportLevelWins(t *testing.T) {
	for _, tc := range []struct{ stored, authored, want string }{
		{"Hidden", "API", "API"},
		{"API", "Hidden", "Hidden"},
		{"API", "", "API"}, // unauthored keeps the stored level (#801)
	} {
		t.Run(tc.stored+"->"+orDefault(tc.authored, "unauthored"), func(t *testing.T) {
			b, dmID := exportLevelFixture(t)
			e := &domainmodel.Entity{Name: "ZzSubject", Persistable: true, Attributes: []*domainmodel.Attribute{
				{Name: "A", Type: &domainmodel.StringAttributeType{Length: 10}},
				{Name: "B", Type: &domainmodel.StringAttributeType{Length: 10}},
			}}
			other := &domainmodel.Entity{Name: "ZzOther", Persistable: true}
			for _, x := range []*domainmodel.Entity{e, other} {
				if err := b.CreateEntity(dmID, x); err != nil {
					t.Fatalf("CreateEntity: %v", err)
				}
			}
			if err := b.CreateAssociation(dmID, &domainmodel.Association{Name: "ZzSubject_ZzOther", ParentID: e.ID, ChildID: other.ID,
				Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault}); err != nil {
				t.Fatalf("CreateAssociation: %v", err)
			}
			if err := b.CreateCrossAssociation(dmID, &domainmodel.CrossModuleAssociation{Name: "ZzSubject_User", ParentID: e.ID,
				ChildRef: "System.User", Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault}); err != nil {
				t.Fatalf("CreateCrossAssociation: %v", err)
			}
			setStoredDomainLevels(t, b, dmID, tc.stored)

			// UpdateEntity: the entity and attribute A carry the authored level,
			// attribute B never says one and keeps what is stored.
			ent := entityNamed(t, reloadDomainModel(t, b, dmID), "ZzSubject")
			ent.ExportLevel = tc.authored
			ent.Attributes[0].ExportLevel = tc.authored
			ent.Attributes[1].ExportLevel = ""
			if err := b.UpdateEntity(dmID, ent); err != nil {
				t.Fatalf("UpdateEntity: %v", err)
			}
			// UpdateAttribute, the single-attribute path.
			attrB := entityNamed(t, reloadDomainModel(t, b, dmID), "ZzSubject").Attributes[1]
			attrB.ExportLevel = tc.authored
			attrB.Documentation = "touched"
			if err := b.UpdateAttribute(dmID, ent.ID, attrB); err != nil {
				t.Fatalf("UpdateAttribute: %v", err)
			}
			// UpdateDomainModel: associations, regular and cross-module.
			dm := reloadDomainModel(t, b, dmID)
			for _, a := range dm.Associations {
				if a.Name == "ZzSubject_ZzOther" {
					a.ExportLevel = tc.authored
				}
			}
			for _, ca := range dm.CrossAssociations {
				if ca.Name == "ZzSubject_User" {
					ca.ExportLevel = tc.authored
				}
			}
			if err := b.UpdateDomainModel(dm); err != nil {
				t.Fatalf("UpdateDomainModel: %v", err)
			}

			got := storedDomainLevels(t, b, dmID)
			for _, key := range []string{"entity ZzSubject", "attribute ZzSubject.A", "attribute ZzSubject.B",
				"association ZzSubject_ZzOther", "cross ZzSubject_User"} {
				if got[key] != tc.want {
					t.Errorf("%s: stored %s, authored %q: ExportLevel %q, want %q", key, tc.stored, tc.authored, got[key], tc.want)
				}
			}
		})
	}
}

// An enumeration is a document of its own; the update direction is a case of
// TestUpdatePaths_KeepStoredExportLevel.
func TestCreateEnumeration_WritesAuthoredExportLevel(t *testing.T) {
	b, _ := exportLevelFixture(t)
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	for _, lvl := range []string{"API", ""} {
		enum := &model.Enumeration{ContainerID: mod.ID, Name: "ZzKind" + lvl, ExportLevel: lvl,
			Values: []model.EnumerationValue{{Name: "A"}}}
		if err := b.CreateEnumeration(enum); err != nil {
			t.Fatalf("CreateEnumeration: %v", err)
		}
		want := orDefault(lvl, "Hidden")
		if got := storedKeys(t, b, enum.ID)["ExportLevel"]; got != want {
			t.Errorf("authored %q: stored ExportLevel %#v, want %s", lvl, got, want)
		}
		if got, err := b.GetEnumeration(enum.ID); err != nil || got.ExportLevel != want {
			t.Errorf("authored %q: read back ExportLevel %q (%v), want %s", lvl, got.ExportLevel, err, want)
		}
	}
}

func exportLevelFixture(t *testing.T) (*Backend, model.ID) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	return b, dm.ID
}

func entityNamed(t *testing.T, dm *domainmodel.DomainModel, name string) *domainmodel.Entity {
	t.Helper()
	for _, e := range dm.Entities {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entity %s not found", name)
	return nil
}

// storedDomainLevels reads the ExportLevel stored on every entity, attribute,
// association and cross-module association of the unit, keyed by kind and name.
func storedDomainLevels(t *testing.T, b *Backend, dmID model.ID) map[string]string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	out := map[string]string{}
	for _, el := range gdm.EntitiesItems() {
		ge, ok := el.(*genDm.Entity)
		if !ok {
			continue
		}
		out["entity "+ge.Name()] = ge.ExportLevel()
		for _, ael := range ge.AttributesItems() {
			if ga, ok := ael.(*genDm.Attribute); ok {
				out["attribute "+ge.Name()+"."+ga.Name()] = ga.ExportLevel()
			}
		}
	}
	for _, el := range gdm.AssociationsItems() {
		if ga, ok := el.(*genDm.Association); ok {
			out["association "+ga.Name()] = ga.ExportLevel()
		}
	}
	for _, el := range gdm.CrossAssociationsItems() {
		if gca, ok := el.(*genDm.CrossAssociation); ok {
			out["cross "+gca.Name()] = gca.ExportLevel()
		}
	}
	return out
}

// setStoredDomainLevels sets the stored ExportLevel of every Zz-prefixed
// element of the unit, the way Studio Pro would have written it.
func setStoredDomainLevels(t *testing.T, b *Backend, dmID model.ID, lvl string) {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	isSubject := func(name string) bool { return len(name) > 2 && name[:2] == "Zz" }
	for _, el := range gdm.EntitiesItems() {
		if ge, ok := el.(*genDm.Entity); ok && isSubject(ge.Name()) {
			ge.SetExportLevel(lvl)
			for _, ael := range ge.AttributesItems() {
				if ga, ok := ael.(*genDm.Attribute); ok {
					ga.SetExportLevel(lvl)
				}
			}
		}
	}
	for _, el := range gdm.AssociationsItems() {
		if ga, ok := el.(*genDm.Association); ok && isSubject(ga.Name()) {
			ga.SetExportLevel(lvl)
		}
	}
	for _, el := range gdm.CrossAssociationsItems() {
		if gca, ok := el.(*genDm.CrossAssociation); ok && isSubject(gca.Name()) {
			gca.SetExportLevel(lvl)
		}
	}
	if err := b.persistDM(dmID, gdm); err != nil {
		t.Fatalf("persist stored levels: %v", err)
	}
	got := storedDomainLevels(t, b, dmID)
	if got["entity ZzSubject"] != lvl {
		t.Fatalf("precondition: stored entity level %q after setting %q", got["entity ZzSubject"], lvl)
	}
}
