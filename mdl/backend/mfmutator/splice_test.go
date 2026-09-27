// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// These tests run the splice on hand-built documents, for the shapes the
// Studio Pro fixture does not have (loops, error handlers, vertical flows).
// The acceptance test on a Studio Pro-drawn flow is in the executor package
// (cmd_alter_flow_pedapp_test.go).

// uid returns a deterministic element id for a short name.
func uid(name string) string {
	b := make([]byte, 16)
	copy(b, name)
	return types.BlobToUUID(b)
}

func bin(name string) primitive.Binary {
	return primitive.Binary{Subtype: 0, Data: types.UUIDToBlob(uid(name))}
}

func obj(name, typ string, x, y int) bson.D {
	w, h := 120, 60
	if typ != "Microflows$ActionActivity" && typ != "Microflows$LoopedActivity" {
		w, h = 20, 20
	}
	return bson.D{
		{Key: "$ID", Value: bin(name)},
		{Key: "$Type", Value: typ},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;%d", x, y)},
		{Key: "Size", Value: fmt.Sprintf("%d;%d", w, h)},
	}
}

func flow(name, from, to string, fromSide, toSide int32, isErr bool) bson.D {
	return bson.D{
		{Key: "$ID", Value: bin(name)},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "CaseValues", Value: bson.A{int32(2), bson.D{{Key: "$ID", Value: bin(name + "c")}, {Key: "$Type", Value: "Microflows$NoCase"}}}},
		{Key: "DestinationConnectionIndex", Value: toSide},
		{Key: "DestinationPointer", Value: bin(to)},
		{Key: "IsErrorHandler", Value: isErr},
		{Key: "Line", Value: bson.D{
			{Key: "$ID", Value: bin(name + "l")},
			{Key: "$Type", Value: "Microflows$BezierCurve"},
			{Key: "DestinationControlVector", Value: "-30;0"},
			{Key: "OriginControlVector", Value: "30;0"},
		}},
		{Key: "OriginConnectionIndex", Value: fromSide},
		{Key: "OriginPointer", Value: bin(from)},
	}
}

func unit(objects []bson.D, flows []bson.D) bson.D {
	objs := bson.A{int32(3)}
	for _, o := range objects {
		objs = append(objs, o)
	}
	fl := bson.A{int32(3)}
	for _, f := range flows {
		fl = append(fl, f)
	}
	return bson.D{
		{Key: "$ID", Value: bin("unit")},
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Flows", Value: fl},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("oc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: objs},
		}},
	}
}

// fakeDeps serializes the way the codec does for the properties the splice
// reads, and records what was saved.
type fakeDeps struct{ saved []byte }

func (d *fakeDeps) SerializeObject(o microflows.MicroflowObject) (bson.D, error) {
	typ := "Microflows$ActionActivity"
	if _, ok := o.(*microflows.ExclusiveMerge); ok {
		typ = "Microflows$ExclusiveMerge"
	}
	p := o.GetPosition()
	sz := objectSize(o)
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Data: types.UUIDToBlob(string(o.GetID()))}},
		{Key: "$Type", Value: typ},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;%d", p.X, p.Y)},
		{Key: "Size", Value: fmt.Sprintf("%d;%d", sz.X, sz.Y)},
	}, nil
}

func (d *fakeDeps) SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error) {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.ID))}},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "DestinationConnectionIndex", Value: int32(f.DestinationConnectionIndex)},
		{Key: "DestinationPointer", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.DestinationID))}},
		{Key: "IsErrorHandler", Value: f.IsErrorHandler},
		{Key: "Line", Value: bson.D{
			{Key: "DestinationControlVector", Value: f.DestinationControlVector},
			{Key: "OriginControlVector", Value: f.OriginControlVector},
		}},
		{Key: "OriginConnectionIndex", Value: int32(f.OriginConnectionIndex)},
		{Key: "OriginPointer", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.OriginID))}},
	}, nil
}

func (d *fakeDeps) SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error) {
	return bson.D{
		{Key: "$ID", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.ID))}},
		{Key: "$Type", Value: "Microflows$AnnotationFlow"},
		{Key: "DestinationPointer", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.DestinationID))}},
		{Key: "OriginPointer", Value: primitive.Binary{Data: types.UUIDToBlob(string(f.OriginID))}},
	}, nil
}

func (d *fakeDeps) SaveUnit(_ string, contents []byte) error {
	d.saved = contents
	return nil
}

func newMutator(t *testing.T, doc bson.D) (*Mutator, *fakeDeps) {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	deps := &fakeDeps{}
	m, err := New(d, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	return m, deps
}

// oneActivity is a single-activity fragment in builder coordinates.
func oneActivity() *backend.MicroflowFragment {
	id := model.ID(types.GenerateID())
	act := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: id},
		Position:    model.Point{X: 360, Y: 200},
		Size:        model.Size{Width: 120, Height: 60},
	}}}
	return &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{act}, Entry: id, Exit: id}
}

func line() []bson.D {
	return []bson.D{
		obj("start", "Microflows$StartEvent", 100, 200),
		obj("a", "Microflows$ActionActivity", 250, 200),
		obj("b", "Microflows$ActionActivity", 420, 200),
		obj("end", "Microflows$EndEvent", 600, 200),
	}
}

func lineFlows() []bson.D {
	return []bson.D{
		flow("f1", "start", "a", 1, 3, false),
		flow("f2", "a", "b", 1, 3, false),
		flow("f3", "b", "end", 1, 3, false),
	}
}

// The control every splice test leans on: decoding a unit and encoding it
// again with nothing spliced gives back the stored bytes exactly.
func TestSplice_UntouchedUnitRoundTripsExactly(t *testing.T) {
	raw, _ := bson.Marshal(unit(line(), lineFlows()))
	m, _ := newMutator(t, unit(line(), lineFlows()))
	out, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("an unmodified unit did not round-trip byte for byte")
	}
}

func TestSplice_InsertAfterKeepsTheErrorHandlerFlow(t *testing.T) {
	objs := append(line(), obj("handler", "Microflows$ActionActivity", 250, 350))
	flows := append(lineFlows(), flow("err", "a", "handler", 2, 0, true))
	m, _ := newMutator(t, unit(objs, flows))
	errBefore, _ := bson.Marshal(flow("err", "a", "handler", 2, 0, true))

	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	g := m.graph()
	for _, f := range g.flows {
		switch f.id {
		case uid("err"):
			got, _ := bson.Marshal(f.doc)
			if !bytes.Equal(got, errBefore) {
				t.Error("the error-handler flow changed")
			}
		case uid("f2"):
			if f.dest != string(frag.Entry) {
				t.Error("the normal flow out of a was not rewired to the fragment")
			}
		}
	}
}

func TestSplice_VerticalFlowUsesTopAndBottom(t *testing.T) {
	objs := []bson.D{
		obj("a", "Microflows$ActionActivity", 200, 100),
		obj("b", "Microflows$ActionActivity", 200, 200),
		obj("below", "Microflows$ActionActivity", 200, 300),
		obj("beside", "Microflows$ActionActivity", 500, 100),
	}
	flows := []bson.D{flow("f", "a", "b", 2, 0, false), flow("g", "b", "below", 2, 0, false)}
	m, _ := newMutator(t, unit(objs, flows))
	frag := oneActivity()
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	g := m.graph()
	if p := g.nodes[uid("b")].pos; p.X != 200 || p.Y <= 200 {
		t.Errorf("b should have moved down, is at %v", p)
	}
	if p := g.nodes[uid("beside")].pos; p.X != 500 || p.Y != 100 {
		t.Errorf("an object above the cut moved to %v", p)
	}
	for _, f := range g.flows {
		if f.id == uid("f") {
			if idx, _ := flowEnd(f.doc, "Destination"); idx != sideTop {
				t.Errorf("the rewired flow enters the fragment on side %d, want top", idx)
			}
		}
		if f.origin == string(frag.Exit) {
			if idx, _ := flowEnd(f.doc, "Origin"); idx != sideBottom {
				t.Errorf("the new flow leaves the fragment on side %d, want bottom", idx)
			}
		}
	}
}

func TestSplice_Refusals(t *testing.T) {
	loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
	loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
		{Key: "$ID", Value: bin("loopoc")},
		{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
		{Key: "Objects", Value: bson.A{int32(3), obj("inner", "Microflows$ActionActivity", 100, 60), obj("inner2", "Microflows$ActionActivity", 260, 60)}},
	}})
	withLoop := []bson.D{obj("start", "Microflows$StartEvent", 100, 200), obj("a", "Microflows$ActionActivity", 250, 200), loop}
	loopFlows := []bson.D{flow("f1", "start", "a", 1, 3, false), flow("f2", "a", "loop", 1, 3, false), flow("fi", "inner", "inner2", 1, 3, false)}

	twoIn := append(line(), obj("c", "Microflows$ActionActivity", 250, 350))
	twoInFlows := append(lineFlows(), flow("f4", "c", "b", 0, 2, false))

	withHandler := append(line(), obj("handler", "Microflows$ActionActivity", 250, 350))
	handlerFlows := append(lineFlows(), flow("err", "a", "handler", 2, 0, true))

	cases := []struct {
		name  string
		objs  []bson.D
		flows []bson.D
		op    func(m *Mutator) error
		want  string
	}{
		{"insert inside a loop", withLoop, loopFlows,
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("inner")), oneActivity()) }, "inside a loop"},
		{"drop inside a loop", withLoop, loopFlows,
			func(m *Mutator) error { return m.Drop(model.ID(uid("inner"))) }, "inside a loop"},
		{"insert before a join", twoIn, twoInFlows,
			func(m *Mutator) error { return m.InsertBefore(model.ID(uid("b")), oneActivity()) }, "2 flows enter"},
		{"insert after the end", line(), lineFlows(),
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("end")), oneActivity()) }, "ends the flow"},
		{"drop an activity with an error handler", withHandler, handlerFlows,
			func(m *Mutator) error { return m.Drop(model.ID(uid("a"))) }, "error handler"},
		{"replace an activity with an error handler", withHandler, handlerFlows,
			func(m *Mutator) error { return m.Replace(model.ID(uid("a")), oneActivity()) }, "error handler"},
		{"drop the end event", line(), lineFlows(),
			func(m *Mutator) error { return m.Drop(model.ID(uid("end"))) }, "cannot drop"},
		{"an empty fragment", line(), lineFlows(),
			func(m *Mutator) error { return m.InsertAfter(model.ID(uid("a")), &backend.MicroflowFragment{}) }, "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newMutator(t, unit(tc.objs, tc.flows))
			before, _ := bson.Marshal(m.doc)
			err := tc.op(m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			after, _ := bson.Marshal(m.doc)
			if !bytes.Equal(before, after) {
				t.Error("a refused operation changed the document")
			}
		})
	}
}

// CLAUDE.md rule 1: an element is never removed while anything still points
// at it. A pointer the splice does not know about (here, a made-up property
// on another object) is found by value, and the write is refused.
func TestSplice_DropRefusesADanglingReference(t *testing.T) {
	objs := line()
	objs[3] = append(objs[3], bson.E{Key: "SomePointer", Value: bin("a")})
	m, deps := newMutator(t, unit(objs, lineFlows()))
	if err := m.Drop(model.ID(uid("a"))); err != nil {
		t.Fatalf("drop: %v", err)
	}
	err := m.Save()
	if err == nil || !strings.Contains(err.Error(), "still points at a removed element") {
		t.Fatalf("want the dangling-reference refusal, got %v", err)
	}
	if deps.saved != nil {
		t.Error("a unit with a dangling reference was saved")
	}
}

// Control for the test above: the same drop without the stray pointer saves.
func TestSplice_DropJoinsTheFlows(t *testing.T) {
	m, deps := newMutator(t, unit(line(), lineFlows()))
	if err := m.Drop(model.ID(uid("a"))); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(uid("a"))) {
		t.Error("the dropped activity's $ID is still in the unit")
	}
	g := m.graph()
	for _, f := range g.flows {
		if f.id == uid("f1") && f.dest != uid("b") {
			t.Errorf("the flow into the dropped activity now enters %s, want b", f.dest)
		}
		if f.id == uid("f2") {
			t.Error("the flow out of the dropped activity is still there")
		}
	}
}

// A loop's body flows are stored in the unit's Flows list, not in the loop.
// Dropping or replacing the loop takes them with it; left behind, they point at
// the removed body objects and Save refuses the unit (a Studio Pro-authored
// loop with two body activities, ACT_ConflictedWorkflowHelper_ApplyJumpTo in
// TestApp, hit exactly that).
func TestSplice_DropOrReplaceALoopTakesItsBodyFlows(t *testing.T) {
	build := func() ([]bson.D, []bson.D) {
		loop := obj("loop", "Microflows$LoopedActivity", 420, 200)
		loop = append(loop, bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("loopoc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), obj("inner", "Microflows$ActionActivity", 100, 60), obj("inner2", "Microflows$ActionActivity", 260, 60)}},
		}})
		objs := []bson.D{
			obj("start", "Microflows$StartEvent", 100, 200),
			obj("a", "Microflows$ActionActivity", 250, 200),
			loop,
			obj("end", "Microflows$EndEvent", 700, 200),
		}
		flows := []bson.D{
			flow("f1", "start", "a", 1, 3, false),
			flow("f2", "a", "loop", 1, 3, false),
			flow("fi", "inner", "inner2", 1, 3, false),
			flow("f3", "loop", "end", 1, 3, false),
		}
		return objs, flows
	}
	ops := map[string]func(m *Mutator) error{
		"drop":    func(m *Mutator) error { return m.Drop(model.ID(uid("loop"))) },
		"replace": func(m *Mutator) error { return m.Replace(model.ID(uid("loop")), oneActivity()) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			objs, flows := build()
			m, deps := newMutator(t, unit(objs, flows))
			if err := op(m); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if err := m.Save(); err != nil {
				t.Fatalf("save: %v", err)
			}
			for _, gone := range []string{"loop", "inner", "inner2", "fi"} {
				if bytes.Contains(deps.saved, types.UUIDToBlob(uid(gone))) {
					t.Errorf("%s is still in the unit", gone)
				}
			}
		})
	}
}
