// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ako/mxcli#1097: a project built by an older mxcli keeps every property-name
// gap in the units no later statement rewrites, and Mendix's merge engine
// throws on each one. CompleteStoredPropertySets is the pass that reaches them.

const (
	repairUnitID   = "77777777-7777-4777-8777-777777777777"
	repairEntityID = "88888888-8888-4888-8888-888888888888"
	// A Studio Pro-style GUID, deliberately not the entity's $ID: an element
	// mxcli created has GUID == $ID, and a GUID test on one detects nothing.
	repairEntityGUID = "99999999-9999-4999-8999-999999999999"
)

func pv(major, minor, patch int) *version.ProjectVersion {
	return &version.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
}

// moduleSettingsWithoutPackageKeys is a Projects$ModuleSettings as an older
// mxcli wrote it: none of the six keys Mendix 11.12+ serializes for it.
func moduleSettingsWithoutPackageKeys(t *testing.T) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: uuidToBlob(repairUnitID)}},
		{Key: "$Type", Value: "Projects$ModuleSettings"},
		{Key: "BasedOnVersion", Value: ""},
		{Key: "ExportLevel", Value: "Source"},
		{Key: "ExtensionName", Value: ""},
		{Key: "JarDependencies", Value: bson.A{int32(3)}},
		{Key: "ProtectedModuleType", Value: "AddOn"},
		{Key: "SolutionIdentifier", Value: ""},
		{Key: "Version", Value: "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// domainModelWithoutEntityKeys is a domain model whose entity lacks the keys
// the table adds to DomainModels$EntityImpl, and carries a GUID that is not its
// $ID — so the test sees whether the repair moves it.
func domainModelWithoutEntityKeys(t *testing.T) []byte {
	t.Helper()
	bin := func(id string) bson.Binary { return bson.Binary{Subtype: 0, Data: uuidToBlob(id)} }
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bin(repairUnitID)},
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "Entities", Value: bson.A{int32(3), bson.D{
			{Key: "$ID", Value: bin(repairEntityID)},
			{Key: "$Type", Value: "DomainModels$EntityImpl"},
			{Key: "GUID", Value: bin(repairEntityGUID)},
			{Key: "Name", Value: "Customer"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func topLevelKeys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range d {
		out[e.Key] = true
	}
	return out
}

var moduleSettingsPackageKeys = []string{
	"Checksum", "ConvertedChecksum", "EnableDetailedTroubleshooting",
	"ModuleDependencies", "OriginalPackageId", "PackageId",
}

// The issue's premise, measured: a rewrite does NOT elide a unit that lacks a
// table key. Normalization adds the key before the comparison, and canon.Equal
// compares strictly, so any rewrite completes the unit. The gap is reach — the
// units nothing rewrites — not elision, which is why the repair is a pass over
// every unit rather than a change to Reconcile.
func TestStoredUnitMissingAKeyIsCompletedByAnyRewrite(t *testing.T) {
	stored := moduleSettingsWithoutPackageKeys(t)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 12, 0)

	if err := w.UpdateRawUnit(repairUnitID, stored); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := topLevelKeys(t, after)
	for _, k := range moduleSettingsPackageKeys {
		if !keys[k] {
			t.Errorf("rewrite of the stored bytes did not add %s — it was elided", k)
		}
	}
}

func TestCompleteStoredPropertySets_RepairsAnUntouchedUnit(t *testing.T) {
	stored := moduleSettingsWithoutPackageKeys(t)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 12, 0)

	// Dry run: the report, and no write.
	gaps, err := w.reader.StoredPropertySetGaps(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if gaps.Added != len(moduleSettingsPackageKeys) || gaps.Stripped != 0 || gaps.Type != "Projects$ModuleSettings" {
		t.Fatalf("dry run = %+v, want %d added", gaps, len(moduleSettingsPackageKeys))
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, stored) {
		t.Fatal("the dry run wrote the unit")
	}

	rep, err := w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != len(moduleSettingsPackageKeys) || !rep.Changed() {
		t.Fatalf("repair = %+v", rep)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := topLevelKeys(t, first)
	for _, k := range moduleSettingsPackageKeys {
		if !keys[k] {
			t.Errorf("repaired unit has no %s", k)
		}
	}
	if _, id := bson.Raw(first).Lookup("$ID").Binary(); !bytes.Equal(id, uuidToBlob(repairUnitID)) {
		t.Error("the repair changed the unit's $ID")
	}
	if got, want := hashOf(first), storedHash(t, w, repairUnitID); got != want {
		t.Errorf("ContentsHash %s does not describe the file (%s)", want, got)
	}

	// Idempotent: the second run has nothing to do and writes nothing.
	rep, err = w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed() {
		t.Fatalf("second repair = %+v, want nothing", rep)
	}
	if second, _ := os.ReadFile(path); !bytes.Equal(second, first) {
		t.Fatal("the second repair rewrote the unit")
	}
}

// The repair adds names and nothing else: the entity keeps its $ID and,
// above all, its GUID — the database's identity for the table.
func TestCompleteStoredPropertySets_KeepsIdentities(t *testing.T) {
	stored := domainModelWithoutEntityKeys(t)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 13, 0)

	rep, err := w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added == 0 {
		t.Fatalf("repair = %+v, want the EntityImpl keys added", rep)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ent := bson.Raw(after).Lookup("Entities").Array().Index(1).Document()
	if _, got := ent.Lookup("$ID").Binary(); !bytes.Equal(got, uuidToBlob(repairEntityID)) {
		t.Errorf("entity $ID moved")
	}
	if _, got := ent.Lookup("GUID").Binary(); !bytes.Equal(got, uuidToBlob(repairEntityGUID)) {
		t.Errorf("entity GUID changed — the next deploy would drop its table")
	}
	if _, err := ent.LookupErr("Image"); err != nil {
		t.Errorf("entity was not completed: %v", err)
	}
}

// A project older than every table row's floor gets nothing: an undeclared key
// is worse than a missing one.
func TestCompleteStoredPropertySets_RespectsTheVersionFloor(t *testing.T) {
	stored := moduleSettingsWithoutPackageKeys(t)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 11, 0)

	rep, err := w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed() {
		t.Fatalf("repair on 11.11 = %+v, want nothing (the keys arrive in 11.12)", rep)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, stored) {
		t.Fatal("the unit was rewritten")
	}
}

// A key the version does not declare yet, holding nothing, is stripped and
// counted — the other half of the name-set difference.
func TestCompleteStoredPropertySets_StripsAnUndeclaredEmptyKey(t *testing.T) {
	bin := bson.Binary{Subtype: 0, Data: uuidToBlob(repairUnitID)}
	stored, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bin},
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "Name", Value: "Responsive"},
		{Key: "ThrowPartialSyncError", Value: false}, // since 11.12.0
	})
	if err != nil {
		t.Fatal(err)
	}
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 11, 0)

	rep, err := w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Stripped != 1 {
		t.Fatalf("repair = %+v, want 1 stripped", rep)
	}
	after, _ := os.ReadFile(path)
	if topLevelKeys(t, after)["ThrowPartialSyncError"] {
		t.Fatal("the undeclared key is still stored")
	}
}
