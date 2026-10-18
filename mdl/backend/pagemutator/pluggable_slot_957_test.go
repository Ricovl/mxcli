// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// dataGrid2WithControlBar is a Data grid 2 whose filtersPlaceholder — the slot
// describe prints as `controlbar { … }` — holds the named buttons, stored as
// Studio Pro leaves it once its last button is gone: an empty Widgets [2].
func dataGrid2WithControlBar(buttons ...string) bson.D {
	typeID := primitive.Binary{Subtype: 0x04, Data: []byte{
		0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68,
		0x69, 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70,
	}}
	widgets := bson.A{int32(2)}
	for _, b := range buttons {
		widgets = append(widgets, bson.D{{Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: b}})
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "dataGrid21"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "WidgetId", Value: "com.mendix.widget.web.datagrid.Datagrid"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
					{Key: "$ID", Value: typeID},
					{Key: "PropertyKey", Value: "filtersPlaceholder"},
					{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "Widgets"}}},
				}}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{int32(2), bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{{Key: "Widgets", Value: widgets}}},
			}}},
		}},
	}
}

func controlBarNames(t *testing.T, m *Mutator) []string {
	t.Helper()
	grid := findWidgetForTest(t, m.rawData, "dataGrid21")
	prop := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(grid, "Object"), "Properties"))[0].(bson.D)
	raw := bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(prop, "Value"), "Widgets"))
	if len(raw) == 0 || raw[0] != int32(2) {
		t.Fatalf("filtersPlaceholder Widgets lost its [2] marker: %v", raw)
	}
	var names []string
	for _, w := range raw[1:] {
		names = append(names, bsonnav.DGetString(w.(bson.D), "Name"))
	}
	return names
}

func newButton(name string) pages.Widget {
	b := &pages.ActionButton{}
	b.Name = name
	return b
}

// mendixlabs/mxcli#957: once the grid's last control-bar button was dropped,
// every route back was refused — `insert into controlBar1` ("not found": the
// bar has no name) and `insert into dataGrid21` ("not a simple container").
// The slot is addressed the way a data view's footer is: `dataGrid21.controlbar`.
func TestInsertIntoDataGrid2ControlBar_957(t *testing.T) {
	for _, member := range []string{"controlbar", "ControlBar", "filtersPlaceholder"} {
		m := New(makeRawPage(dataGrid2WithControlBar()), model.ID("u"), &stubWidgetDeps{})
		if err := m.InsertWidget("dataGrid21", member, backend.InsertPosition("into"), []pages.Widget{newButton("btnNew")}); err != nil {
			t.Fatalf("insert into dataGrid21.%s: %v", member, err)
		}
		if got := controlBarNames(t, m); len(got) != 1 || got[0] != "btnNew" {
			t.Errorf("dataGrid21.%s after insert = %v, want [btnNew]", member, got)
		}
	}
}

func TestInsertIntoDataGrid2ControlBarAppends_957(t *testing.T) {
	m := New(makeRawPage(dataGrid2WithControlBar("btnA")), model.ID("u"), &stubWidgetDeps{})
	if err := m.InsertWidget("dataGrid21", "controlbar", backend.InsertPosition("into"), []pages.Widget{newButton("btnB")}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(controlBarNames(t, m), ","); got != "btnA,btnB" {
		t.Errorf("control bar = %s, want btnA,btnB", got)
	}
}

func TestDataGrid2ControlBarRefusesBeforeAfter_957(t *testing.T) {
	m := New(makeRawPage(dataGrid2WithControlBar()), model.ID("u"), &stubWidgetDeps{})
	err := m.InsertWidget("dataGrid21", "controlbar", backend.InsertPosition("after"), []pages.Widget{newButton("btnNew")})
	if err == nil || !strings.Contains(err.Error(), "INSERT INTO") {
		t.Fatalf("insert after a slot should be refused with the INTO hint, got %v", err)
	}
}

func TestReplaceAndDropDataGrid2ControlBar_957(t *testing.T) {
	m := New(makeRawPage(dataGrid2WithControlBar("btnA", "btnB")), model.ID("u"), &stubWidgetDeps{})
	if err := m.ReplaceWidget("dataGrid21", "controlbar", []pages.Widget{newButton("btnC")}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(controlBarNames(t, m), ","); got != "btnC" {
		t.Errorf("after replace = %s, want btnC", got)
	}
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "dataGrid21", Column: "controlbar"}}); err != nil {
		t.Fatal(err)
	}
	if got := controlBarNames(t, m); len(got) != 0 {
		t.Errorf("after drop = %v, want empty (slot kept)", got)
	}
}

// `insert into dataGrid21` names the grid itself; the refusal now points at
// the slot address that works instead of a widget inside a column or tab.
func TestInsertIntoDataGrid2ItselfNamesTheSlot_957(t *testing.T) {
	m := New(makeRawPage(dataGrid2WithControlBar()), model.ID("u"), &stubWidgetDeps{})
	err := m.InsertWidget("dataGrid21", "", backend.InsertPosition("into"), []pages.Widget{newButton("btnNew")})
	if err == nil || !strings.Contains(err.Error(), "dataGrid21.controlbar") {
		t.Fatalf("want a refusal naming dataGrid21.controlbar, got %v", err)
	}
}

// The generic ALTER resolves the target before the operation runs; it must
// accept the slot address the operation accepts.
func TestResolveAlterTargetDataGrid2ControlBar_957(t *testing.T) {
	m := New(makeRawPage(dataGrid2WithControlBar()), model.ID("u"), &stubWidgetDeps{})
	got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"dataGrid21", "controlbar"}})
	if err != nil {
		t.Fatalf("ResolveAlterTarget(dataGrid21.controlbar): %v", err)
	}
	if got.Kind != "region" {
		t.Errorf("kind = %q, want region", got.Kind)
	}
}
