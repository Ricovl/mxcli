// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1694: a decision of the shape `alter … insert` produces — two
// paths of plain activities, or none, rejoining at one merge — is taken out by
// a drop or a replace; any other decision is refused, saying which part of
// the shape is out of reach.

// caseFlow is flow with the case value an exclusive split's path carries.
func caseFlow(name, from, to string, fromSide, toSide int32, value string) bson.D {
	f := flow(name, from, to, fromSide, toSide, false)
	dSet(f, "CaseValues", bson.A{int32(2), bson.D{
		{Key: "$ID", Value: bin(name + "c")},
		{Key: "$Type", Value: "Microflows$EnumerationCase"},
		{Key: "Value", Value: value},
	}})
	return f
}

// decisionUnit is start -> a -> split -> (true: t1 | false: f1) -> merge -> b
// -> end, the shape `alter … insert` produces for `if … then … else … end if`.
// edit changes the objects and flows before the unit is built.
func decisionUnit(edit func(objs []bson.D, flows []bson.D) ([]bson.D, []bson.D)) bson.D {
	objs := []bson.D{
		obj("start", "Microflows$StartEvent", 100, 200),
		obj("a", "Microflows$ActionActivity", 250, 200),
		obj("split", "Microflows$ExclusiveSplit", 400, 200),
		obj("t1", "Microflows$ActionActivity", 550, 200),
		obj("f1", "Microflows$ActionActivity", 550, 300),
		obj("merge", "Microflows$ExclusiveMerge", 700, 200),
		obj("b", "Microflows$ActionActivity", 850, 200),
		obj("end", "Microflows$EndEvent", 1000, 200),
	}
	flows := []bson.D{
		flow("f1s", "start", "a", 1, 3, false),
		flow("fa", "a", "split", 1, 3, false),
		caseFlow("ft", "split", "t1", 1, 3, "true"),
		caseFlow("ff", "split", "f1", 2, 3, "false"),
		flow("ftm", "t1", "merge", 1, 3, false),
		flow("ffm", "f1", "merge", 1, 2, false),
		flow("fm", "merge", "b", 1, 3, false),
		flow("fb", "b", "end", 1, 3, false),
	}
	if edit != nil {
		objs, flows = edit(objs, flows)
	}
	return unit(objs, flows)
}

func TestSpliceDecision_DropTakesTheShapeOut(t *testing.T) {
	m, deps := newMutator(t, decisionUnit(nil))
	if got := m.DecisionNodes(model.ID(uid("split"))); len(got) != 2 {
		t.Fatalf("want the two path activities, got %v", got)
	}
	if err := m.Drop(model.ID(uid("split"))); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, _ := geometry(t, deps.saved)
	for _, gone := range []string{"split", "t1", "f1", "merge"} {
		if _, ok := pos[uid(gone)]; ok {
			t.Errorf("%s is still in the unit", gone)
		}
	}
	for _, kept := range []string{"start", "a", "b", "end"} {
		if _, ok := pos[uid(kept)]; !ok {
			t.Errorf("%s was taken out", kept)
		}
	}
	e := edges(t, deps.saved)
	if !e[[2]string{uid("a"), uid("b")}] || len(e) != 3 {
		t.Errorf("want start -> a -> b -> end, got %v", e)
	}
	// The flow a -> b is the stored a -> split flow, re-pointed: its $ID is
	// kept.
	if !bytes.Contains(deps.saved, bin("fa").Data) || bytes.Contains(deps.saved, bin("fm").Data) {
		t.Error("want the flow into the decision kept and the one out of its merge gone")
	}
}

func TestSpliceDecision_OnePathEmpty(t *testing.T) {
	// `if … then … end if`: the false path runs straight to the merge.
	m, deps := newMutator(t, decisionUnit(func(objs, flows []bson.D) ([]bson.D, []bson.D) {
		var o []bson.D
		for _, d := range objs {
			if binaryID(dGet(d, "$ID")) != uid("f1") {
				o = append(o, d)
			}
		}
		var f []bson.D
		for _, d := range flows {
			switch binaryID(dGet(d, "$ID")) {
			case uid("ff"):
				f = append(f, caseFlow("ff", "split", "merge", 2, 2, "false"))
			case uid("ffm"):
			default:
				f = append(f, d)
			}
		}
		return o, f
	}))
	if err := m.Drop(model.ID(uid("split"))); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if e := edges(t, deps.saved); !e[[2]string{uid("a"), uid("b")}] || len(e) != 3 {
		t.Errorf("want start -> a -> b -> end, got %v", e)
	}
}

func TestSpliceDecision_ReplaceTakesItsPlace(t *testing.T) {
	m, deps := newMutator(t, decisionUnit(nil))
	frag := oneActivity()
	if err := m.Replace(model.ID(uid("split")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	e := edges(t, deps.saved)
	id := string(frag.Entry)
	if !e[[2]string{uid("a"), id}] || !e[[2]string{id, uid("b")}] || len(e) != 4 {
		t.Errorf("want start -> a -> new -> b -> end, got %v", e)
	}
	pos, _ := geometry(t, deps.saved)
	// It starts where the split started (x 390), and fits in the span the
	// decision took (390 .. 710): nothing moves.
	if pos[id] != (point{450, 200}) || pos[uid("b")] != (point{850, 200}) {
		t.Errorf("the activity is at %v and b at %v, want 450;200 and 850;200", pos[id], pos[uid("b")])
	}
}

func TestSpliceDecision_OtherShapesRefused(t *testing.T) {
	without := func(drop ...string) func(objs, flows []bson.D) ([]bson.D, []bson.D) {
		return func(objs, flows []bson.D) ([]bson.D, []bson.D) {
			skip := map[string]bool{}
			for _, d := range drop {
				skip[uid(d)] = true
			}
			var o, f []bson.D
			for _, d := range objs {
				if !skip[binaryID(dGet(d, "$ID"))] {
					o = append(o, d)
				}
			}
			for _, d := range flows {
				if !skip[binaryID(dGet(d, "$ID"))] {
					f = append(f, d)
				}
			}
			return o, f
		}
	}
	plus := func(base func(objs, flows []bson.D) ([]bson.D, []bson.D), objs []bson.D, flows []bson.D) func(o, f []bson.D) ([]bson.D, []bson.D) {
		return func(o, f []bson.D) ([]bson.D, []bson.D) {
			if base != nil {
				o, f = base(o, f)
			}
			return append(o, objs...), append(f, flows...)
		}
	}
	cases := []struct {
		name string
		edit func(objs, flows []bson.D) ([]bson.D, []bson.D)
		want string
	}{
		{"a path that returns", plus(without("ffm"),
			[]bson.D{obj("ret", "Microflows$EndEvent", 700, 300)},
			[]bson.D{flow("fr", "f1", "ret", 1, 3, false)}), "its false path ends in a return"},
		{"a nested decision", plus(without("ffm"),
			[]bson.D{obj("inner", "Microflows$ExclusiveSplit", 650, 300)},
			[]bson.D{flow("fi", "f1", "inner", 1, 3, false), caseFlow("fi1", "inner", "merge", 1, 2, "true"), caseFlow("fi2", "inner", "merge", 2, 2, "false")}),
			"its false path holds a decision"},
		{"an error handler on a path", plus(nil,
			[]bson.D{obj("h", "Microflows$ActionActivity", 550, 450)},
			[]bson.D{flow("fh", "t1", "h", 2, 0, true), flow("fhm", "h", "merge", 1, 2, false)}),
			"has an error handler"},
		{"a path from elsewhere joins the merge", plus(nil,
			[]bson.D{obj("x", "Microflows$ActionActivity", 550, 450)},
			[]bson.D{flow("fx", "x", "merge", 1, 2, false)}),
			"3 flows join at its merge"},
		{"three paths", plus(nil,
			[]bson.D{obj("x", "Microflows$ActionActivity", 550, 100)},
			[]bson.D{caseFlow("fx", "split", "x", 0, 3, "other"), flow("fxm", "x", "merge", 1, 0, false)}),
			"it has 3 paths"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, verb := range []string{"drop", "replace"} {
				m, _ := newMutator(t, decisionUnit(tc.edit))
				before, _ := bson.Marshal(m.doc)
				var err error
				if verb == "drop" {
					err = m.Drop(model.ID(uid("split")))
				} else {
					err = m.Replace(model.ID(uid("split")), oneActivity())
				}
				if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "cannot "+verb) {
					t.Fatalf("%s: want a refusal saying %q, got %v", verb, tc.want, err)
				}
				after, _ := bson.Marshal(m.doc)
				if !bytes.Equal(before, after) {
					t.Errorf("%s: a refused operation changed the document", verb)
				}
				if m.DecisionNodes(model.ID(uid("split"))) != nil {
					t.Errorf("DecisionNodes of a refused shape must be nil")
				}
			}
		})
	}
}

// A note on an activity of the decision's paths goes with it; one also
// attached to a node that stays keeps that line, and stays.
func TestSpliceDecision_PathNotes(t *testing.T) {
	note := func(name string, x, y int) bson.D { return obj(name, "Microflows$Annotation", x, y) }
	line := func(name, from, to string) bson.D {
		return bson.D{
			{Key: "$ID", Value: bin(name)},
			{Key: "$Type", Value: "Microflows$AnnotationFlow"},
			{Key: "DestinationPointer", Value: bin(to)},
			{Key: "OriginPointer", Value: bin(from)},
		}
	}
	m, deps := newMutator(t, decisionUnit(func(objs, flows []bson.D) ([]bson.D, []bson.D) {
		return append(objs, note("own", 550, 100), note("shared", 700, 100)),
			append(flows, line("l1", "own", "t1"), line("l2", "shared", "f1"), line("l3", "shared", "b"))
	}))
	if err := m.Drop(model.ID(uid("split"))); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	pos, _ := geometry(t, deps.saved)
	if _, ok := pos[uid("own")]; ok {
		t.Error("the note on a path activity stayed behind")
	}
	if _, ok := pos[uid("shared")]; !ok {
		t.Error("the note also attached to b was taken out")
	}
	if !bytes.Contains(deps.saved, bin("l3").Data) || bytes.Contains(deps.saved, bin("l2").Data) {
		t.Error("want the shared note's line to b kept and its line to f1 gone")
	}
}
