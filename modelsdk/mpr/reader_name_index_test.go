// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

func TestGetUnitByName_IndexesHeadersAndLoadsOneUnit(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "index.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT,
			Contents BLOB
		)
	`); err != nil {
		t.Fatal(err)
	}

	rootID := "00000000-0000-0000-0000-000000000001"
	moduleID := "00000000-0000-0000-0000-000000000002"
	folderID := "00000000-0000-0000-0000-000000000003"
	firstID := "00000000-0000-0000-0000-000000000004"
	secondID := "00000000-0000-0000-0000-000000000005"

	insertNamedUnit(t, db, moduleID, rootID, "Projects$ModuleImpl", "Sales")
	insertNamedUnit(t, db, folderID, moduleID, "Projects$Folder", "Processes")
	firstContents := insertNamedUnit(t, db, firstID, folderID, "Microflows$Microflow", "First")
	insertNamedUnit(t, db, secondID, moduleID, "Microflows$Microflow", "Second")

	r := &Reader{db: db, version: MPRVersionV1}
	unit, err := r.GetUnitByName("microflow", "Sales.First")
	if err != nil {
		t.Fatalf("GetUnitByName: %v", err)
	}
	if unit == nil {
		t.Fatal("GetUnitByName returned nil")
	}
	if unit.ID != firstID || unit.ContainerID != folderID {
		t.Fatalf("unit = (%s, %s), want (%s, %s)", unit.ID, unit.ContainerID, firstID, folderID)
	}
	if string(unit.Contents) != string(firstContents) {
		t.Fatal("direct lookup returned the wrong BSON contents")
	}

	missing, err := r.GetUnitByName("microflow", "Sales.Missing")
	if err != nil {
		t.Fatalf("missing lookup: %v", err)
	}
	if missing != nil {
		t.Fatalf("missing lookup = %#v, want nil", missing)
	}
}

func TestGetUnitByName_V2RetainsOnlyHeaders(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "index.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT
		)
	`); err != nil {
		t.Fatal(err)
	}

	contentsDir := filepath.Join(root, "mprcontents")
	rootID := "00000000-0000-0000-0000-000000000011"
	moduleID := "00000000-0000-0000-0000-000000000012"
	folderID := "00000000-0000-0000-0000-000000000013"
	microflowID := "00000000-0000-0000-0000-000000000014"
	insertV2NamedUnit(t, db, contentsDir, moduleID, rootID, "Projects$ModuleImpl", "Sales")
	insertV2NamedUnit(t, db, contentsDir, folderID, moduleID, "Projects$Folder", "Processes")
	wantContents := insertV2NamedUnit(t, db, contentsDir, microflowID, folderID, "Microflows$Microflow", "Run")

	r := &Reader{db: db, version: MPRVersionV2, contentsDir: contentsDir}
	r.EnableContentCache()
	unit, err := r.GetUnitByName("microflow", "Sales.Run")
	if err != nil {
		t.Fatalf("GetUnitByName: %v", err)
	}
	if unit == nil || unit.ID != microflowID || unit.ContainerID != folderID {
		t.Fatalf("unit = %#v", unit)
	}
	if string(unit.Contents) != string(wantContents) {
		t.Fatal("direct V2 lookup returned the wrong contents")
	}
	if len(r.contentCache) != 1 {
		t.Fatalf("content cache contains %d units, want only the selected unit", len(r.contentCache))
	}
	foundHeader := false
	for _, header := range r.unitCache {
		if header.Type == "Microflows$Microflow" {
			foundHeader = true
			if header.Name != "Run" {
				t.Fatalf("cached microflow header name = %q", header.Name)
			}
		}
	}
	if !foundHeader {
		t.Fatal("microflow header was not indexed")
	}
}

func insertNamedUnit(t *testing.T, db *sql.DB, id, containerID, unitType, name string) []byte {
	t.Helper()
	contents, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: unitType},
		{Key: "Name", Value: name},
		// A large nested payload would remain raw during index construction.
		{Key: "Payload", Value: bson.D{{Key: "Nested", Value: "value"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, '', ?)`,
		uuidToBlob(id), uuidToBlob(containerID), contents,
	); err != nil {
		t.Fatal(err)
	}
	return contents
}

func insertV2NamedUnit(t *testing.T, db *sql.DB, contentsDir, id, containerID, unitType, name string) []byte {
	t.Helper()
	contents, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: unitType},
		{Key: "Name", Value: name},
		{Key: "Payload", Value: bson.D{{Key: "Nested", Value: "value"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO Unit (UnitID, ContainerID, ContainmentName) VALUES (?, ?, '')`,
		uuidToBlob(id), uuidToBlob(containerID),
	); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(contentsDir, id[:2], id[2:4])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".mxunit"), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	return contents
}
