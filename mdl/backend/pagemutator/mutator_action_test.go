// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// stubActionDeps serializes a client action to a marker document so the mutator
// test can assert what was written without pulling in an engine.
type stubActionDeps struct {
	Deps
	serialized bson.D
}

func (d *stubActionDeps) SerializeClientAction(a pages.ClientAction) bson.D {
	return d.serialized
}

// TestSetWidgetAction_ReplacesAction covers the FINDINGS gap: retargeting a
// button's action required REPLACEing the whole widget, which silently drops
// every property the author did not restate. Setting the action in place has to
// leave the rest of the widget alone.
func TestSetWidgetAction_ReplacesAction(t *testing.T) {
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnGo"},
		{Key: "Action", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowClientAction"},
			{Key: "Microflow", Value: "M.ACT_Save"},
		}},
		// The properties REPLACE would lose.
		{Key: "ButtonStyle", Value: "Primary"},
		{Key: "CaptionTemplate", Value: "Go"},
	}
	raw := makeRawPage(btn)
	newAction := bson.D{
		{Key: "$Type", Value: "Forms$MicroflowClientAction"},
		{Key: "Microflow", Value: "M.ACT_Other"},
	}
	m := New(raw, model.ID("unit-1"), &stubActionDeps{serialized: newAction})

	if err := m.SetWidgetAction("btnGo", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Other"}); err != nil {
		t.Fatalf("SetWidgetAction: %v", err)
	}

	widget := m.widgetFinder(m.rawData, "btnGo")
	if widget == nil {
		t.Fatal("btnGo not found after mutation")
	}
	action := bsonnav.DGetDoc(widget.widget, "Action")
	if action == nil {
		t.Fatal("Action missing after mutation")
	}
	if got := bsonnav.DGet(action, "Microflow"); got != "M.ACT_Other" {
		t.Errorf("Microflow = %v, want M.ACT_Other", got)
	}
	// The whole point: everything else survives.
	if got := bsonnav.DGet(widget.widget, "ButtonStyle"); got != "Primary" {
		t.Errorf("ButtonStyle = %v, want Primary — a set must not disturb sibling properties", got)
	}
	if got := bsonnav.DGet(widget.widget, "CaptionTemplate"); got != "Go" {
		t.Errorf("CaptionTemplate = %v, want Go", got)
	}
}

// TestSetWidgetAction_WritesTheWidgetsOwnClickKey is ako/mxcli#1503
// (mendixlabs/mxcli#1592): a widget's on-click action is not always stored as
// Action. A container keeps it in OnClickAction; a list view and both image
// viewers in ClickAction. `set (Action: …) on card1` was refused for a
// container CREATE had just written with an action, with a message naming "a
// clickable container" as a valid target.
//
// The write has to land in the key the type declares and add no other:
// Studio Pro throws on a stored property its type does not have, while mxbuild
// tolerates it, so an Action key on a container builds clean and fails to open.
func TestSetWidgetAction_WritesTheWidgetsOwnClickKey(t *testing.T) {
	for _, tc := range []struct {
		typeName, key string
	}{
		{"Forms$ActionButton", "Action"},
		{"Forms$DivContainer", "OnClickAction"},
		{"Forms$ListView", "ClickAction"},
		{"Forms$StaticImageViewer", "ClickAction"},
		{"Forms$DynamicImageViewer", "ClickAction"},
	} {
		t.Run(tc.typeName, func(t *testing.T) {
			w := bson.D{
				{Key: "$Type", Value: tc.typeName},
				{Key: "Name", Value: "w1"},
				{Key: tc.key, Value: bson.D{
					{Key: "$Type", Value: "Forms$MicroflowAction"},
					{Key: "Microflow", Value: "M.ACT_OpenOld"},
				}},
				{Key: "Class", Value: "card"},
			}
			raw := makeRawPage(w)
			newAction := bson.D{
				{Key: "$Type", Value: "Forms$MicroflowAction"},
				{Key: "Microflow", Value: "M.ACT_OpenNew"},
			}
			m := New(raw, model.ID("unit-1"), &stubActionDeps{serialized: newAction})

			if err := m.SetWidgetAction("w1", &pages.MicroflowClientAction{MicroflowName: "M.ACT_OpenNew"}); err != nil {
				t.Fatalf("SetWidgetAction: %v", err)
			}

			got := m.widgetFinder(m.rawData, "w1").widget
			if mf := bsonnav.DGet(bsonnav.DGetDoc(got, tc.key), "Microflow"); mf != "M.ACT_OpenNew" {
				t.Errorf("%s.Microflow = %v, want M.ACT_OpenNew", tc.key, mf)
			}
			for _, other := range []string{"Action", "OnClickAction", "ClickAction"} {
				if other != tc.key && bsonnav.DGet(got, other) != nil {
					t.Errorf("%s written on a %s, whose click action is %s", other, tc.typeName, tc.key)
				}
			}
			if len(got) != len(w) {
				t.Errorf("widget has %d keys after the set, want %d — a set adds no property", len(got), len(w))
			}
			if c := bsonnav.DGet(got, "Class"); c != "card" {
				t.Errorf("Class = %v, want card — a set must not disturb sibling properties", c)
			}
		})
	}
}

// TestSetWidgetAction_RefusesWidgetWithoutAction is the guard-don't-drop half.
//
// Studio Pro resolves every stored property against the type's property list and
// throws on one it does not know, while mxbuild's deserializer tolerates it — so
// writing an Action onto a widget with no click action would build clean and
// then fail to open. Refusing is the only safe answer, and the build is not a
// safety net here.
//
// The fixture used to be a container without an OnClickAction, a shape no model
// holds — every stored container has one — and it pinned the refusal #1503
// reported.
func TestSetWidgetAction_RefusesWidgetWithoutAction(t *testing.T) {
	text := bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Name", Value: "t1"},
	}
	raw := makeRawPage(text)
	m := New(raw, model.ID("unit-1"), &stubActionDeps{serialized: bson.D{{Key: "$Type", Value: "Forms$MicroflowAction"}}})

	err := m.SetWidgetAction("t1", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Save"})
	if err == nil {
		t.Fatal("expected an error setting Action on a dynamic text")
	}
	for _, want := range []string{"t1", "Forms$DynamicText", "no on-click action"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	// And nothing was written.
	widget := m.widgetFinder(m.rawData, "t1")
	if len(widget.widget) != len(text) {
		t.Errorf("widget has %d keys after a refused set, want %d", len(widget.widget), len(text))
	}
}

// TestSetWidgetAction_UnknownWidget keeps the not-found path a clear error.
func TestSetWidgetAction_UnknownWidget(t *testing.T) {
	raw := makeRawPage()
	m := New(raw, model.ID("unit-1"), &stubActionDeps{})
	err := m.SetWidgetAction("nope", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Save"})
	if err == nil {
		t.Fatal("expected an error for an unknown widget")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q does not name the widget", err.Error())
	}
}

// Compile-time reminder that the Mutator still satisfies the backend interface
// the new method was added to.
var _ backend.PageMutator = (*Mutator)(nil)
