// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// attributeNewType returns the stored $Type of the named attribute's NewType —
// the bytes Studio Pro reads, not the semantic reader's interpretation of them.
func attributeNewType(t *testing.T, b *Backend, dmID, entID model.ID, name string) string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	ge := findGenEntity(gdm, entID)
	if ge == nil {
		t.Fatalf("gen entity %s not found", entID)
	}
	for _, el := range ge.AttributesItems() {
		a, ok := el.(*genDm.Attribute)
		if !ok || a.Name() != name {
			continue
		}
		rv, err := a.Raw().LookupErr("NewType")
		if err != nil {
			t.Fatalf("attribute %s has no NewType: %v", name, err)
		}
		doc, ok := rv.DocumentOK()
		if !ok {
			t.Fatalf("attribute %s NewType is not a document", name)
		}
		typ, err := doc.LookupErr("$Type")
		if err != nil {
			t.Fatalf("attribute %s NewType has no $Type", name)
		}
		return typ.StringValue()
	}
	t.Fatalf("attribute %s not found", name)
	return ""
}

// Changing an existing attribute's type to HashedString and back must write
// DomainModels$HashedStringAttributeType and keep the attribute's storage GUID
// both ways: the runtime keys mendixsystem$attribute.id on it, and a re-minted
// GUID drops the column and its data (#1119). The fixture attribute is
// Studio Pro-authored (GUID != $ID, asserted) — on an mxcli-created one
// re-minting GUID = $ID reproduces the same value and nothing could fail.
func TestHashedStringAttribute_TypeChangeEncodesAndKeepsGUID(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	dmID, ent := richestEntity(t, b)
	if len(ent.Attributes) < 2 {
		t.Fatalf("fixture entity %s has %d attributes; need >= 2", ent.Name, len(ent.Attributes))
	}
	target := ent.Attributes[1].Name
	guids := attributeGUIDs(t, b, dmID, ent.ID)
	ids := attributeIDs(t, b, dmID, ent.ID)
	wantGUID := guids[target]
	if wantGUID == "" || wantGUID == ids[target] {
		t.Fatalf("fixture attribute %s: GUID %q vs $ID %q — need a Studio Pro-authored attribute (GUID != $ID)",
			target, wantGUID, ids[target])
	}

	steps := []struct {
		name     string
		typ      domainmodel.AttributeType
		wantType string
		wantName string
	}{
		{"to HashedString", &domainmodel.HashedStringAttributeType{}, "DomainModels$HashedStringAttributeType", "HashedString"},
		{"back to String", &domainmodel.StringAttributeType{Length: 200}, "DomainModels$StringAttributeType", "String"},
	}
	for _, st := range steps {
		for _, a := range ent.Attributes {
			if a.Name == target {
				a.Type = st.typ
			}
		}
		if err := b.UpdateEntity(dmID, ent); err != nil {
			t.Fatalf("%s: UpdateEntity: %v", st.name, err)
		}
		if err := b.Disconnect(); err != nil {
			t.Fatalf("%s: disconnect: %v", st.name, err)
		}
		// Reopen: only what reached disk counts.
		b = New()
		if err := b.Connect(proj); err != nil {
			t.Fatalf("%s: reconnect: %v", st.name, err)
		}

		if got := attributeNewType(t, b, dmID, ent.ID, target); got != st.wantType {
			t.Errorf("%s: stored NewType $Type = %s, want %s", st.name, got, st.wantType)
		}
		if got := attributeGUIDs(t, b, dmID, ent.ID)[target]; got != wantGUID {
			t.Errorf("%s: storage GUID changed %s -> %s — the runtime would drop the column", st.name, wantGUID, got)
		}
		dms, err := b.ListDomainModels()
		if err != nil {
			t.Fatalf("ListDomainModels: %v", err)
		}
		for _, d := range dms {
			if d.ID != dmID {
				continue
			}
			for _, e := range d.Entities {
				if e.ID != ent.ID {
					continue
				}
				ent = e // carry the re-read entity into the next step
				for _, a := range e.Attributes {
					if a.Name == target && a.Type.GetTypeName() != st.wantName {
						t.Errorf("%s: read back as %s, want %s", st.name, a.Type.GetTypeName(), st.wantName)
					}
				}
			}
		}
	}
	_ = b.Disconnect()
}
