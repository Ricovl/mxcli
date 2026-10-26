// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1695: LinkNote draws a line from a note already in the flow to an
// object, oriented by where the two are drawn — a loop body's object relative
// to its loop box — and draws none where one is already.

// sidesDeps records the note lines the mutator serializes, sides included,
// which fakeDeps does not write.
type sidesDeps struct {
	*fakeDeps
	lines []*microflows.AnnotationFlow
}

func (d *sidesDeps) SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error) {
	d.lines = append(d.lines, f)
	return d.fakeDeps.SerializeAnnotationFlow(f)
}

// noteLinkUnit is a loop at (420, 200), 600×300 (its box's top-left corner
// at (120, 50)), holding "inner" 200×60 at (100, 60) relative to that corner —
// (220, 110) on the canvas — and a note "note" 200×30 at (700, 115).
func noteLinkUnit(extraFlows ...bson.D) bson.D {
	inner := obj("inner", "Microflows$ActionActivity", 100, 60)
	dSet(inner, "Size", "200;60")
	loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
	dSet(loop, "Size", "600;300")
	loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
		{Key: "$ID", Value: bin("loopoc")},
		{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
		{Key: "Objects", Value: bson.A{int32(3), inner}},
	}})
	note := obj("note", "Microflows$Annotation", 700, 115)
	dSet(note, "Size", "200;30")
	return unit([]bson.D{obj("start", "Microflows$StartEvent", 50, 200), loop, note},
		append([]bson.D{flow("f1", "start", "loop", 1, 3, false)}, extraFlows...))
}

func newSidesMutator(t *testing.T, doc bson.D) (*Mutator, *sidesDeps) {
	t.Helper()
	m, fd := newMutator(t, doc)
	deps := &sidesDeps{fakeDeps: fd}
	m.deps = deps
	return m, deps
}

func TestLinkNote_FacesTheObjectWhereItIsDrawn(t *testing.T) {
	m, deps := newSidesMutator(t, noteLinkUnit())
	if err := m.LinkNote(model.ID(uid("note")), model.ID(uid("inner"))); err != nil {
		t.Fatal(err)
	}
	if len(deps.lines) != 1 {
		t.Fatalf("want one line drawn, got %d", len(deps.lines))
	}
	// On the canvas the note (y 100..130) is level with inner (y 80..140) and
	// right of it, so the line runs from the note's left to inner's right.
	// Read in the loop's own coordinates (y 30..90), the note would be below.
	if s := deps.lines[0].SidesOrDefault(); s.Origin != 3 || s.Destination != 1 {
		t.Errorf("want the note's left side to inner's right side, got origin %d, destination %d", s.Origin, s.Destination)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestLinkNote_DrawsNoSecondLine(t *testing.T) {
	m, deps := newSidesMutator(t, noteLinkUnit(annotationFlow("al", "note", "inner")))
	if err := m.LinkNote(model.ID(uid("note")), model.ID(uid("inner"))); err != nil {
		t.Fatal(err)
	}
	if len(deps.lines) != 0 {
		t.Errorf("a line is drawn already, want none added, got %d", len(deps.lines))
	}
}

func TestLinkNote_RefusesWhatIsNotANote(t *testing.T) {
	m, _ := newSidesMutator(t, noteLinkUnit())
	if err := m.LinkNote(model.ID(uid("start")), model.ID(uid("inner"))); err == nil {
		t.Error("a line from an object that is not a note must be refused")
	}
	if err := m.LinkNote(model.ID(uid("note")), model.ID(uid("gone"))); err == nil {
		t.Error("a line to an object that is not in the flow must be refused")
	}
}
