// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A note is not part of the room a spliced activity needs on its line
// (mendixlabs/mxcli#1598, case 1). Adding `@annotation(text: 'note D',
// position: (1500, 600))` to one statement made a Replace of it whose box ran
// from the activity to the note — 1300px — so every node after it shifted by
// 1180px and the note itself, translated with the box, landed at x 3390. A
// size-only note wider than its activity shifted the row by the difference.
func TestSplice_ReplaceWithANoteKeepsTheRow(t *testing.T) {
	fragment := func(notePos model.Point, noteSize model.Size, stated bool) (*backend.MicroflowFragment, model.ID, model.ID) {
		frag := oneActivity()
		note := &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    notePos,
			Size:        noteSize,
		}}
		frag.Objects = append(frag.Objects, note)
		frag.AnnotationFlows = []*microflows.AnnotationFlow{{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			OriginID:    note.ID, DestinationID: frag.Entry,
		}}
		if stated {
			frag.Pinned = map[model.ID]bool{note.ID: true}
		}
		return frag, frag.Entry, note.ID
	}
	positions := func(t *testing.T, m *Mutator) map[string]point {
		t.Helper()
		out := map[string]point{}
		for _, n := range m.graph().order {
			out[n.id] = n.pos
		}
		return out
	}

	t.Run("a note at a stated position", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		frag, act, note := fragment(model.Point{X: 1500, Y: 600}, model.Size{Width: 200, Height: 80}, true)
		if err := m.Replace(model.ID(uid("a")), frag); err != nil {
			t.Fatalf("replace: %v", err)
		}
		got := positions(t, m)
		if p := got[uid("b")]; p.X != 420 {
			t.Errorf("b moved to x=%d, want it left at 420", p.X)
		}
		if p := got[uid("end")]; p.X != 600 {
			t.Errorf("the end event moved to x=%d, want it left at 600", p.X)
		}
		if p := got[string(act)]; p.X != 250 || p.Y != 200 {
			t.Errorf("the replacement is at (%d, %d), want the replaced activity's (250, 200)", p.X, p.Y)
		}
		if p := got[string(note)]; p.X != 1500 || p.Y != 600 {
			t.Errorf("the note is at (%d, %d), want where the script put it, (1500, 600)", p.X, p.Y)
		}
	})

	t.Run("a size-only note wider than its activity", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		// Where the builder draws an unplaced note: above its activity.
		frag, act, note := fragment(model.Point{X: 360, Y: 100}, model.Size{Width: 400, Height: 50}, false)
		if err := m.Replace(model.ID(uid("a")), frag); err != nil {
			t.Fatalf("replace: %v", err)
		}
		got := positions(t, m)
		if p := got[uid("b")]; p.X != 420 {
			t.Errorf("b moved to x=%d, want it left at 420", p.X)
		}
		// The note travels with its activity.
		a, n := got[string(act)], got[string(note)]
		if n.X-a.X != 0 || a.Y-n.Y != 100 {
			t.Errorf("the note is at (%d, %d) for an activity at (%d, %d); want it 100px above it, as built",
				n.X, n.Y, a.X, a.Y)
		}
	})
}
