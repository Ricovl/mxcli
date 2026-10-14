// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"os"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1451, reported as: "`alter entity … add attribute` writes the
// entity back, and an existing index that includes a system member … comes back
// corrupted: each such segment is written with `Type = Normal` and still no
// attribute pointer", after which mxbuild throws ArgumentOutOfRangeException in
// IndexedAttribute.get_Attribute.
//
// This is the reporter's sequence through the real write path. The index is
// seeded the way Studio Pro stores it — Type CreatedDate / ChangedDate, an
// all-zero AttributePointer — by patching the unit below the writer, so the
// test does not depend on mxcli being able to author the shape it then reads.
// The ALTER is the backend half of `alter entity … add attribute`: re-read the
// entity, append the attribute, write it back.
func TestIssue1451_AddAttributeKeepsSystemMemberIndexSegments(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	dmID, _ := richestEntity(t, b)
	const entName = "Issue1451Item"
	ent := &domainmodel.Entity{
		Name:           entName,
		Persistable:    true,
		HasCreatedDate: true,
		HasChangedDate: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 50}},
		},
	}
	if err := b.CreateEntity(dmID, ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	// Two ordinary segments, so the unit holds two IndexedAttributes to re-type.
	stored := mustFindEntity(t, b, dmID, entName)
	nameID := stored.Attributes[0].ID
	stored.Indexes = []*domainmodel.Index{{
		Attributes: []*domainmodel.IndexAttribute{
			{AttributeID: nameID, Ascending: true},
			{AttributeID: nameID, Ascending: true},
		},
	}}
	if err := b.UpdateEntity(dmID, stored); err != nil {
		t.Fatalf("UpdateEntity (add index): %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	want := []string{"CreatedDate", "ChangedDate"}
	seedSystemIndexSegments(t, proj, dmID, entName, want)
	before := systemIndexSegments(t, proj, dmID, entName)
	for i, s := range before {
		if s.typ != want[i] || !s.zeroPointer {
			t.Fatalf("seed did not take: segment %d = %+v, want Type %s with a zero pointer", i, s, want[i])
		}
	}

	// alter entity … add attribute "X": String(50);
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	target := mustFindEntity(t, b2, dmID, entName)
	target.Attributes = append(target.Attributes, &domainmodel.Attribute{
		Name: "X", Type: &domainmodel.StringAttributeType{Length: 50},
	})
	if err := b2.UpdateEntity(dmID, target); err != nil {
		t.Fatalf("UpdateEntity (add attribute): %v", err)
	}
	if err := b2.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	after := systemIndexSegments(t, proj, dmID, entName)
	if len(after) != len(before) {
		t.Fatalf("index has %d segments after the ALTER, want %d", len(after), len(before))
	}
	for i, s := range after {
		if s.typ != before[i].typ {
			t.Errorf("segment %d Type = %q after the ALTER, want %q", i, s.typ, before[i].typ)
		}
		if !s.zeroPointer {
			t.Errorf("segment %d AttributePointer is no longer all-zero after the ALTER", i)
		}
		if s.id != before[i].id {
			t.Errorf("segment %d $ID = %s after the ALTER, want %s", i, s.id, before[i].id)
		}
	}
}

type indexSegment struct {
	id          string
	typ         string
	zeroPointer bool
}

// systemIndexSegments reads the entity's index segments from the stored unit.
func systemIndexSegments(t *testing.T, proj string, dmID model.ID, entName string) []indexSegment {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = b.Disconnect() }()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	var out []indexSegment
	for _, el := range gdm.EntitiesItems() {
		ge, ok := el.(*genDm.Entity)
		if !ok || ge.Name() != entName {
			continue
		}
		for _, iel := range ge.IndexesItems() {
			idx, ok := iel.(*genDm.Index)
			if !ok {
				continue
			}
			for _, sel := range idx.AttributesItems() {
				seg, ok := sel.(*genDm.IndexedAttribute)
				if !ok {
					continue
				}
				out = append(out, indexSegment{
					id:          string(seg.ID()),
					typ:         seg.Type(),
					zeroPointer: rawKeyHex(t, seg.Raw(), "AttributePointer") == "00000000000000000000000000000000",
				})
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no index segments stored on %s", entName)
	}
	return out
}

// seedSystemIndexSegments re-types the entity's stored index segments, in
// order, to types[i] and zeroes their AttributePointer — what Studio Pro stores
// for a segment on createdDate / changedDate, which is not an attribute.
func seedSystemIndexSegments(t *testing.T, proj string, dmID model.ID, entName string, types []string) {
	t.Helper()
	path := unitFilePath(t, proj, entName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}

	typeOf := func(d bson.D) string {
		for _, e := range d {
			if e.Key == "$Type" {
				s, _ := e.Value.(string)
				return s
			}
		}
		return ""
	}
	patched := 0
	var patchSegments func(any)
	patchSegments = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if typeOf(x) == "DomainModels$IndexedAttribute" && patched < len(types) {
				for i, e := range x {
					switch e.Key {
					case "Type":
						x[i].Value = types[patched]
					case "AttributePointer":
						if bin, ok := e.Value.(bson.Binary); ok {
							x[i].Value = bson.Binary{Subtype: bin.Subtype, Data: make([]byte, 16)}
						}
					}
				}
				patched++
				return
			}
			for _, e := range x {
				patchSegments(e.Value)
			}
		case bson.A:
			for _, e := range x {
				patchSegments(e)
			}
		}
	}
	// Only inside the named entity: other entities in the unit may be indexed too.
	var findEntity func(any)
	findEntity = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if typeOf(x) == "DomainModels$EntityImpl" { // storage name of DomainModels$Entity
				for _, e := range x {
					if e.Key == "Name" && e.Value == entName {
						patchSegments(x)
						return
					}
				}
			}
			for _, e := range x {
				findEntity(e.Value)
			}
		case bson.A:
			for _, e := range x {
				findEntity(e)
			}
		}
	}
	findEntity(doc)
	if patched != len(types) {
		t.Fatalf("seeded %d index segments, want %d", patched, len(types))
	}
	if bytes.Equal(raw, mustMarshal(t, doc)) {
		t.Fatal("seeding changed nothing")
	}
	writeUnitDoc(t, proj, dmID, path, doc)
}

func mustMarshal(t *testing.T, doc bson.D) []byte {
	t.Helper()
	out, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
