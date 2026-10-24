// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#549: where Studio Pro stored DisabledDuringExecution false on an
// unused action slot (a Forms$NoAction), a rewrite wrote the writer's true over
// it. The classic widgets of a page or snippet already kept it (#721 L2,
// carryNoActionExecution). Two writers still did not: a pluggable widget the
// statement changes, rebuilt from its template — measured on ako/TestApp's
// WorkflowCommons.Snip_TaskDashboard_Header, where dropDown2's onEnterEvent and
// onLeaveEvent came back true from a check-clean `create or modify snippet` —
// and every widget of a layout, whose rewrite never called the carry.
//
// mxcli writes true on every slot it creates, so these tests seed the false a
// Studio Pro-authored element holds; without the seed a rewrite resetting it
// is undetectable.

const issue549Setup = `create enumeration MyFirstModule.Colour (Red 'Red', Blue 'Blue');
create persistent entity MyFirstModule.Thing (Colour: Enumeration(MyFirstModule.Colour));`

const issue549Page = `create or modify page MyFirstModule.ThingEdit (Title: 'Thing', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvThing (DataSource: $Thing) {
    combobox cbColour (Label: 'Colour', Attribute: Colour)
  }
};`

// storedUnitDoc reads and decodes a stored unit.
func storedUnitDoc(t *testing.T, exec *Executor, unitID string) bson.D {
	t.Helper()
	raw, err := exec.backend.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func writeUnitDoc(t *testing.T, exec *Executor, unitID string, doc bson.D) {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.backend.UpdateRawUnit(unitID, raw); err != nil {
		t.Fatal(err)
	}
}

// namedDoc finds the element named name in doc.
func namedDoc(t *testing.T, doc bson.D, name string) bson.D {
	t.Helper()
	var found bson.D
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if found == nil && bsonnav.DGetString(n, "Name") == name {
				found = n
				return
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("no element %s in the stored unit", name)
	}
	return found
}

// pluggableSlotAction returns the action a pluggable widget stores under the
// property key.
func pluggableSlotAction(t *testing.T, widget bson.D, key string) bson.D {
	t.Helper()
	var typeID string
	for _, pt := range bsonDocs(bsonValue(bsonDocField(bsonDocField(widget, "Type"), "ObjectType"), "PropertyTypes")) {
		if bsonString(pt, "PropertyKey") == key {
			typeID = bsonIDKey(bsonValue(pt, "$ID"))
		}
	}
	for _, prop := range bsonDocs(bsonValue(bsonDocField(widget, "Object"), "Properties")) {
		if typeID != "" && bsonIDKey(bsonValue(prop, "TypePointer")) == typeID {
			return bsonDocField(bsonDocField(prop, "Value"), "Action")
		}
	}
	t.Fatalf("widget %s stores no property %s", bsonnav.DGetString(widget, "Name"), key)
	return nil
}

func storedFlag(t *testing.T, action bson.D) bool {
	t.Helper()
	if got := bsonnav.DGetString(action, "$Type"); got != "Forms$NoAction" {
		t.Fatalf("slot holds %s, want Forms$NoAction", got)
	}
	v, ok := bsonnav.DGet(action, "DisabledDuringExecution").(bool)
	if !ok {
		t.Fatal("slot stores no DisabledDuringExecution")
	}
	return v
}

func TestIssue549_ChangedPluggableWidgetKeepsStoredNoActionFlag(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue549Setup)
	run(t, exec, issue549Page)
	id := unitIDByName(t, exec, "page", "ThingEdit")

	doc := storedUnitDoc(t, exec, id)
	bsonnav.DSet(pluggableSlotAction(t, namedDoc(t, doc, "cbColour"), "onEnterEvent"), "DisabledDuringExecution", false)
	writeUnitDoc(t, exec, id, doc)
	if storedFlag(t, pluggableSlotAction(t, namedDoc(t, storedUnitDoc(t, exec, id), "cbColour"), "onEnterEvent")) {
		t.Fatal("seed did not take")
	}

	// A changed combo box is rebuilt from the template, not passed through.
	run(t, exec, strings.Replace(issue549Page, "Label: 'Colour'", "Label: 'Colour (changed)'", 1))
	if !storedHas(t, exec, id, "Colour (changed)") {
		t.Fatal("the rewrite did not land, so it shows nothing about the carry")
	}
	cb := namedDoc(t, storedUnitDoc(t, exec, id), "cbColour")
	if storedFlag(t, pluggableSlotAction(t, cb, "onEnterEvent")) {
		t.Error("rebuilding cbColour wrote DisabledDuringExecution true over the stored false of its onEnterEvent")
	}
	// The carry copies the stored value; it does not write false everywhere.
	if !storedFlag(t, pluggableSlotAction(t, cb, "onLeaveEvent")) {
		t.Error("onLeaveEvent, stored true, came back false")
	}
}

func TestIssue549_LayoutRewriteKeepsStoredNoActionFlag(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	const layout = `create or modify layout MyFirstModule.MyLayout (layouttype: 'Default') {
  actionbutton btnHome (Caption: 'Home')
  placeholder Main
};`
	run(t, exec, layout)
	ls, err := exec.backend.ListLayouts()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, l := range ls {
		if l.Name == "MyLayout" {
			id = string(l.ID)
		}
	}
	if id == "" {
		t.Fatal("no layout MyLayout")
	}

	doc := storedUnitDoc(t, exec, id)
	bsonnav.DSet(bsonnav.DGetDoc(namedDoc(t, doc, "btnHome"), "Action"), "DisabledDuringExecution", false)
	writeUnitDoc(t, exec, id, doc)

	run(t, exec, strings.Replace(layout, "Caption: 'Home'", "Caption: 'Start'", 1))
	if !storedHas(t, exec, id, "Start") {
		t.Fatal("the rewrite did not land, so it shows nothing about the carry")
	}
	if storedFlag(t, bsonnav.DGetDoc(namedDoc(t, storedUnitDoc(t, exec, id), "btnHome"), "Action")) {
		t.Error("rewriting the layout wrote DisabledDuringExecution true over btnHome's stored false")
	}
}

// pluggableWith is a pluggable widget Type and Object whose properties hold the
// given actions, by property key; ids prefixes the property type IDs.
func pluggableWith(ids string, actions map[string]bson.D) (typ, obj bson.D) {
	types := bson.A{int32(2)}
	props := bson.A{int32(2)}
	for key, action := range actions {
		types = append(types, bson.D{{Key: "$ID", Value: hcBin(ids + key)}, {Key: "PropertyKey", Value: key}})
		props = append(props, bson.D{
			{Key: "TypePointer", Value: hcBin(ids + key)},
			{Key: "Value", Value: bson.D{{Key: "Action", Value: action}}},
		})
	}
	return bson.D{{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: types}}}},
		bson.D{{Key: "Properties", Value: props}}
}

func noAction(flag bool) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$NoAction"}, {Key: "DisabledDuringExecution", Value: flag}}
}

func TestCarryPluggableNoActionExecution(t *testing.T) {
	const widgetID = "com.mendix.widget.web.combobox.Combobox"
	storedTyp, storedObj := pluggableWith("stored-", map[string]bson.D{
		"onEnterEvent":  noAction(false),
		"onChangeEvent": noAction(false),
	})
	pt := &pluggablePassthrough{byName: map[string]*storedPluggable{
		"cb1": {widgetID: widgetID, typ: storedTyp, obj: storedObj},
	}}
	def := &WidgetDefinition{WidgetID: widgetID}
	// The template's IDs are not the stored ones: properties pair by key.
	built := func() *pages.CustomWidget {
		typ, obj := pluggableWith("template-", map[string]bson.D{
			"onEnterEvent":  noAction(true),
			"onChangeEvent": {{Key: "$Type", Value: "Forms$MicroflowAction"}, {Key: "DisabledDuringExecution", Value: true}},
			"onLeaveEvent":  noAction(true),
		})
		return &pages.CustomWidget{RawType: typ, RawObject: obj}
	}
	flags := func(cw *pages.CustomWidget) map[string]bool {
		out := map[string]bool{}
		w := bson.D{{Key: "Name", Value: "cb1"}, {Key: "Type", Value: cw.RawType}, {Key: "Object", Value: cw.RawObject}}
		for _, key := range []string{"onEnterEvent", "onChangeEvent", "onLeaveEvent"} {
			out[key], _ = bsonnav.DGet(pluggableSlotAction(t, w, key), "DisabledDuringExecution").(bool)
		}
		return out
	}

	cw := built()
	pt.carryNoActionExecution(def, &ast.WidgetV3{Type: "combobox", Name: "cb1"}, cw)
	got := flags(cw)
	if got["onEnterEvent"] {
		t.Error("onEnterEvent: the stored false was not carried")
	}
	if !got["onChangeEvent"] {
		t.Error("onChangeEvent: an action the statement writes took the stored NoAction's flag")
	}
	if !got["onLeaveEvent"] {
		t.Error("onLeaveEvent: a slot the stored widget does not hold changed")
	}

	// Another widget under the name, or no stored widget of it: nothing moves.
	for _, tc := range []struct {
		name string
		def  *WidgetDefinition
		w    *ast.WidgetV3
	}{
		{"another package", &WidgetDefinition{WidgetID: "com.mendix.widget.web.gallery.Gallery"}, &ast.WidgetV3{Name: "cb1"}},
		{"no stored widget", def, &ast.WidgetV3{Name: "cb2"}},
	} {
		cw := built()
		pt.carryNoActionExecution(tc.def, tc.w, cw)
		if !flags(cw)["onEnterEvent"] {
			t.Errorf("%s: the flag was carried", tc.name)
		}
	}
}
