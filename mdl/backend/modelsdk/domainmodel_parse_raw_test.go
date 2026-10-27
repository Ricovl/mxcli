// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
)

// TestParseDomainModelFromRawMatchesStoredRead feeds the method every domain
// model in the fixture as diff-local does — raw BSON unmarshalled to a map,
// with no unit to look up — and asserts each entity's access rules come out as
// GetDomainModel reads them. diff-local renders the rules from this parse with
// DESCRIBE ENTITY's writer, so a lossy parse would put the rules on the page
// and still hide a GRANT's change (ako/mxcli#1562).
func TestParseDomainModelFromRawMatchesStoredRead(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	refs, err := b.reader.ListUnitsByType("DomainModels$DomainModel")
	if err != nil || len(refs) == 0 {
		t.Fatalf("no domain model in the fixture: %v", err)
	}
	rules := 0
	for _, ref := range refs {
		var raw map[string]any
		if err := bsoncompat.UnmarshalDocMap(ref.Contents, &raw); err != nil {
			t.Fatal(err)
		}
		got, err := b.ParseDomainModelFromRaw(raw, model.ID(ref.ID), model.ID(ref.ContainerID))
		if err != nil {
			t.Fatalf("ParseDomainModelFromRaw(%s): %v", ref.ID, err)
		}
		want, err := b.GetDomainModel(model.ID(ref.ContainerID))
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Entities) != len(want.Entities) {
			t.Fatalf("%s: %d entities, want %d", ref.ID, len(got.Entities), len(want.Entities))
		}
		for i, e := range got.Entities {
			if e.Name != want.Entities[i].Name {
				t.Errorf("entity %d = %q, want %q", i, e.Name, want.Entities[i].Name)
			}
			if !reflect.DeepEqual(e.AccessRules, want.Entities[i].AccessRules) {
				t.Errorf("%s access rules = %+v, want %+v", e.Name, e.AccessRules, want.Entities[i].AccessRules)
			}
			rules += len(e.AccessRules)
		}
	}
	if rules == 0 {
		t.Fatal("no entity in the fixture has an access rule — the comparison above checked nothing")
	}
}

func TestParseDomainModelFromRawRefusesOtherDocuments(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	refs, err := b.reader.ListUnitsByType("Settings$ProjectSettings")
	if err != nil || len(refs) == 0 {
		t.Fatalf("no settings unit in the fixture: %v", err)
	}
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(refs[0].Contents, &raw); err != nil {
		t.Fatal(err)
	}
	if dm, err := b.ParseDomainModelFromRaw(raw, "", ""); err == nil {
		t.Errorf("a settings document parsed as a domain model: %+v", dm)
	}
}
