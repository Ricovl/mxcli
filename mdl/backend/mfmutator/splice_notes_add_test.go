// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1517: a note added to a decision or an end event — nodes
// the splice changes in place, never replaces — was refused. AddNotes attaches
// the notes the builder drew against a stand-in to the stored node, and
// changes nothing else.

// notesFor is a stand-in at (360, 200) with one note drawn 100px above it, and
// a second whose canvas position is stated (pinned).
func notesFor() (*backend.MicroflowFragment, model.ID, model.ID) {
	frag := oneActivity()
	n1, n2 := model.ID(types.GenerateID()), model.ID(types.GenerateID())
	note := func(id model.ID, x, y int) *microflows.Annotation {
		return &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: id}, Position: model.Point{X: x, Y: y},
			Size: model.Size{Width: 200, Height: 50}}, Caption: "note"}
	}
	frag.Objects = append(frag.Objects, note(n1, 360, 100), note(n2, 40, 40))
	frag.AnnotationFlows = []*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())}, OriginID: n1, DestinationID: frag.Entry},
		{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())}, OriginID: n2, DestinationID: frag.Entry},
	}
	frag.Pinned = map[model.ID]bool{n2: true}
	return frag, n1, n2
}

func TestSplice_AddNotesAttachesToTheStoredNode(t *testing.T) {
	m, deps := newMutator(t, unit(line(), lineFlows()))
	frag, n1, n2 := notesFor()
	if err := m.AddNotes(model.ID(uid("end")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got bson.D
	if err := bson.Unmarshal(deps.saved, &got); err != nil {
		t.Fatal(err)
	}
	// The stored objects and flows are untouched and come first; the notes
	// and their lines are appended.
	objs := arrayElements(dGet(dDoc(got, "ObjectCollection"), "Objects"))
	if len(objs) != len(line())+2 {
		t.Fatalf("want the two notes appended, got %d objects", len(objs))
	}
	for i, want := range line() {
		w, _ := bson.Marshal(want)
		g, _ := bson.Marshal(objs[i])
		if !bytes.Equal(w, g) {
			t.Errorf("stored object %d changed", i)
		}
	}
	pos := map[string]string{}
	for _, el := range objs[len(line()):] {
		d := el.(bson.D)
		pos[binaryID(dGet(d, "$ID"))] = dString(d, "RelativeMiddlePoint")
	}
	// The end event is at (600, 200): the stand-in's note 100px above it moves
	// with it; the pinned one stays.
	if pos[string(n1)] != "600;100" || pos[string(n2)] != "40;40" {
		t.Errorf("note positions %v, want n1 at 600;100 and n2 at 40;40", pos)
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(string(frag.Entry))) {
		t.Error("the stand-in, or a line to it, was written")
	}
	lines := 0
	for _, el := range arrayElements(dGet(got, "Flows")) {
		d := el.(bson.D)
		if binaryID(dGet(d, "DestinationPointer")) == uid("end") && binaryID(dGet(d, "OriginPointer")) != uid("b") {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("want two note lines into the end event, got %d", lines)
	}
}

func TestSplice_AddNotesRefusesAnActivity(t *testing.T) {
	m, _ := newMutator(t, unit(line(), lineFlows()))
	frag := oneActivity()
	frag.Objects = append(frag.Objects, oneActivity().Objects...)
	before, _ := bson.Marshal(m.doc)
	err := m.AddNotes(model.ID(uid("end")), frag)
	if err == nil || !strings.Contains(err.Error(), "not a set of notes") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if after, _ := bson.Marshal(m.doc); !bytes.Equal(before, after) {
		t.Error("a refused AddNotes changed the document")
	}
}

// A free note is attached to nothing, so SetFreeNotes swaps the free notes and
// leaves every other element — an attached note and its line included — as
// stored.
func TestSplice_SetFreeNotesReplacesOnlyTheFreeNotes(t *testing.T) {
	objs := append(line(), obj("free", "Microflows$Annotation", 100, 50), obj("att", "Microflows$Annotation", 250, 100))
	flows := append(lineFlows(), annotationFlow("al", "att", "a"))
	m, deps := newMutator(t, unit(objs, flows))
	frag, n1, _ := notesFor()
	// Only the notes no line attaches are free: n1 and n2 both have lines in
	// notesFor, so give the fragment one more, unattached.
	free := model.ID(types.GenerateID())
	frag.Objects = append(frag.Objects, &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: free}, Position: model.Point{X: 700, Y: 40},
		Size: model.Size{Width: 200, Height: 50}}, Caption: "new free"})
	if err := m.SetFreeNotes(frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(uid("free"))) {
		t.Error("the stored free note is still there")
	}
	for _, kept := range []string{"att", "al", "a", "b"} {
		if !bytes.Contains(deps.saved, types.UUIDToBlob(uid(kept))) {
			t.Errorf("%s was removed", kept)
		}
	}
	if !bytes.Contains(deps.saved, types.UUIDToBlob(string(free))) {
		t.Error("the new free note was not written")
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(string(n1))) || bytes.Contains(deps.saved, types.UUIDToBlob(string(frag.Entry))) {
		t.Error("an attached note of the fragment, or its stand-in, was written")
	}
}
