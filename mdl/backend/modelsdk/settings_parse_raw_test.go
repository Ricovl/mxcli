// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
)

// TestParseProjectSettingsFromRawMatchesStoredRead feeds the method what
// diff-local feeds it — the settings unit's raw BSON, unmarshalled to a map,
// as if read back with `git show` — and asserts it converts the document the
// way GetProjectSettings converts the stored one. RawParts matter as much as
// the typed fields: DESCRIBE SETTINGS prints only the properties the document
// stores, so a parse without them renders a different property list than the
// describe it stands in for (ako/mxcli#1512).
func TestParseProjectSettingsFromRawMatchesStoredRead(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	want, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	refs, err := b.reader.ListUnitsByType("Settings$ProjectSettings")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListUnitsByType(Settings$ProjectSettings) = %d units, %v; want 1", len(refs), err)
	}
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(refs[0].Contents, &raw); err != nil {
		t.Fatal(err)
	}

	got, err := b.ParseProjectSettingsFromRaw(raw)
	if err != nil {
		t.Fatalf("ParseProjectSettingsFromRaw: %v", err)
	}
	if got.Model == nil || got.Configuration == nil || len(got.Configuration.Configurations) == 0 {
		t.Fatalf("parsed settings lack the runtime or configuration parts: %+v", got)
	}
	if !reflect.DeepEqual(got.Model, want.Model) {
		t.Errorf("Model = %+v, want %+v", got.Model, want.Model)
	}
	if !reflect.DeepEqual(got.Configuration, want.Configuration) {
		t.Errorf("Configuration = %+v, want %+v", got.Configuration, want.Configuration)
	}
	if len(got.RawParts) == 0 || len(got.RawParts) != len(want.RawParts) {
		t.Errorf("RawParts: %d parts, want %d (the stored read's)", len(got.RawParts), len(want.RawParts))
	}
}

// TestParseProjectSettingsFromRawRefusesOtherDocuments: diff-local renders
// whatever unit it is handed, so a document of another type must be an error,
// not an empty settings value that renders as "no settings".
func TestParseProjectSettingsFromRawRefusesOtherDocuments(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	refs, err := b.reader.ListUnitsByType("Microflows$Microflow")
	if err != nil || len(refs) == 0 {
		t.Fatalf("no microflow in the fixture: %v", err)
	}
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(refs[0].Contents, &raw); err != nil {
		t.Fatal(err)
	}
	if ps, err := b.ParseProjectSettingsFromRaw(raw); err == nil {
		t.Errorf("a microflow parsed as project settings: %+v", ps)
	}
}
