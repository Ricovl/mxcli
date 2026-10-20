// SPDX-License-Identifier: Apache-2.0

package bsoncompat

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// nestedDoc is a document holding a nested document and an array of documents,
// the shape every .mpr unit has.
func nestedDoc(t *testing.T) []byte {
	t.Helper()
	raw, err := bson.Marshal(bson.D{
		{Key: "Outer", Value: bson.D{
			{Key: "Inner", Value: bson.D{{Key: "Deep", Value: int32(1)}}},
		}},
		{Key: "Arr", Value: bson.A{bson.D{{Key: "K", Value: "v"}}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestDecodeShapes pins which nested Go type each destination produces. These
// are the shapes the v1 driver produced, which mxcli's type assertions were
// written against; the migration off v1 (ako/mxcli#1372 follow-up) keeps them
// only because of the helpers in this package.
//
// The middle case is the trap: a plain bson.Unmarshal into a bson.M nests a
// bson.D, so a `doc["Key"].(bson.M)` assertion silently stops matching and the
// property is dropped from the next write.
func TestDecodeShapes(t *testing.T) {
	raw := nestedDoc(t)

	t.Run("bson.D destination needs no helper", func(t *testing.T) {
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := d[0].Value.(bson.D); !ok {
			t.Errorf("nested in bson.D = %T, want bson.D", d[0].Value)
		}
	})

	t.Run("plain Unmarshal into bson.M nests bson.D (the trap)", func(t *testing.T) {
		var m bson.M
		if err := bson.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := m["Outer"].(bson.M); ok {
			t.Fatal("plain Unmarshal now nests bson.M in a bson.M — " +
				"the driver changed and UnmarshalDocM may no longer be needed; " +
				"re-measure before removing it")
		}
	})

	t.Run("UnmarshalDocM nests bson.M at every level", func(t *testing.T) {
		var m bson.M
		if err := UnmarshalDocM(raw, &m); err != nil {
			t.Fatalf("UnmarshalDocM: %v", err)
		}
		outer, ok := m["Outer"].(bson.M)
		if !ok {
			t.Fatalf("nested = %T, want bson.M", m["Outer"])
		}
		if _, ok := outer["Inner"].(bson.M); !ok {
			t.Errorf("nested-in-nested = %T, want bson.M", outer["Inner"])
		}
		arr, ok := m["Arr"].(bson.A)
		if !ok {
			t.Fatalf("array = %T, want bson.A", m["Arr"])
		}
		if _, ok := arr[0].(bson.M); !ok {
			t.Errorf("array element = %T, want bson.M", arr[0])
		}
	})

	t.Run("UnmarshalDocMap nests map[string]any, not bson.M", func(t *testing.T) {
		var m map[string]any
		if err := UnmarshalDocMap(raw, &m); err != nil {
			t.Fatalf("UnmarshalDocMap: %v", err)
		}
		// bson.M is a named type, so this assertion fails on a bson.M value —
		// which is exactly why UnmarshalDocM is not interchangeable here.
		outer, ok := m["Outer"].(map[string]any)
		if !ok {
			t.Fatalf("nested = %T, want map[string]any", m["Outer"])
		}
		if _, ok := outer["Inner"].(map[string]any); !ok {
			t.Errorf("nested-in-nested = %T, want map[string]any", outer["Inner"])
		}
	})

	t.Run("UnmarshalDocM is not a substitute for UnmarshalDocMap", func(t *testing.T) {
		var m map[string]any
		if err := UnmarshalDocM(raw, &m); err != nil {
			t.Fatalf("UnmarshalDocM: %v", err)
		}
		if _, ok := m["Outer"].(map[string]any); ok {
			t.Error("UnmarshalDocM nested a map[string]any; the two helpers have " +
				"converged and the distinction this package documents is stale")
		}
	})
}

func TestDMap(t *testing.T) {
	got := DMap(bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 2}})
	if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
		t.Errorf("DMap = %v, want a:1 b:2", got)
	}
	// Last value wins on a duplicate key, as v1's bson.D.Map() did.
	if got := DMap(bson.D{{Key: "a", Value: 1}, {Key: "a", Value: 2}}); got["a"] != 2 {
		t.Errorf("duplicate key = %v, want last value 2", got["a"])
	}
	if got := DMap(nil); len(got) != 0 {
		t.Errorf("DMap(nil) = %v, want empty", got)
	}
}
