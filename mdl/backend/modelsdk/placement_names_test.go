// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestListDocumentUnits_NamesMatchFullDecode pins what eachDocumentUnit
// reports to what a full decode of each document says. The walk reads only the
// top-level Name now (describe module ... with all spent ~2 s of a 7 s run
// unmarshalling every document of a 23k-unit project to name it), so this is
// the guard that the cheaper read names the same documents and skips the same
// ones.
func TestListDocumentUnits_NamesMatchFullDecode(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	got, err := b.ListDocumentUnits()
	if err != nil {
		t.Fatalf("ListDocumentUnits: %v", err)
	}
	gotNames := make(map[string]string, len(got))
	for _, d := range got {
		gotNames[string(d.ID)] = d.Name
	}

	units, err := b.reader.ListUnits()
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	want := 0
	for _, u := range units {
		if u.ContainmentName != "Documents" {
			continue
		}
		raw, err := b.reader.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		name := docNameOf(doc)
		if name == "" {
			if _, listed := gotNames[u.ID]; listed {
				t.Errorf("unit %s has no name but was listed", u.ID)
			}
			continue
		}
		want++
		if gotNames[u.ID] != name {
			t.Errorf("unit %s: listed as %q, full decode names it %q", u.ID, gotNames[u.ID], name)
		}
	}
	if want == 0 {
		t.Fatal("fixture has no named document units; the comparison checked nothing")
	}
	if len(got) != want {
		t.Errorf("listed %d document units, full decode names %d", len(got), want)
	}
}
