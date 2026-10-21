// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func mfID(name string) bson.Binary {
	b := make([]byte, 16)
	copy(b, name)
	return bson.Binary{Data: b}
}

func mfUUID(name string) string { return types.BlobToUUID(mfID(name).Data) }

func mfObj(name, typ string, x int) bson.D {
	return bson.D{
		{Key: "$ID", Value: mfID(name)},
		{Key: "$Type", Value: typ},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;200", x)},
		{Key: "Size", Value: "120;60"},
	}
}

func mfFlow(name, from, to string) bson.D {
	return bson.D{
		{Key: "$ID", Value: mfID(name)},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "DestinationConnectionIndex", Value: int32(3)},
		{Key: "DestinationPointer", Value: mfID(to)},
		{Key: "IsErrorHandler", Value: false},
		{Key: "Line", Value: bson.D{{Key: "$Type", Value: "Microflows$BezierCurve"},
			{Key: "DestinationControlVector", Value: "-30;0"}, {Key: "OriginControlVector", Value: "30;0"}}},
		{Key: "OriginConnectionIndex", Value: int32(1)},
		{Key: "OriginPointer", Value: mfID(from)},
	}
}

// storedReduce is the shape of TestApp's Microflows.MicroflowReduce, the flow
// the live check ran on: a start, three activities 170 apart, an end.
func storedReduce() bson.D {
	return bson.D{
		{Key: "$ID", Value: mfID("unit")},
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Flows", Value: bson.A{int32(3), mfFlow("f0", "start", "a"), mfFlow("f1", "a", "b"), mfFlow("f2", "b", "end")}},
		{Key: "Name", Value: "Reduce"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: mfID("oc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3),
				mfObj("start", "Microflows$StartEvent", -98),
				mfObj("end", "Microflows$EndEvent", 564),
				mfObj("a", "Microflows$ActionActivity", 214),
				mfObj("b", "Microflows$ActionActivity", 404),
			}},
		}},
	}
}

func openFlowMutator(t *testing.T, b *Backend) (*mcpFlowMutator, *mcpFlowDeps) {
	t.Helper()
	stored := storedReduce()
	raw, _ := bson.Marshal(stored)
	var storedD, work bson.D
	_ = bson.Unmarshal(raw, &storedD)
	_ = bson.Unmarshal(raw, &work)
	deps := &mcpFlowDeps{b: b, docType: microflowDocType, qn: "M.Reduce", stored: storedD,
		objects: map[string]microflows.MicroflowObject{}, flows: map[string]*microflows.SequenceFlow{},
		annotations: map[string]*microflows.AnnotationFlow{}}
	m, err := mfmutator.New(work, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	return &mcpFlowMutator{Mutator: m, qn: "M.Reduce"}, deps
}

func logFragment() *backend.MicroflowFragment {
	id := model.ID(types.GenerateID())
	act := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: id},
			Position:    model.Point{X: 360, Y: 200},
			Size:        model.Size{Width: 120, Height: 60},
		}},
		Action: &microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'Reduce'",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "reduced"}}},
	}
	return &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{act}, Entry: id, Exit: id}
}

// The splice becomes PED path operations addressed by the stored indexes:
// the moved end, the new activity (added bare, then its action and position
// set, as PED's skeleton constructor requires), the rewired flow's end, and
// the new flow — and no removal. Which of them wait for the second update is
// TestFlowPatchOps_NewElementsAreFilledInOnceTheyExist.
func TestFlowPatchOps_InsertAfter(t *testing.T) {
	var sent []any
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			return `{"results":[{"path":"/objectCollection/objects","result":[` +
				`{"$Type":"Microflows$StartEvent","relativeMiddlePoint":{"x":-98,"y":200}},` +
				`{"$Type":"Microflows$EndEvent","relativeMiddlePoint":{"x":564,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":214,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":404,"y":200}}]},` +
				`{"path":"/flows","result":[{},{},{}]}]}`, false
		case "ped_update_document":
			sent = append(sent, args["operations"])
			return "SUCCESS: All operations have been performed successfully.", false
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	m, _ := openFlowMutator(t, b)
	if err := m.InsertAfter(model.ID(mfUUID("a")), logFragment()); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("want the insert and its fill, got %d updates", len(sent))
	}
	js, _ := json.Marshal(append(sent[0].([]any), sent[1].([]any)...))
	got := string(js)
	var ops []pedOpEntry
	if err := json.Unmarshal(js, &ops); err != nil {
		t.Fatal(err)
	}
	has := func(path, typ, value string) bool {
		for _, op := range ops {
			v, _ := json.Marshal(op.Operation.Value)
			if op.Path == path && op.Operation.Type == typ && strings.Contains(string(v), value) {
				return true
			}
		}
		return false
	}
	for _, w := range []struct{ path, typ, value string }{
		{"/objectCollection/objects/3/relativeMiddlePoint", "set", `"x":534`}, // b moved along
		{"/objectCollection/objects", "add", `"Microflows$ActionActivity"`},
		{"/objectCollection/objects/4/action", "set", `"Microflows$LogMessageAction"`},
		{"/flows/1/destination", "set", `$id(/objectCollection/objects/4)`},
		{"/flows", "add", `"destinationId":"$id(/objectCollection/objects/3)"`},
		{"/flows", "add", `"originId":"$id(/objectCollection/objects/4)"`},
	} {
		if !has(w.path, w.typ, w.value) {
			t.Errorf("no %s %s with %s in:\n%s", w.typ, w.path, w.value, got)
		}
	}
	if strings.Contains(got, `"remove"`) {
		t.Errorf("an insert sent a removal:\n%s", got)
	}
}

// A note inserted with an activity is added as an object and its line as a
// Microflows$AnnotationFlow element, whose ends are origin/destination — not a
// SequenceFlow constructor's originId/destinationId. Measured on 11.15.0-rc.4:
// the old keys fail the whole update with `"/origin":"Expected reference
// (string), got undefined"`.
func TestFlowPatchOps_InsertNote(t *testing.T) {
	var sent []any
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			return `{"results":[{"path":"/objectCollection/objects","result":[` +
				`{"$Type":"Microflows$StartEvent","relativeMiddlePoint":{"x":-98,"y":200}},` +
				`{"$Type":"Microflows$EndEvent","relativeMiddlePoint":{"x":564,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":214,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":404,"y":200}}]},` +
				`{"path":"/flows","result":[{},{},{}]}]}`, false
		case "ped_update_document":
			sent = append(sent, args["operations"])
			return "SUCCESS: All operations have been performed successfully.", false
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	m, _ := openFlowMutator(t, b)
	frag := logFragment()
	note := &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Position:    model.Point{X: 360, Y: 100},
		Size:        model.Size{Width: 200, Height: 50},
	}, Caption: "Why we log"}
	frag.Objects = append(frag.Objects, note)
	frag.AnnotationFlows = []*microflows.AnnotationFlow{{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:    note.ID, DestinationID: frag.Entry,
	}}
	if err := m.InsertAfter(model.ID(mfUUID("a")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("want the insert and its fill, got %d updates", len(sent))
	}
	js, _ := json.Marshal(sent[0])
	var ops []pedOpEntry
	if err := json.Unmarshal(js, &ops); err != nil {
		t.Fatal(err)
	}
	var noteAdd, line map[string]any
	lineAt, lastSequenceOp := -1, -1
	for i, op := range ops {
		v, _ := op.Operation.Value.(map[string]any)
		switch {
		case op.Operation.Type == "add" && v["$Type"] == "Microflows$Annotation":
			noteAdd = v
		case op.Operation.Type == "add" && v["$Type"] == "Microflows$AnnotationFlow":
			line, lineAt = v, i
		case strings.HasPrefix(op.Path, "/flows/3"), op.Operation.Type == "add" && v["$Type"] == "Microflows$SequenceFlow":
			lastSequenceOp = i
		}
	}
	// PED stores a note line after a sequence flow added later in the same
	// update, so the new sequence flow must come first: the fill that follows
	// addresses it as /flows/3.
	if lineAt < lastSequenceOp {
		t.Errorf("the note line (op %d) is added before the new sequence flow's ops (last at %d):\n%s", lineAt, lastSequenceOp, js)
	}
	if noteAdd == nil || noteAdd["caption"] != "Why we log" {
		t.Errorf("note add = %v, want the Annotation with its caption in:\n%s", noteAdd, js)
	}
	if line == nil {
		t.Fatalf("no AnnotationFlow add in:\n%s", js)
	}
	// The activity is the first object added (index 4), the note the second.
	if line["origin"] != "$id(/objectCollection/objects/5)" || line["destination"] != "$id(/objectCollection/objects/4)" {
		t.Errorf("note line = %v -> %v, want objects/5 -> objects/4", line["origin"], line["destination"])
	}
	if _, ok := line["originId"]; ok {
		t.Errorf("note line carries originId, which PED's AnnotationFlow does not take: %v", line)
	}
	if fmt.Sprint(line["originConnectionIndex"], line["destinationConnectionIndex"]) != "2 0" {
		t.Errorf("note line sides = %v -> %v, want the unoriented 2 -> 0", line["originConnectionIndex"], line["destinationConnectionIndex"])
	}
}

// reducePED fakes Studio Pro holding storedReduce, and records each
// ped_update_document's operations.
func reducePED(t *testing.T, sent *[][]pedOpEntry) *Backend {
	t.Helper()
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			return `{"results":[{"path":"/objectCollection/objects","result":[` +
				`{"$Type":"Microflows$StartEvent","relativeMiddlePoint":{"x":-98,"y":200}},` +
				`{"$Type":"Microflows$EndEvent","relativeMiddlePoint":{"x":564,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":214,"y":200}},` +
				`{"$Type":"Microflows$ActionActivity","relativeMiddlePoint":{"x":404,"y":200}}]},` +
				`{"path":"/flows","result":[{},{},{}]}]}`, false
		case "ped_update_document":
			js, _ := json.Marshal(args["operations"])
			var ops []pedOpEntry
			if err := json.Unmarshal(js, &ops); err != nil {
				t.Fatal(err)
			}
			*sent = append(*sent, ops)
			return "SUCCESS: All operations have been performed successfully.", false
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	return &Backend{client: f.connectClient(t)}
}

// opsAt maps the paths an update sets to their JSON values.
func opsAt(ops []pedOpEntry) map[string]string {
	got := map[string]string{}
	for _, op := range ops {
		if op.Operation.Type == "set" {
			v, _ := json.Marshal(op.Operation.Value)
			got[op.Path] = string(v)
		}
	}
	return got
}

// PED resolves a path against the document as it was before the update, so
// an element the update adds cannot be addressed in it (measured on
// 11.15.0-rc.4): a set below a new flow's line fails the whole update with
// `undefined is not an object (evaluating 'u.$Type')`, which made every insert
// fail, and a set of a new activity's action is accepted and lost, which left
// it with "No action defined". So the update that adds an element addresses
// nothing on it, and a second update fills it in.
func TestFlowPatchOps_NewElementsAreFilledInOnceTheyExist(t *testing.T) {
	var sent [][]pedOpEntry
	m, _ := openFlowMutator(t, reducePED(t, &sent))
	if err := m.InsertAfter(model.ID(mfUUID("a")), logFragment()); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("want the insert and then its fill, got %d updates: %v", len(sent), sent)
	}
	// The stored document has four objects and three flows, so the insert
	// adds /objectCollection/objects/4 and /flows/3.
	for _, op := range sent[0] {
		if strings.HasPrefix(op.Path, "/objectCollection/objects/4/") || strings.HasPrefix(op.Path, "/flows/3/") {
			t.Errorf("the update that adds the element also sets %s on it", op.Path)
		}
	}
	got := opsAt(sent[1])
	if !strings.Contains(got["/objectCollection/objects/4/action"], `"Microflows$LogMessageAction"`) {
		t.Errorf("the fill does not set the new activity's action: %v", got)
	}
	for p, v := range map[string]string{
		// The new flow leaves the activity on the right…
		"/flows/3/line/originControlVector": `{"height":0,"width":15}`,
		// …and keeps the end the rewired flow had at b.
		"/flows/3/line/destinationControlVector": `{"height":0,"width":-30}`,
	} {
		if got[p] != v {
			t.Errorf("%s = %q, want %s", p, got[p], v)
		}
	}
	for p := range got {
		if !strings.HasPrefix(p, "/objectCollection/objects/4/") && !strings.HasPrefix(p, "/flows/3/") {
			t.Errorf("the fill sets %s, which the first update could address", p)
		}
	}
}

// Several elements appended to one array in one update are stored in reverse,
// and $id() resolves against that final order (measured on 11.15.0-rc.4): an
// IF inserted over MCP was wired to the wrong objects. Every add therefore
// carries the stored length as its index, which keeps them in listed order.
func TestFlowPatchOps_AddsKeepTheirListedOrder(t *testing.T) {
	var sent [][]pedOpEntry
	m, _ := openFlowMutator(t, reducePED(t, &sent))
	frag := logFragment()
	second := logFragment().Objects[0]
	second.SetPosition(model.Point{X: 520, Y: 200})
	frag.Objects = append(frag.Objects, second)
	frag.Flows = []*microflows.SequenceFlow{{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:    frag.Entry, DestinationID: second.GetID(),
		OriginConnectionIndex: 1, DestinationConnectionIndex: 3,
	}}
	frag.Exit = second.GetID()
	if err := m.InsertAfter(model.ID(mfUUID("a")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	adds := map[string]int{}
	for _, op := range sent[0] {
		if op.Operation.Type != "add" {
			continue
		}
		adds[op.Path]++
		want := map[string]int{"/objectCollection/objects": 4, "/flows": 3}[op.Path]
		if op.Operation.Index == nil || *op.Operation.Index != want {
			t.Errorf("add to %s has index %v, want the stored length %d", op.Path, op.Operation.Index, want)
		}
	}
	if adds["/objectCollection/objects"] != 2 || adds["/flows"] != 2 {
		t.Errorf("adds = %v, want two objects and two flows", adds)
	}
}

// A 0;0 control vector is the line's schema default, which PED asks not to be
// sent.
func TestNewFlowLineOps_LeavesOutDefaults(t *testing.T) {
	ops := newFlowLineOps("/flows/4", &microflows.SequenceFlow{OriginControlVector: "0;0", DestinationControlVector: "0;-15"})
	if len(ops) != 1 || ops[0].Path != "/flows/4/line/destinationControlVector" {
		t.Errorf("ops = %+v, want only the destination vector", ops)
	}
	if ops := newFlowLineOps("/flows/4", &microflows.SequenceFlow{OriginControlVector: "0;0"}); len(ops) != 0 {
		t.Errorf("default vectors produced %+v", ops)
	}
}

// An error handler flow is marked in the fill too. Set in the update that adds
// the flow, isErrorHandler is accepted and the flow reads back as an ordinary
// one (measured on 11.15.0-rc.4) — a second normal exit from the activity.
func TestFlowPatchOps_NewErrorHandlerFlowIsMarkedOnceItExists(t *testing.T) {
	var sent [][]pedOpEntry
	m, _ := openFlowMutator(t, reducePED(t, &sent))
	frag := logFragment()
	handler := logFragment().Objects[0]
	handler.SetPosition(model.Point{X: 360, Y: 300})
	frag.Objects = append(frag.Objects, handler)
	frag.Flows = []*microflows.SequenceFlow{{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:    frag.Entry, DestinationID: handler.GetID(),
		OriginConnectionIndex: 2, IsErrorHandler: true,
	}}
	if err := m.InsertAfter(model.ID(mfUUID("a")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("want the insert and then its fill, got %d updates", len(sent))
	}
	for _, op := range sent[0] {
		if strings.HasSuffix(op.Path, "/isErrorHandler") {
			t.Errorf("the update that adds the flow also marks it at %s", op.Path)
		}
	}
	// The fragment's own flow is added first (/flows/3), then the one leaving
	// the fragment (/flows/4).
	if got := opsAt(sent[1]); got["/flows/3/isErrorHandler"] != "true" {
		t.Errorf("the error handler flow is not marked in the fill: %v", got)
	}
}

// PED addresses by index, so a live document that differs from the stored one
// must refuse the write before anything is sent.
func TestFlowPatchOps_StaleLiveDocumentIsRefused(t *testing.T) {
	updates := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			return `{"results":[{"path":"/objectCollection/objects","result":[{},{},{},{},{}]},{"path":"/flows","result":[{},{},{}]}]}`, false
		case "ped_update_document":
			updates++
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	m, _ := openFlowMutator(t, b)
	if err := m.InsertAfter(model.ID(mfUUID("a")), logFragment()); err != nil {
		t.Fatal(err)
	}
	err := m.Save()
	if err == nil || !strings.Contains(err.Error(), "no longer matches the local project") {
		t.Fatalf("want the stale-document refusal, got %v", err)
	}
	if updates != 0 {
		t.Error("an update was sent to a document that did not match")
	}
}

// Drop and replace need a removal, which PED does not roll back when an
// update fails; over MCP they are refused.
func TestMCPFlowMutator_RefusesRemovals(t *testing.T) {
	m, _ := openFlowMutator(t, &Backend{})
	if err := m.Drop(model.ID(mfUUID("a"))); err == nil || !strings.Contains(err.Error(), "not supported by the MCP backend") {
		t.Errorf("drop: %v", err)
	}
	if err := m.Replace(model.ID(mfUUID("a")), logFragment()); err == nil || !strings.Contains(err.Error(), "not supported by the MCP backend") {
		t.Errorf("replace: %v", err)
	}
}
