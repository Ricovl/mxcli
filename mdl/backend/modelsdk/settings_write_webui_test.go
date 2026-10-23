// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// readWebUIPart returns the raw Web UI settings part from disk. Studio Pro stores
// it as Forms$WebUIProjectSettingsPart (measured on 11.6, 11.13 and 11.14).
func readWebUIPart(t *testing.T, proj string) map[string]any {
	t.Helper()
	r, err := mmpr.Open(proj)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()
	refs, err := r.ListUnitsByType("Settings$ProjectSettings")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListUnitsByType = %v, %v", refs, err)
	}
	raw, err := r.GetRawUnitBytes(refs[0].ID)
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var doc bson.M
	if err := bsoncompat.UnmarshalDocM(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, part := range settingsoverlay.ArrayElements(doc["Settings"]) {
		if part["$Type"] == "Forms$WebUIProjectSettingsPart" {
			return part
		}
	}
	t.Fatalf("no Forms$WebUIProjectSettingsPart on disk")
	return nil
}

// mendixlabs/mxcli#1549: `alter settings webui (OptimizedClient: 'No')` has to
// reach the stored part. Before, UpdateProjectSettings passed the Web UI part
// through as read, so the value set on the model was dropped on the floor.
func TestUpdateProjectSettings_WebUIOptimizedClient(t *testing.T) {
	proj := copyFixture(t)
	before := readWebUIPart(t, proj)
	if before["UseOptimizedClient"] != "Yes" {
		t.Fatalf("fixture precondition: UseOptimizedClient = %#v, want Yes", before["UseOptimizedClient"])
	}

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	if ps.WebUI == nil {
		t.Fatal("Web UI settings not read")
	}
	ps.WebUI.UseOptimizedClient = "No"
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	after := readWebUIPart(t, proj)
	if after["UseOptimizedClient"] != "No" {
		t.Errorf("stored UseOptimizedClient = %#v, want No", after["UseOptimizedClient"])
	}
	// Only that property changed: the rest of the part — its identity included —
	// is as Studio Pro wrote it.
	delete(before, "UseOptimizedClient")
	delete(after, "UseOptimizedClient")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the write changed more than UseOptimizedClient:\nbefore %#v\nafter  %#v", before, after)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	ps2, err := b2.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings(2): %v", err)
	}
	if ps2.WebUI == nil || ps2.WebUI.UseOptimizedClient != "No" {
		t.Errorf("UseOptimizedClient read back as %+v, want No", ps2.WebUI)
	}
}
