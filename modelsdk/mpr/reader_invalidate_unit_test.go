// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"fmt"
	"sort"
	"testing"
)

// unitCacheSnapshot is the unit cache as sorted lines, order-independent.
func unitCacheSnapshot(t *testing.T, r *Reader) []string {
	t.Helper()
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			t.Fatal(err)
		}
	}
	out := make([]string, 0, len(r.unitCache))
	for _, cu := range r.unitCache {
		out = append(out, fmt.Sprintf("%+v", cu))
	}
	sort.Strings(out)
	return out
}

// TestInvalidateUnit_MatchesFreshBuild writes through the writer — rename,
// move, insert, delete — and after each write requires the patched unit cache
// to equal the one a freshly opened reader builds.
func TestInvalidateUnit_MatchesFreshBuild(t *testing.T) {
	path, _ := copyV2Fixture(t)
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	r := w.reader
	if _, err := r.ListModules(); err != nil {
		t.Fatal(err)
	}

	var folder, doc cachedUnit
	for _, cu := range r.unitCache {
		if cu.Type == "Projects$Folder" && folder.ID == "" && len(cu.Name) > 1 {
			folder = cu
		}
		if cu.Type == "Microflows$Microflow" && doc.ID == "" {
			doc = cu
		}
	}
	if folder.ID == "" || doc.ID == "" {
		t.Fatal("fixture has no folder or microflow")
	}

	check := func(step string) {
		t.Helper()
		fresh, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer fresh.Close()
		got, want := unitCacheSnapshot(t, r), unitCacheSnapshot(t, fresh)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: patched unit cache (%d units) differs from a fresh build (%d units)", step, len(got), len(want))
		}
	}

	// Rename the folder: same-length name, last letter swapped.
	b, err := r.GetRawUnitBytes(folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	renamed := bytes.Replace(b, []byte(folder.Name+"\x00"), []byte(folder.Name[:len(folder.Name)-1]+"Q\x00"), 1)
	if err := w.UpdateRawUnit(folder.ID, renamed); err != nil {
		t.Fatalf("rename folder: %v", err)
	}
	check("rename")

	if err := w.MoveUnit(doc.ID, folder.ID); err != nil {
		t.Fatalf("move: %v", err)
	}
	check("move")

	// Insert a clone of the folder under a new ID.
	const cloneID = "0b0b0b0b-1111-2222-3333-444444444444"
	clone := bytes.Replace(renamed, uuidToBlob(folder.ID), uuidToBlob(cloneID), 1)
	if bytes.Equal(clone, renamed) {
		t.Fatal("folder bytes do not hold its $ID as a blob")
	}
	if err := w.InsertUnit(cloneID, folder.ContainerID, folder.ContainmentName, "Projects$Folder", clone); err != nil {
		t.Fatalf("insert: %v", err)
	}
	check("insert")

	if err := w.DeleteUnit(cloneID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	check("delete")
}
