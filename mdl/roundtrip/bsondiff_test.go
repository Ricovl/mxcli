// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A re-minted GUID is the one change this fixture exists to catch (CLAUDE.md,
// "A GUID Is the Database's Identity"), and it is binary like the $IDs and
// pointers bsonDiff skips. The failure message must name it, or a fixer reads a
// dropped database table as "$ID-shaped noise".
func TestBSONDiffNamesAChangedGUID(t *testing.T) {
	unit := func(id, guid byte) []byte {
		b, err := bson.Marshal(bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 3, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "DomainModels$DomainModel"},
			{Key: "Entities", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: bson.Binary{Subtype: 3, Data: []byte{id, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}},
				{Key: "GUID", Value: bson.Binary{Subtype: 3, Data: []byte{guid, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}}},
				{Key: "Name", Value: "Account"},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	paths := bsonDiff(unit(1, 1), unit(1, 2))
	if !strings.Contains(strings.Join(paths, "\n"), "(Account)/GUID") {
		t.Fatalf("bsonDiff does not name the changed GUID; got %q", paths)
	}
	// Control: an $ID-only change stays silent, as before — a rebuild
	// renumbers $IDs and canon judges that separately.
	if paths := bsonDiff(unit(1, 1), unit(2, 1)); len(paths) != 0 {
		t.Fatalf("an $ID-only change was reported as a content change: %q", paths)
	}
}
