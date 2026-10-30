// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// ako/mxcli#1790: the incremental source index decides what changed from
// UnitContentHashes, so a hash must follow the contents. On MPR v1 the stored
// ContentsHash cannot be trusted — mxcli wrote v1 units without updating it
// before ako/mxcli#1159 — and the hash is computed from the blob.
func TestUnitContentHashes_FollowTheContents(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		f := newTypedListingFixture(t, []string{"Microflows$Microflow", "Forms$Page"})
		id := v1UnitID(1)
		// A stale stored hash, as an older mxcli left it.
		if _, err := f.db.Exec(`UPDATE Unit SET ContentsHash = 'stale' WHERE UnitID = ?`, uuidToBlob(id)); err != nil {
			t.Fatal(err)
		}
		before, err := f.r.UnitContentHashes()
		if err != nil {
			t.Fatal(err)
		}
		if len(before) != 2 {
			t.Fatalf("hashed %d units, want 2", len(before))
		}
		key := blobToUUID(uuidToBlob(id))
		if before[key] == "stale" {
			t.Fatalf("the v1 hash is the stored column, not the contents")
		}
		if _, err := f.db.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`,
			v1UnitContents(t, 1, "Microflows$Microflow", 100), uuidToBlob(id)); err != nil {
			t.Fatal(err)
		}
		after, err := f.r.UnitContentHashes()
		if err != nil {
			t.Fatal(err)
		}
		if after[key] == before[key] {
			t.Errorf("an edited v1 unit kept its hash %q", after[key])
		}
	})
	t.Run("v2", func(t *testing.T) {
		p := newV2Project(t)
		id := p.add(t, 1, "Microflows$Microflow")
		p.add(t, 2, "Forms$Page")
		r := p.reader()
		before, err := r.UnitContentHashes()
		if err != nil {
			t.Fatal(err)
		}
		if len(before) != 2 {
			t.Fatalf("hashed %d units, want 2", len(before))
		}
		if reads := r.UnitFileReads(); reads != 0 {
			t.Errorf("hashing a v2 project opened %d unit files; the Unit table has the hashes", reads)
		}
		key := blobToUUID(uuidToBlob(id))
		p.rewrite(t, id, typedDoc(t, "Microflows$Microflow", "Renamed"))
		after, err := r.UnitContentHashes()
		if err != nil {
			t.Fatal(err)
		}
		if after[key] == before[key] {
			t.Errorf("a rewritten v2 unit kept its hash %q", after[key])
		}
	})
}
