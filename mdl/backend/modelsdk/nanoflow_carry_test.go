// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#705 item 2. describe -> exec of the Blank template's
// FeedbackModule.ACT_Feedback_UploadImage lost its three annotation connectors
// and three header properties:
//
//   - nanoflowToGen never wrote ObjectCollection.AnnotationFlows, though the
//     reader fills it and the flow builder produces it. microflowToGen always
//     did; ruleToGen had the same omission. With the connectors gone the notes
//     float free, and the next DESCRIBE no longer attaches them to anything.
//   - ExportLevel, UseListParameterByReference and ReturnVariableName were never
//     written, so a rewrite deleted them. None has an MDL spelling except the
//     return variable (`returns T as $Var`), so UpdateNanoflow carries the stored
//     keys — and only the keys the stored document carries, since Studio Pro
//     accepts an absent one and a key a project's metamodel does not declare
//     makes it unopenable.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// annotatedCollection is a start event, one activity and a note attached to it.
func annotatedCollection() *microflows.MicroflowObjectCollection {
	start := &microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000001")},
	}}
	end := &microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000002")},
		Position:    model.Point{X: 200, Y: 100},
	}}
	note := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000003")},
			Position:    model.Point{X: 200, Y: 0},
			Size:        model.Size{Width: 200, Height: 50},
		},
		Caption: "Explains the end",
	}
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, end, note},
		Flows: []*microflows.SequenceFlow{{
			BaseElement:   model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000004")},
			OriginID:      start.ID,
			DestinationID: end.ID,
		}},
		AnnotationFlows: []*microflows.AnnotationFlow{{
			BaseElement:   model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000005")},
			OriginID:      note.ID,
			DestinationID: end.ID,
		}},
	}
}

func countFlowType(t *testing.T, raw []byte, typeName string) int {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	n := 0
	for _, e := range doc {
		if e.Key != "Flows" {
			continue
		}
		for _, f := range e.Value.(bson.A) {
			if d, ok := f.(bson.D); ok {
				for _, fe := range d {
					if fe.Key == "$Type" && fe.Value == typeName {
						n++
					}
				}
			}
		}
	}
	return n
}

func TestNanoflowToGen_WritesAnnotationFlows(t *testing.T) {
	nf := &microflows.Nanoflow{Name: "NF", ObjectCollection: annotatedCollection()}
	g := nanoflowToGen(nf, 11)
	assignNanoflowIDs(g)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if n := countFlowType(t, raw, "Microflows$AnnotationFlow"); n != 1 {
		t.Errorf("AnnotationFlows written = %d, want 1 — the note is left unattached", n)
	}
	// Control: the sequence flow was always written.
	if n := countFlowType(t, raw, "Microflows$SequenceFlow"); n != 1 {
		t.Errorf("SequenceFlows written = %d, want 1", n)
	}
}

func TestRuleToGen_WritesAnnotationFlows(t *testing.T) {
	r := &microflows.Rule{Name: "R", ReturnType: &microflows.BooleanType{}, ObjectCollection: annotatedCollection()}
	g := ruleToGen(r, 11)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if n := countFlowType(t, raw, "Microflows$AnnotationFlow"); n != 1 {
		t.Errorf("AnnotationFlows written = %d, want 1 — the note is left unattached", n)
	}
}

func nanoflowFixture(t *testing.T) (*Backend, *microflows.Nanoflow) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	nf := &microflows.Nanoflow{ContainerID: mod.ID, Name: "ZzCarryNanoflow", ReturnType: &microflows.BooleanType{}}
	if err := b.CreateNanoflow(nf); err != nil {
		t.Fatalf("CreateNanoflow: %v", err)
	}
	return b, nf
}

// setStoredKeys stands in for a Studio Pro nanoflow: the three header keys at
// values that differ from anything a rebuild would produce.
func setStoredKeys(t *testing.T, b *Backend, id model.ID, kv bson.D) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range kv {
		set := false
		for i := range d {
			if d[i].Key == e.Key {
				d[i].Value, set = e.Value, true
			}
		}
		if !set {
			d = append(d, e)
		}
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(id), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

func storedKeys(t *testing.T, b *Backend, id model.ID) map[string]any {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]any{}
	for _, e := range d {
		out[e.Key] = e.Value
	}
	return out
}

func TestUpdateNanoflow_CarriesStoredHeaderKeys(t *testing.T) {
	b, nf := nanoflowFixture(t)
	setStoredKeys(t, b, nf.ID, bson.D{
		{Key: "ExportLevel", Value: "API"},
		{Key: "UseListParameterByReference", Value: false},
		{Key: "ReturnVariableName", Value: "IsValid"},
	})

	nf.Documentation = "rewritten"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	got := storedKeys(t, b, nf.ID)
	for k, want := range map[string]any{
		"ExportLevel": "API", "UseListParameterByReference": false, "ReturnVariableName": "IsValid",
		// Control: the authored change still lands.
		"Documentation": "rewritten",
	} {
		if got[k] != want {
			t.Errorf("%s = %#v, want %#v", k, got[k], want)
		}
	}
}

// The return variable is the one of the three MDL can author, and what the
// statement says wins over what is stored.
func TestUpdateNanoflow_AuthoredReturnVariableWins(t *testing.T) {
	b, nf := nanoflowFixture(t)
	setStoredKeys(t, b, nf.ID, bson.D{{Key: "ReturnVariableName", Value: "IsValid"}})

	nf.ReturnVariableName = "Result"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	if got := storedKeys(t, b, nf.ID)["ReturnVariableName"]; got != "Result" {
		t.Errorf("ReturnVariableName = %#v, want the authored Result", got)
	}
	if got, err := b.GetNanoflow(nf.ID); err != nil || got.ReturnVariableName != "Result" {
		t.Errorf("read back ReturnVariableName = %+v (%v), want Result", got, err)
	}
}

// Control: a stored document WITHOUT the keys gets none. Writing a key the
// project's metamodel may not declare is worse than leaving it absent.
func TestUpdateNanoflow_WritesNoKeyTheStoredDocumentLacks(t *testing.T) {
	b, nf := nanoflowFixture(t)
	nf.Documentation = "rewritten"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	got := storedKeys(t, b, nf.ID)
	for _, k := range []string{"ExportLevel", "UseListParameterByReference"} {
		if v, ok := got[k]; ok {
			t.Errorf("%s = %#v was written, but the stored document had no such key", k, v)
		}
	}
}
