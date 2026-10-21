// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A note's line leaves the side of the note that faces its element and enters
// the side of the element that faces the note (mendixlabs/mxcli#1598, case 2).
// The writer stored 2 -> 0 (note bottom -> element top) for every note, so a
// note placed BELOW its activity drew its line out of its own bottom edge and
// round the outside of both boxes.
//
// The expected sides are Studio Pro's, measured rather than assumed: of 523
// distinct Studio Pro-authored AnnotationFlows (ako/TestApp, PedApp and nine
// customer apps), the element's side follows "vertically separated -> top or
// bottom, else left or right" in 513 (98%), and the numbering is the activity
// one (0 top, 1 right, 2 bottom, 3 left): a note level with its activity on the
// left stores 1 -> 3, on the right 3 -> 1. The issue's guess that a note is
// numbered "top 0, bottom 1, left 2, right 3" does not survive those two.
func TestAnnotationFlowLeavesTheFacingSide(t *testing.T) {
	cases := []struct {
		name        string
		note        string
		origin, dst int
	}{
		{"above", "(400, 80)", AnchorBottom, AnchorTop},
		{"below", "(400, 360)", AnchorTop, AnchorBottom},
		{"left", "(120, 200)", AnchorRight, AnchorLeft},
		{"right", "(680, 200)", AnchorLeft, AnchorRight},
		// Diagonal: vertically clear of the activity, so the vertical sides win,
		// as they do in Studio Pro's own drawings of a note above and to one side.
		{"above-left", "(150, 60)", AnchorBottom, AnchorTop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, `
  @position(400, 200)
  @annotation(text: 'note', position: `+tc.note+`, size: (200, 60))
  log info node 'A' 'a';`)
			if len(fb.annotationFlows) != 1 {
				t.Fatalf("got %d annotation flows, want 1", len(fb.annotationFlows))
			}
			got := fb.annotationFlows[0].Sides
			if got == nil {
				t.Fatalf("note %s: no sides chosen; the writer falls back to %d -> %d",
					tc.note, AnchorBottom, AnchorTop)
			}
			if got.Origin != tc.origin || got.Destination != tc.dst {
				t.Errorf("note %s of an activity at (400, 200): line %d -> %d, want %d -> %d",
					tc.note, got.Origin, got.Destination, tc.origin, tc.dst)
			}
		})
	}
}

// A note on an activity INSIDE a loop is wired from the parent collection, and
// the two positions are in different coordinate planes: the body's are
// relative to the loop box. Compared raw, a note above the loop would read as
// below an activity whose body-relative y is small.
func TestAnnotationFlowSidesAcrossALoopBoundary(t *testing.T) {
	note := &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: "note"},
		Position:    model.Point{X: 500, Y: 400},
		Size:        model.Size{Width: 200, Height: 60},
	}}
	inner := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: "inner"},
		Position:    model.Point{X: 200, Y: 80}, // relative to the loop's top-left
		Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
	}}}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "loop"},
			Position:    model.Point{X: 500, Y: 200},
			Size:        model.Size{Width: 400, Height: 160}, // top-left (300, 120)
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{inner}},
	}
	af := &microflows.AnnotationFlow{OriginID: "note", DestinationID: "inner"}
	orientAnnotationFlows([]microflows.MicroflowObject{note, loop}, []*microflows.AnnotationFlow{af})
	// The inner activity is at absolute (500, 200); the note, at y=400, is below it.
	if af.Sides == nil || af.Sides.Origin != AnchorTop || af.Sides.Destination != AnchorBottom {
		t.Fatalf("sides %+v, want %d -> %d (note below the loop-body activity)", af.Sides, AnchorTop, AnchorBottom)
	}
}

// A note whose position the script states is recorded, so a spliced fragment
// keeps it where it is instead of moving it with the fragment
// (mendixlabs/mxcli#1598, case 1); a note placed by the layout is not.
func TestStatedNotesAreRecordedForTheSplice(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  @annotation(text: 'stated', position: (1500, 600), size: (200, 80))
  log info node 'A' 'a';
  @annotation(text: 'placed', size: (200, 80))
  log info node 'B' 'b';`)
	stated := map[string]bool{}
	for _, o := range fb.objects {
		if n, ok := o.(*microflows.Annotation); ok {
			stated[n.Caption] = fb.statedNotes[n.ID]
		}
	}
	if !stated["stated"] || stated["placed"] {
		t.Errorf("recorded as stated: %v, want only the note with a position", stated)
	}
}
