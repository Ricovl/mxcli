// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1498 (follow-up): `export level api` on a nanoflow parsed,
// check passed, exec reported "Created nanoflow", and the document was stored
// with ExportLevel "Hidden" — the clause never reached the writer. A microflow
// with the same clause was stored as API.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestCreateNanoflow_WritesAuthoredExportLevel(t *testing.T) {
	b, _ := nanoflowFixture(t)
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	nf := &microflows.Nanoflow{ContainerID: mod.ID, Name: "ZzApiNanoflow", ExportLevel: "API"}
	if err := b.CreateNanoflow(nf); err != nil {
		t.Fatalf("CreateNanoflow: %v", err)
	}
	if got := storedKeys(t, b, nf.ID)["ExportLevel"]; got != "API" {
		t.Errorf("ExportLevel = %#v, want the authored API", got)
	}
	if got, err := b.GetNanoflow(nf.ID); err != nil || got.ExportLevel != "API" {
		t.Errorf("read back ExportLevel = %q (%v), want API", got.ExportLevel, err)
	}
}

// An authored level wins over the stored one, in both directions: Hidden over
// API alone cannot tell a writer that honours the clause from one that writes
// the "Hidden" constant (the #816 lesson). Unauthored still carries the stored
// level — TestUpdateNanoflow_CarriesStoredHeaderKeys.
func TestUpdateNanoflow_AuthoredExportLevelWins(t *testing.T) {
	b, nf := nanoflowFixture(t)
	for _, tc := range []struct{ stored, authored string }{
		{"Hidden", "API"},
		{"API", "Hidden"},
	} {
		setStoredKeys(t, b, nf.ID, bson.D{{Key: "ExportLevel", Value: tc.stored}})
		nf.ExportLevel = tc.authored
		if err := b.UpdateNanoflow(nf); err != nil {
			t.Fatalf("UpdateNanoflow: %v", err)
		}
		if got := storedKeys(t, b, nf.ID)["ExportLevel"]; got != tc.authored {
			t.Errorf("stored %s, authored %s: ExportLevel = %#v", tc.stored, tc.authored, got)
		}
	}
}
