// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1693: inside a loop body the splice inserts, drops and replaces
// plain activities, laid out among the body's nodes in the loop box's
// coordinates; the box grows on its far side when the body no longer fits,
// and what is drawn past it moves along.

// loopUnit is start -> loop -> end, the loop drawn as mxcli's layout draws it
// (centre 500;200, 640x160, so its box runs from x=180 to x=820) around a body
// n1 -> n2 -> n3 at y=80 in the box's coordinates.
func loopUnit(body ...string) bson.D {
	if len(body) == 0 {
		body = []string{"n1", "n2", "n3"}
	}
	objs := bson.A{int32(3)}
	var flows []bson.D
	for i, n := range body {
		objs = append(objs, obj(n, "Microflows$ActionActivity", 210+160*i, 80))
		if i > 0 {
			flows = append(flows, flow("b"+n, body[i-1], n, 1, 3, false))
		}
	}
	loop := obj("loop", "Microflows$LoopedActivity", 500, 200)
	dSet(loop, "Size", "640;160")
	loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
		{Key: "$ID", Value: bin("loopoc")},
		{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
		{Key: "Objects", Value: objs},
	}})
	return unit(
		[]bson.D{obj("start", "Microflows$StartEvent", 60, 200), loop, obj("end", "Microflows$EndEvent", 900, 200)},
		append([]bson.D{flow("fs", "start", "loop", 1, 3, false), flow("fe", "loop", "end", 1, 3, false)}, flows...),
	)
}

// geometry returns every object's position and size by its $ID, after a save.
func geometry(t *testing.T, saved []byte) (map[string]point, map[string]point) {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(saved, &d); err != nil {
		t.Fatal(err)
	}
	m, err := New(d, "unit", &fakeDeps{})
	if err != nil {
		t.Fatal(err)
	}
	pos, size := map[string]point{}, map[string]point{}
	for _, n := range m.graph().order {
		pos[n.id], size[n.id] = n.pos, n.size
	}
	return pos, size
}

// edges returns the unit's sequence flows as origin->destination $ID pairs.
func edges(t *testing.T, saved []byte) map[[2]string]bool {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(saved, &d); err != nil {
		t.Fatal(err)
	}
	m, _ := New(d, "unit", &fakeDeps{})
	out := map[[2]string]bool{}
	for _, f := range m.graph().flows {
		if f.isSequence() {
			out[[2]string{f.origin, f.dest}] = true
		}
	}
	return out
}

func TestSpliceLoop_InsertInTheMiddleGrowsTheBox(t *testing.T) {
	m, deps := newMutator(t, loopUnit())
	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("n1")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, size := geometry(t, deps.saved)
	id := string(frag.Entry)
	// 40px between n1 and n2; the activity and a gap either side need 200.
	if pos[id] != (point{370, 80}) {
		t.Errorf("the activity is drawn at %v, want 370;80 between n1 and the moved n2", pos[id])
	}
	if pos[uid("n2")] != (point{530, 80}) || pos[uid("n3")] != (point{690, 80}) {
		t.Errorf("n2 and n3 did not move along by 160: %v %v", pos[uid("n2")], pos[uid("n3")])
	}
	if pos[uid("n1")] != (point{210, 80}) {
		t.Errorf("n1 moved: %v", pos[uid("n1")])
	}
	// The box grows on its far side by what the body grew: its left edge,
	// and so the flow into it, stay; the end event moves along.
	if size[uid("loop")] != (point{800, 160}) || pos[uid("loop")] != (point{580, 200}) {
		t.Errorf("the loop box is %v at %v, want 800;160 at 580;200", size[uid("loop")], pos[uid("loop")])
	}
	if pos[uid("end")] != (point{1060, 200}) {
		t.Errorf("the end event is at %v, want 1060;200", pos[uid("end")])
	}
	e := edges(t, deps.saved)
	if !e[[2]string{uid("n1"), id}] || !e[[2]string{id, uid("n2")}] || e[[2]string{uid("n1"), uid("n2")}] {
		t.Errorf("the body flow does not run through the activity: %v", e)
	}
}

func TestSpliceLoop_EndsOfTheBody(t *testing.T) {
	t.Run("after the last", func(t *testing.T) {
		m, deps := newMutator(t, loopUnit())
		frag := oneActivity()
		if err := m.InsertAfter(model.ID(uid("n3")), frag); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		pos, _ := geometry(t, deps.saved)
		if pos[string(frag.Entry)] != (point{690, 80}) {
			t.Errorf("the activity is drawn at %v, want 690;80 after n3", pos[string(frag.Entry)])
		}
		if !edges(t, deps.saved)[[2]string{uid("n3"), string(frag.Entry)}] {
			t.Error("no flow from n3 into the activity")
		}
	})
	t.Run("before the first", func(t *testing.T) {
		m, deps := newMutator(t, loopUnit())
		frag := oneActivity()
		if err := m.InsertBefore(model.ID(uid("n1")), frag); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		pos, _ := geometry(t, deps.saved)
		if pos[string(frag.Entry)] != (point{210, 80}) || pos[uid("n1")] != (point{370, 80}) {
			t.Errorf("want the activity where n1 was and n1 moved along, got %v and %v", pos[string(frag.Entry)], pos[uid("n1")])
		}
		if !edges(t, deps.saved)[[2]string{string(frag.Entry), uid("n1")}] {
			t.Error("no flow from the activity into n1")
		}
	})
	for _, tc := range []struct {
		name, target string
		gone         [][2]string
		kept         [][2]string
	}{
		{"drop the first", "n1", [][2]string{{"n1", "n2"}}, [][2]string{{"n2", "n3"}}},
		{"drop the last", "n3", [][2]string{{"n2", "n3"}}, [][2]string{{"n1", "n2"}}},
		{"drop the middle", "n2", [][2]string{{"n1", "n2"}, {"n2", "n3"}}, [][2]string{{"n1", "n3"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, deps := newMutator(t, loopUnit())
			if err := m.Drop(model.ID(uid(tc.target))); err != nil {
				t.Fatal(err)
			}
			if err := m.Save(); err != nil {
				t.Fatal(err)
			}
			e := edges(t, deps.saved)
			for _, g := range tc.gone {
				if e[[2]string{uid(g[0]), uid(g[1])}] {
					t.Errorf("flow %v is still there", g)
				}
			}
			for _, k := range tc.kept {
				if !e[[2]string{uid(k[0]), uid(k[1])}] {
					t.Errorf("flow %v is missing", k)
				}
			}
			pos, size := geometry(t, deps.saved)
			if _, ok := pos[uid(tc.target)]; ok {
				t.Errorf("%s is still in the unit", tc.target)
			}
			if size[uid("loop")] != (point{640, 160}) {
				t.Errorf("a drop resized the loop box to %v", size[uid("loop")])
			}
		})
	}
	t.Run("drop the only one", func(t *testing.T) {
		m, deps := newMutator(t, loopUnit("n1"))
		if err := m.Drop(model.ID(uid("n1"))); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		if pos, _ := geometry(t, deps.saved); len(pos) != 3 {
			t.Errorf("want start, loop and end left, got %d objects", len(pos))
		}
	})
}

func TestSpliceLoop_ReplaceByTwo(t *testing.T) {
	m, deps := newMutator(t, loopUnit())
	frag := oneActivity()
	second := oneActivity()
	second.Objects[0].SetPosition(model.Point{X: 520, Y: 200})
	frag.Objects = append(frag.Objects, second.Objects...)
	frag.Flows = []*microflows.SequenceFlow{{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID: frag.Entry, DestinationID: second.Entry, OriginConnectionIndex: 1, DestinationConnectionIndex: 3}}
	frag.Exit = second.Exit
	if err := m.Replace(model.ID(uid("n2")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, size := geometry(t, deps.saved)
	if pos[string(frag.Entry)] != (point{370, 80}) || pos[string(second.Entry)] != (point{530, 80}) {
		t.Errorf("the two activities are at %v and %v, want 370;80 and 530;80", pos[string(frag.Entry)], pos[string(second.Entry)])
	}
	if pos[uid("n3")] != (point{690, 80}) {
		t.Errorf("n3 did not move along by the extra 160: %v", pos[uid("n3")])
	}
	if size[uid("loop")].X != 800 {
		t.Errorf("the loop box did not grow to fit: %v", size[uid("loop")])
	}
	e := edges(t, deps.saved)
	if !e[[2]string{uid("n1"), string(frag.Entry)}] || !e[[2]string{string(second.Exit), uid("n3")}] {
		t.Errorf("the body flow does not run through the fragment: %v", e)
	}
}

// A shift the script's stated positions forbid is refused, inside a body as
// at top level (PinInLoop), and so is a box that has to grow past a node the
// script draws where it is.
func TestSpliceLoop_PinnedRefusals(t *testing.T) {
	t.Run("a pinned body node", func(t *testing.T) {
		m, _ := newMutator(t, loopUnit())
		_ = m.PinInLoop(model.ID(uid("loop")), model.Point{X: 370, Y: 80})
		err := m.InsertAfter(model.ID(uid("n1")), oneActivity())
		if err == nil || !strings.Contains(err.Error(), "draws where it is") {
			t.Fatalf("want the pinned refusal, got %v", err)
		}
	})
	t.Run("a pinned node past the loop", func(t *testing.T) {
		m, _ := newMutator(t, loopUnit())
		_ = m.Pin(model.Point{X: 900, Y: 200})
		err := m.InsertAfter(model.ID(uid("n3")), oneActivity())
		if err == nil || !strings.Contains(err.Error(), "draws where it is") {
			t.Fatalf("want the pinned refusal, got %v", err)
		}
	})
}

// ReplaceLoopBody keeps the loop — its $ID, position and size where the new
// body fits — and replaces everything inside it, flows included.
func TestSpliceLoop_ReplaceBody(t *testing.T) {
	newBody := func(n int) *backend.MicroflowFragment {
		loop := &microflows.LoopedActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Size:        model.Size{Width: 200 + 160*n, Height: 160},
			},
			ObjectCollection: &microflows.MicroflowObjectCollection{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())}},
		}
		frag := &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{loop}, Entry: loop.ID, Exit: loop.ID}
		var prev model.ID
		for i := 0; i < n; i++ {
			a := oneActivity().Objects[0]
			a.SetPosition(model.Point{X: 210 + 160*i, Y: 80})
			loop.ObjectCollection.Objects = append(loop.ObjectCollection.Objects, a)
			if prev != "" {
				frag.Flows = append(frag.Flows, &microflows.SequenceFlow{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					OriginID: prev, DestinationID: a.GetID(), OriginConnectionIndex: 1, DestinationConnectionIndex: 3})
			}
			prev = a.GetID()
		}
		return frag
	}
	t.Run("a body that fits keeps the box", func(t *testing.T) {
		m, deps := newMutator(t, loopUnit())
		if err := m.ReplaceLoopBody(model.ID(uid("loop")), newBody(2)); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		pos, size := geometry(t, deps.saved)
		if pos[uid("loop")] != (point{500, 200}) || size[uid("loop")] != (point{640, 160}) {
			t.Errorf("the loop moved or was resized: %v %v", pos[uid("loop")], size[uid("loop")])
		}
		for _, n := range []string{"n1", "n2", "n3"} {
			if _, ok := pos[uid(n)]; ok {
				t.Errorf("stored body node %s survived", n)
			}
		}
		if len(pos) != 5 {
			t.Errorf("want start, loop, end and the two new activities, got %d objects", len(pos))
		}
		if e := edges(t, deps.saved); !e[[2]string{uid("start"), uid("loop")}] || !e[[2]string{uid("loop"), uid("end")}] {
			t.Errorf("the flows around the loop changed: %v", e)
		}
	})
	t.Run("a body that does not fit grows the box", func(t *testing.T) {
		m, deps := newMutator(t, loopUnit())
		if err := m.ReplaceLoopBody(model.ID(uid("loop")), newBody(5)); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		pos, size := geometry(t, deps.saved)
		if size[uid("loop")] != (point{1000, 160}) || pos[uid("loop")] != (point{680, 200}) {
			t.Errorf("the loop box is %v at %v, want 1000;160 at 680;200", size[uid("loop")], pos[uid("loop")])
		}
		if pos[uid("end")] != (point{1260, 200}) {
			t.Errorf("the end event did not move along: %v", pos[uid("end")])
		}
	})
	t.Run("a note outside attached inside is refused", func(t *testing.T) {
		doc := loopUnit()
		objs := arrayElements(dGet(dDoc(doc, "ObjectCollection"), "Objects"))
		objs = append(objs, obj("note", "Microflows$Annotation", 500, 20))
		setArray(dDoc(doc, "ObjectCollection"), "Objects", objs)
		fl := arrayElements(dGet(doc, "Flows"))
		fl = append(fl, bson.D{
			{Key: "$ID", Value: bin("af")},
			{Key: "$Type", Value: "Microflows$AnnotationFlow"},
			{Key: "DestinationPointer", Value: bin("n2")},
			{Key: "OriginPointer", Value: bin("note")},
		})
		setArray(doc, "Flows", fl)
		m, _ := newMutator(t, doc)
		err := m.ReplaceLoopBody(model.ID(uid("loop")), newBody(1))
		if err == nil || !strings.Contains(err.Error(), "note outside the loop") {
			t.Fatalf("want the outside-note refusal, got %v", err)
		}
	})
}
