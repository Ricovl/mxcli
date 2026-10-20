// SPDX-License-Identifier: Apache-2.0

package bsonutil

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestOrderStorageValue_IDFirstTypeSecondRestSorted(t *testing.T) {
	in := bson.M{
		"Name":  "X",
		"$Type": "Some$Type",
		"Apple": 1,
		"$ID":   "id-1",
		"Zebra": 2,
	}
	got, ok := OrderStorageValue(in).(bson.D)
	if !ok {
		t.Fatalf("expected bson.D, got %T", OrderStorageValue(in))
	}
	want := []string{"$ID", "$Type", "Apple", "Name", "Zebra"}
	if len(got) != len(want) {
		t.Fatalf("key count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, k := range want {
		if got[i].Key != k {
			t.Errorf("key[%d] = %q, want %q", i, got[i].Key, k)
		}
	}
}

func TestOrderStorageValue_RecursesNestedAndArrays(t *testing.T) {
	in := bson.M{
		"$ID":   "parent",
		"$Type": "Parent",
		"Child": bson.M{"Name": "c", "$Type": "Child", "$ID": "child"},
		"Kids": bson.A{
			int32(2), // versioned-array marker must stay first
			bson.M{"Field": "v", "$Type": "Kid", "$ID": "kid-0"},
		},
	}
	got := OrderStorageValue(in).(bson.D)

	if got[0].Key != "$ID" {
		t.Fatalf("parent[0] = %q, want $ID", got[0].Key)
	}

	// Nested Child document: $ID must be first.
	child := findKey(t, got, "Child").(bson.D)
	if child[0].Key != "$ID" {
		t.Errorf("Child[0] = %q, want $ID", child[0].Key)
	}

	// Array: marker preserved as element 0, nested doc ordered.
	kids := findKey(t, got, "Kids").(bson.A)
	if kids[0] != int32(2) {
		t.Errorf("Kids[0] = %v, want marker int32(2)", kids[0])
	}
	kid := kids[1].(bson.D)
	if kid[0].Key != "$ID" {
		t.Errorf("Kids[1][0] = %q, want $ID", kid[0].Key)
	}
}

func TestOrderStorageValue_MarshalsWithIDFirst(t *testing.T) {
	in := bson.M{"B": 1, "$Type": "T", "A": 2, "$ID": "id"}
	raw, err := bson.Marshal(OrderStorageValue(in))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d[0].Key != "$ID" {
		t.Errorf("on-the-wire first key = %q, want $ID", d[0].Key)
	}
}

func findKey(t *testing.T, d bson.D, key string) any {
	t.Helper()
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("key %q not found in %v", key, d)
	return nil
}

// TestHoistStorageID_PreservesOrderExceptID is the key distinction from
// OrderStorageValue: HoistStorageID lifts "$ID" first and "$Type" second but must
// NOT reorder the remaining keys. A blind sort (as OrderStorageValue does)
// corrupts template-derived pluggable-widget page trees, so the nightly's
// datagrid pages must round-trip with their widget field order intact.
func TestHoistStorageID_PreservesOrderExceptID(t *testing.T) {
	// A widget-shaped doc: $ID appears late, and the non-$ID keys are in a
	// deliberately non-alphabetical order that must be preserved.
	in := bson.D{
		{Key: "Name", Value: "w"},
		{Key: "LabelTemplate", Value: "lt"},
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "$ID", Value: "id-1"},
		{Key: "Attribute", Value: "Attr"},
	}
	got, ok := HoistStorageID(in).(bson.D)
	if !ok {
		t.Fatalf("HoistStorageID returned %T, want bson.D", HoistStorageID(in))
	}
	gotKeys := make([]string, len(got))
	for i, e := range got {
		gotKeys[i] = e.Key
	}
	// $ID first, $Type second, then the rest in ORIGINAL order (not sorted).
	want := []string{"$ID", "$Type", "Name", "LabelTemplate", "TabIndex", "Attribute"}
	if len(gotKeys) != len(want) {
		t.Fatalf("keys = %v, want %v", gotKeys, want)
	}
	for i := range want {
		if gotKeys[i] != want[i] {
			t.Errorf("key[%d] = %q, want %q (full: %v)", i, gotKeys[i], want[i], gotKeys)
		}
	}
}

// TestHoistStorageID_RecursesAndMarshalsIDFirst verifies nested objects are
// hoisted too and the marshalled bytes lead with $ID at every level.
func TestHoistStorageID_RecursesAndMarshalsIDFirst(t *testing.T) {
	in := bson.D{
		{Key: "Widget", Value: bson.D{
			{Key: "LabelTemplate", Value: "lt"},
			{Key: "$Type", Value: "T"},
			{Key: "$ID", Value: "child"},
		}},
		{Key: "$ID", Value: "root"},
	}
	raw, err := bson.Marshal(HoistStorageID(in))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d[0].Key != "$ID" {
		t.Errorf("root first key = %q, want $ID", d[0].Key)
	}
	child := findKey(t, d, "Widget").(bson.D)
	if child[0].Key != "$ID" {
		t.Errorf("child first key = %q, want $ID", child[0].Key)
	}
}

// TestHoistStorageID_MapFallbackHoistsID confirms a Go map (no inherent order)
// still comes out $ID-first after marshalling.
func TestHoistStorageID_MapFallbackHoistsID(t *testing.T) {
	in := map[string]any{"Zeta": 1, "LabelTemplate": 2, "$ID": "x", "$Type": "T"}
	raw, err := bson.Marshal(HoistStorageID(in))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d[0].Key != "$ID" || d[1].Key != "$Type" {
		t.Errorf("first two keys = %q,%q, want $ID,$Type", d[0].Key, d[1].Key)
	}
}
