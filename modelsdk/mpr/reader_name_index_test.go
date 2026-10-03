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

// A name is not unique: a module may hold an excluded twin of a document
// (#914). The lookup must return the live one whichever order the headers come
// in, and still resolve when every match is excluded.
func TestGetUnitByName_PrefersLiveTwin(t *testing.T) {
	for _, excludedFirst := range []bool{true, false} {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "twins.mpr"))
		if err != nil {
			t.Fatal(err)
		}
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
		rootID := "00000000-0000-0000-0000-000000000021"
		moduleID := "00000000-0000-0000-0000-000000000022"
		liveID := "00000000-0000-0000-0000-000000000023"
		excludedID := "00000000-0000-0000-0000-000000000024"
		onlyExcludedID := "00000000-0000-0000-0000-000000000025"
		insertNamedUnit(t, db, moduleID, rootID, "Projects$ModuleImpl", "Sales")
		if excludedFirst {
			insertExcludedUnit(t, db, excludedID, moduleID, "Microflows$Microflow", "Calc")
			insertNamedUnit(t, db, liveID, moduleID, "Microflows$Microflow", "Calc")
		} else {
			insertNamedUnit(t, db, liveID, moduleID, "Microflows$Microflow", "Calc")
			insertExcludedUnit(t, db, excludedID, moduleID, "Microflows$Microflow", "Calc")
		}
		insertExcludedUnit(t, db, onlyExcludedID, moduleID, "Microflows$Microflow", "Old")

		r := &Reader{db: db, version: MPRVersionV1}
		unit, err := r.GetUnitByName("microflow", "Sales.Calc")
		if err != nil {
			t.Fatalf("GetUnitByName: %v", err)
		}
		if unit == nil || unit.ID != liveID {
			t.Fatalf("excludedFirst=%v: got %#v, want the live twin %s", excludedFirst, unit, liveID)
		}
		old, err := r.GetUnitByName("microflow", "Sales.Old")
		if err != nil {
			t.Fatalf("GetUnitByName: %v", err)
		}
		if old == nil || old.ID != onlyExcludedID {
			t.Fatalf("an all-excluded name must still resolve; got %#v", old)
		}
		_ = db.Close()
	}
}

func insertExcludedUnit(t *testing.T, db *sql.DB, id, containerID, unitType, name string) {
	t.Helper()
	contents, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: unitType},
		{Key: "Name", Value: name},
		{Key: "Excluded", Value: true},
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
}

// The container tree behind the name index answers "which module is this in"
// and "which containers make up this module" without re-reading unit contents.
func TestHeaderHierarchy_ModuleAndContainers(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tree.mpr"))
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
	rootID := "00000000-0000-0000-0000-000000000031"
	salesID := "00000000-0000-0000-0000-000000000032"
	otherID := "00000000-0000-0000-0000-000000000033"
	folderID := "00000000-0000-0000-0000-000000000034"
	nestedID := "00000000-0000-0000-0000-000000000035"
	otherFolderID := "00000000-0000-0000-0000-000000000036"
	mappingID := "00000000-0000-0000-0000-000000000037"
	insertNamedUnit(t, db, rootID, rootID, "Projects$Project", "")
	insertNamedUnit(t, db, salesID, rootID, "Projects$ModuleImpl", "Sales")
	insertNamedUnit(t, db, otherID, rootID, "Projects$ModuleImpl", "Other")
	insertNamedUnit(t, db, folderID, salesID, "Projects$Folder", "Integration")
	insertNamedUnit(t, db, nestedID, folderID, "Projects$Folder", "Import")
	insertNamedUnit(t, db, otherFolderID, otherID, "Projects$Folder", "Misc")
	insertNamedUnit(t, db, mappingID, nestedID, "ImportMappings$ImportMapping", "IMM_Order")

	r := &Reader{db: db, version: MPRVersionV1}
	for id, want := range map[string]string{mappingID: "Sales", nestedID: "Sales", otherFolderID: "Other", rootID: ""} {
		got, err := r.ModuleNameOf(id)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("ModuleNameOf(%s) = %q, want %q", id, got, want)
		}
	}

	set, err := r.ContainersInModule(salesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 3 || !set[salesID] || !set[folderID] || !set[nestedID] {
		t.Fatalf("ContainersInModule(Sales) = %v, want the module and its two nested folders", set)
	}
	set[otherID] = true // callers own the result; it must not leak into the next one
	if again, _ := r.ContainersInModule(salesID); again[otherID] {
		t.Fatal("ContainersInModule returned a shared map")
	}
}

// Once the index is built, ModuleNameOf and ContainersInModule read no unit
// contents: with mprcontents/ gone they still answer. The backend's by-name
// lookups call them once per candidate document, and re-reading every unit on
// each call is what made them cost seconds over a describe sweep.
func TestHeaderHierarchy_ReadsNoContentsOnceBuilt(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "tree.mpr"))
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
	rootID := "00000000-0000-0000-0000-000000000041"
	moduleID := "00000000-0000-0000-0000-000000000042"
	folderID := "00000000-0000-0000-0000-000000000043"
	docID := "00000000-0000-0000-0000-000000000044"
	insertV2NamedUnit(t, db, contentsDir, moduleID, rootID, "Projects$ModuleImpl", "Sales")
	insertV2NamedUnit(t, db, contentsDir, folderID, moduleID, "Projects$Folder", "Integration")
	insertV2NamedUnit(t, db, contentsDir, docID, folderID, "ImportMappings$ImportMapping", "IMM_Order")

	r := &Reader{db: db, version: MPRVersionV2, contentsDir: contentsDir}
	if _, err := r.ModuleNameOf(docID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(contentsDir); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ModuleNameOf(docID); err != nil || got != "Sales" {
		t.Fatalf("ModuleNameOf after removing contents = %q, %v; want Sales", got, err)
	}
	if set, err := r.ContainersInModule(moduleID); err != nil || !set[folderID] {
		t.Fatalf("ContainersInModule after removing contents = %v, %v", set, err)
	}
}
