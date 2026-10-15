// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Publishing a specialization and exposing an attribute it inherits wrote the
// member as the specialization's (Probe.Sub.Code). Mendix resolves an attribute
// by the entity that declares it, so mx check reported
//
//	[CE1613] "The selected attribute 'Probe.Sub.Code' no longer exists."
//
// while check and exec were clean. lookupEntityMembers read only the published
// entity's own attributes, so an inherited one fell through to the bare-name
// fallback and the writer prefixed the published entity. It is now qualified by
// its declaring entity — two levels up included — and keeps its EDM type.
func TestPublishedODataService_InheritedAttributeIsQualifiedByDeclaringEntity(t *testing.T) {
	mod := mkModule("Probe")
	str := func(name string) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: name, Type: &domainmodel.StringAttributeType{}}
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{
			{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Root", Persistable: true,
				Attributes: []*domainmodel.Attribute{str("Origin")}},
			{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Base", Persistable: true,
				GeneralizationRef: "Probe.Root",
				Attributes:        []*domainmodel.Attribute{str("Code"), str("Label")}},
			{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Sub", Persistable: true,
				GeneralizationRef: "Probe.Base",
				Attributes:        []*domainmodel.Attribute{str("Extra")}},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Probe" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	et, _ := astEntityDefToModel(ctx, &ast.PublishedEntityDef{
		Entity:      ast.QualifiedName{Module: "Probe", Name: "Sub"},
		ExposedName: "Subs",
		Members: []*ast.PublishedMemberDef{
			{Name: "Code", IsPartOfKey: true},
			{Name: "Label"},
			{Name: "Extra"},
			{Name: "Origin"},
		},
	})

	want := map[string]string{
		"Code":   "Probe.Base.Code",   // inherited one level up
		"Label":  "Probe.Base.Label",  // inherited one level up
		"Extra":  "Extra",             // its own: the writer qualifies it by the published entity
		"Origin": "Probe.Root.Origin", // inherited two levels up
	}
	if len(et.Members) != len(want) {
		t.Fatalf("got %d members, want %d", len(et.Members), len(want))
	}
	for _, m := range et.Members {
		if m.Kind != "attribute" {
			t.Errorf("%s: kind = %q, want attribute", m.ExposedName, m.Kind)
		}
		if got := m.Name; got != want[m.ExposedName] {
			t.Errorf("%s: member name = %q, want %q — Mendix resolves an inherited attribute by its declaring entity (CE1613)",
				m.ExposedName, got, want[m.ExposedName])
		}
		if m.EdmType != "Edm.String" {
			t.Errorf("%s: EdmType = %q, want Edm.String (an unread type is CE5016 from 11.12)", m.ExposedName, m.EdmType)
		}
	}
}

// The association twin: an association the generalization takes part in is
// published from the specialization as an association end (not the attribute
// fallback, which named a PublishedAttribute after it — CE1613), with the
// association's own qualified name, the other side as target, and the
// specialization standing in for the generalization's side.
func TestPublishedODataService_InheritedAssociationIsAnAssociationEnd(t *testing.T) {
	mod := mkModule("Probe")
	baseID, subID, tagID := nextID("ent"), nextID("ent"), nextID("ent")
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{
			{BaseElement: model.BaseElement{ID: baseID}, Name: "Base", Persistable: true},
			{BaseElement: model.BaseElement{ID: subID}, Name: "Sub", Persistable: true, GeneralizationRef: "Probe.Base"},
			{BaseElement: model.BaseElement{ID: tagID}, Name: "Tag", Persistable: true},
		},
		Associations: []*domainmodel.Association{
			// FROM Base TO Tag: a Reference, so to-one from the FROM side.
			{BaseElement: model.BaseElement{ID: nextID("assoc")}, Name: "Base_Tag", ParentID: baseID, ChildID: tagID, Type: domainmodel.AssociationTypeReference},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Probe" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	et, _ := astEntityDefToModel(ctx, &ast.PublishedEntityDef{
		Entity:  ast.QualifiedName{Module: "Probe", Name: "Sub"},
		Members: []*ast.PublishedMemberDef{{Name: "Base_Tag"}},
	})
	if len(et.Members) != 1 {
		t.Fatalf("got %d members, want 1", len(et.Members))
	}
	m := et.Members[0]
	if m.Kind != "association" {
		t.Fatalf("kind = %q, want association — the attribute fallback is CE1613", m.Kind)
	}
	if m.Name != "Probe.Base_Tag" || m.AssociationTargetEntity != "Probe.Tag" || m.IsMany {
		t.Errorf("got name %q target %q isMany %v, want Probe.Base_Tag → Probe.Tag, to-one",
			m.Name, m.AssociationTargetEntity, m.IsMany)
	}
}
