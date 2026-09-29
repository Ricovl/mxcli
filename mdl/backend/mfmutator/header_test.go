// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#818: header properties, parameters and stated positions are
// patched onto the stored document, never rebuilt.

func param(name, typ string, x int) bson.D {
	return bson.D{
		{Key: "$ID", Value: bin("p-" + name)},
		{Key: "$Type", Value: parameterType},
		{Key: "Documentation", Value: "doc of " + name},
		{Key: "Name", Value: name},
		{Key: "VariableType", Value: bson.D{{Key: "$ID", Value: bin("t-" + name)}, {Key: "$Type", Value: typ}}},
		{Key: "RelativeMiddlePoint", Value: strconv.Itoa(x) + ";53"},
		{Key: "Size", Value: "30;30"},
	}
}

// headerUnit is a stored microflow with header properties and parameters.
func headerUnit(params ...bson.D) bson.D {
	objs := append(append([]bson.D(nil), params...), line()...)
	u := unit(objs, lineFlows())
	return append(u,
		bson.E{Key: "Documentation", Value: "stored doc"},
		bson.E{Key: "ExportLevel", Value: "Hidden"},
		bson.E{Key: "MarkAsUsed", Value: true},
		bson.E{Key: "MicroflowReturnType", Value: bson.D{{Key: "$ID", Value: bin("rt")}, {Key: "$Type", Value: "DataTypes$VoidType"}}},
		bson.E{Key: "Name", Value: "Flow"},
	)
}

// declaredDoc is what the encoder produces for a declared header: fresh $IDs
// everywhere, and the properties the statement does not state at whatever the
// build carried or defaulted (MarkAsUsed false here, which must not be copied).
func declaredDoc(exportLevel, rt string, params ...bson.D) bson.D {
	objs := bson.A{int32(3)}
	for _, p := range params {
		objs = append(objs, p)
	}
	return bson.D{
		{Key: "$ID", Value: bin("fresh-unit")},
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Documentation", Value: "stored doc"},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "MarkAsUsed", Value: false},
		{Key: "MicroflowReturnType", Value: bson.D{{Key: "$ID", Value: bin("fresh-rt")}, {Key: "$Type", Value: rt}}},
		{Key: "Name", Value: "Flow"},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: bin("fresh-oc")}, {Key: "$Type", Value: "Microflows$MicroflowObjectCollection"}, {Key: "Objects", Value: objs}}},
	}
}

// freshParam is a declared parameter as the encoder writes it: new $IDs.
func freshParam(name, typ string, x int) bson.D {
	p := param(name, typ, x)
	p[0].Value = bin("fresh-p-" + name)
	p[2].Value = ""
	p[4].Value = bson.D{{Key: "$ID", Value: bin("fresh-t-" + name)}, {Key: "$Type", Value: typ}}
	return p
}

// The control: the declared header equal to the stored one changes nothing,
// byte for byte — including MarkAsUsed, which the declared side has at a
// different value but a statement cannot state.
func TestSetHeader_UnchangedWritesNothing(t *testing.T) {
	stored := headerUnit(param("A", "DataTypes$StringType", 200))
	raw, _ := bson.Marshal(stored)
	m, _ := newMutator(t, stored)
	changed, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType", freshParam("A", "DataTypes$StringType", 200)))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed %v, want nothing", changed)
	}
	out, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("an unchanged header rewrote the unit")
	}
}

func TestSetHeader_ChangesOnlyWhatDiffers(t *testing.T) {
	stored := headerUnit(param("A", "DataTypes$StringType", 200))
	m, _ := newMutator(t, stored)
	changed, err := m.SetHeader(declaredDoc("API", "DataTypes$StringType", freshParam("A", "DataTypes$IntegerType", 200)))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(changed, ","); got != "MicroflowReturnType,ExportLevel,parameter $A type" {
		t.Fatalf("changed %q", got)
	}
	if v := dString(m.doc, "ExportLevel"); v != "API" {
		t.Errorf("ExportLevel = %q", v)
	}
	if v, _ := dGet(m.doc, "MarkAsUsed").(bool); !v {
		t.Error("MarkAsUsed, which no statement states, was overwritten")
	}
	g := m.graph()
	p := g.nodes[uid("p-A")]
	if p == nil {
		t.Fatal("the retyped parameter lost its $ID")
	}
	if dString(p.doc, "Documentation") != "doc of A" || dString(p.doc, "RelativeMiddlePoint") != "200;53" {
		t.Error("retyping touched the parameter's documentation or position")
	}
	if typ := dString(dDoc(p.doc, "VariableType"), "$Type"); typ != "DataTypes$IntegerType" {
		t.Errorf("parameter type = %s", typ)
	}
	if _, err := m.Bytes(); err != nil {
		t.Fatalf("integrity: %v", err)
	}
}

func TestSetHeader_AddsAndRemovesParameters(t *testing.T) {
	stored := headerUnit(param("A", "DataTypes$StringType", 200), param("B", "DataTypes$StringType", 300))
	m, _ := newMutator(t, stored)
	changed, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType",
		freshParam("A", "DataTypes$StringType", 200), freshParam("N", "DataTypes$BooleanType", 300)))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(changed, ","); got != "parameter $B removed,parameter $N added" {
		t.Fatalf("changed %q", got)
	}
	var names []string
	for _, el := range arrayElements(dGet(dDoc(m.doc, "ObjectCollection"), "Objects")) {
		if d, _ := el.(bson.D); dString(d, "$Type") == parameterType {
			names = append(names, dString(d, "Name"))
		}
	}
	if strings.Join(names, ",") != "A,N" {
		t.Errorf("parameters %v, want A,N in that order", names)
	}
	if _, err := m.Bytes(); err != nil {
		t.Fatalf("integrity: %v", err)
	}
}

// A removed parameter the flow still reads is refused: the expression naming
// it would be left behind.
func TestSetHeader_RefusesRemovingAParameterInUse(t *testing.T) {
	stored := headerUnit(param("A", "DataTypes$StringType", 200))
	objs := dDoc(stored, "ObjectCollection")
	list := dGet(objs, "Objects").(bson.A)
	act := obj("uses", "Microflows$ActionActivity", 250, 400)
	act = append(act, bson.E{Key: "Action", Value: bson.D{{Key: "$ID", Value: bin("uses-a")}, {Key: "$Type", Value: "Microflows$LogMessageAction"},
		{Key: "Expression", Value: "'x' + $A"}}})
	dSet(objs, "Objects", append(list, act))
	m, _ := newMutator(t, stored)
	_, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType"))
	if err == nil || !strings.Contains(err.Error(), "$A is removed, but the flow still uses it") {
		t.Fatalf("want a refusal naming the use, got %v", err)
	}
}

func TestSetHeader_RefusesAReorder(t *testing.T) {
	stored := headerUnit(param("A", "DataTypes$StringType", 200), param("B", "DataTypes$StringType", 300))
	m, _ := newMutator(t, stored)
	_, err := m.SetHeader(declaredDoc("Hidden", "DataTypes$VoidType",
		freshParam("B", "DataTypes$StringType", 200), freshParam("A", "DataTypes$StringType", 300)))
	if err == nil || !strings.Contains(err.Error(), "reordered") {
		t.Fatalf("want a reorder refusal, got %v", err)
	}
}

// Move changes the node's position and nothing else: its flows keep their
// sides and control vectors, and no other node moves.
func TestMove_SetsThePositionOnly(t *testing.T) {
	m, _ := newMutator(t, unit(line(), lineFlows()))
	before := m.graph()
	if err := m.Move(model.ID(uid("b")), model.Point{X: 420, Y: 330}); err != nil {
		t.Fatal(err)
	}
	after := m.graph()
	for id, n := range after.nodes {
		want := before.nodes[id].pos
		if id == uid("b") {
			want = point{420, 330}
		}
		if n.pos != want {
			t.Errorf("%s at %v, want %v", describeNode(n), n.pos, want)
		}
	}
	for i := range after.flows {
		a, _ := bson.Marshal(after.flows[i].doc)
		b, _ := bson.Marshal(before.flows[i].doc)
		if !bytes.Equal(a, b) {
			t.Errorf("flow %s changed", after.flows[i].id)
		}
	}
}

// A placed fragment stays where the builder put it and moves nothing to make
// room; its new flow ends face the nodes they connect.
func TestSplice_PlacedFragmentStaysWhereStated(t *testing.T) {
	m, _ := newMutator(t, unit(line(), lineFlows()))
	frag := oneActivity()
	frag.Objects[0].SetPosition(model.Point{X: 250, Y: 350})
	frag.Placed = true
	if err := m.InsertAfter(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	g := m.graph()
	if p := g.nodes[string(frag.Entry)].pos; p != (point{250, 350}) {
		t.Errorf("the placed fragment is at %v, want 250;350", p)
	}
	if p := g.nodes[uid("b")].pos; p != (point{420, 200}) {
		t.Errorf("b moved to %v; a placed fragment makes no room", p)
	}
	for _, f := range g.flows {
		if f.dest == string(frag.Entry) {
			if idx, _ := flowEnd(f.doc, "Destination"); idx != sideTop {
				t.Errorf("the flow from a enters the fragment below it on side %d, want top", idx)
			}
		}
		if f.origin == string(frag.Entry) {
			if idx, _ := flowEnd(f.doc, "Origin"); idx != sideRight {
				t.Errorf("the flow to b, up and to the right, leaves the fragment on side %d, want right", idx)
			}
		}
	}
}
