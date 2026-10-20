// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1476: no MDL form bound the Image widget to an uploaded
// image — a System.Image specialisation in the data view's context.
//
// The pluggable Image widget's `imageObject` is image-typed, and an image-typed
// value holds EITHER an image collection entry (its `Image` key, a by-name
// reference) OR an object from the context (its `EntityRef`). There is no
// Studio Pro sample of the second in any project this repo carries, so the
// shape was settled against mxbuild 11.12.1 on a page with a data view over
// `MyFirstModule.Photo extends System.Image`:
//
//	EntityRef = DirectEntityRef{Entity: MyFirstModule.Photo}  → 0 errors
//	EntityRef = DirectEntityRef{Entity: System.User}          → CE7247 "Entity should be
//	                                                             the same as the enclosing …"
//	EntityRef = DirectEntityRef{Entity: MyFirstModule.Note}    → CE7247 "Widget expects
//	  (a data view over a non-image entity)                      entity of type System.Image."
//	neither Image nor EntityRef                                → "No image selected."
//
// The two refusals are the evidence that mxbuild READS the key rather than
// ignoring an unknown one.

func imageValueWithEntityRef() bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "EntityRef", Value: nil},
		{Key: "Image", Value: "Mod.Coll.Stale"},
		{Key: "PrimitiveValue", Value: ""},
	}
}

func TestSetImageEntityValue_WritesADirectEntityRef(t *testing.T) {
	got := setImageEntityValue(imageValueWithEntityRef(), "MyFirstModule.Photo")

	ref, ok := get(got, "EntityRef").(bson.D)
	if !ok {
		t.Fatalf("EntityRef = %#v, want a DirectEntityRef document", get(got, "EntityRef"))
	}
	if typ := get(ref, "$Type"); typ != "DomainModels$DirectEntityRef" {
		t.Errorf("EntityRef $Type = %v, want DomainModels$DirectEntityRef", typ)
	}
	if ent := get(ref, "Entity"); ent != "MyFirstModule.Photo" {
		t.Errorf("EntityRef Entity = %v, want MyFirstModule.Photo", ent)
	}
	if get(ref, "$ID") == nil {
		t.Error("EntityRef has no $ID")
	}
}

// The two sources are exclusive: a collection entry left beside the binding
// would leave the widget saying two different things about what it shows.
func TestSetImageEntityValue_ClearsTheCollectionEntry(t *testing.T) {
	if v := get(setImageEntityValue(imageValueWithEntityRef(), "M.Photo"), "Image"); v != "" {
		t.Errorf("Image = %v, want it cleared", v)
	}
}

// Key set and order are untouched — a gained or lost key is CE0463.
func TestSetImageEntityValue_KeepsTheKeySet(t *testing.T) {
	in := imageValueWithEntityRef()
	got := setImageEntityValue(in, "M.Photo")
	if len(got) != len(in) {
		t.Fatalf("key count changed: %d -> %d", len(in), len(got))
	}
	for i := range in {
		if got[i].Key != in[i].Key {
			t.Errorf("key %d: %q -> %q", i, in[i].Key, got[i].Key)
		}
	}
}

// No entity, no change — "the script did not say" never overwrites.
func TestSetImageEntityValue_EmptyEntityIsANoOp(t *testing.T) {
	got := setImageEntityValue(imageValueWithEntityRef(), "")
	if get(got, "EntityRef") != nil || get(got, "Image") != "Mod.Coll.Stale" {
		t.Errorf("value changed for an empty entity: %v", got)
	}
}
