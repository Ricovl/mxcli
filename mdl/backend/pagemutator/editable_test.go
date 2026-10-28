// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1214: `alter page … set Editable = true on <listview>`
// reported "Altered page" and stored nothing. A list view's (and data view's)
// Editable is a BOOLEAN; the setter only wrote string values (the input-widget
// enum), so a boolean fell through and returned nil.
func TestSetWidgetProperty_Editable_ListViewBoolean(t *testing.T) {
	for _, typ := range []string{"Forms$ListView", "Forms$DataView"} {
		t.Run(typ, func(t *testing.T) {
			w := bson.D{
				{Key: "$Type", Value: typ},
				{Key: "Name", Value: "lv"},
				{Key: "Editable", Value: false},
			}
			rawData := makeRawPage(w)
			m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
			if err := m.SetWidgetProperty("lv", "Editable", true); err != nil {
				t.Fatalf("set Editable = true: %v", err)
			}
			if got := bsonnav.DGet(findBsonWidget(rawData, "lv").widget, "Editable"); got != true {
				t.Fatalf("Editable = %#v after set Editable = true, want true", got)
			}
			// Back to false, so the test is not satisfied by a setter that only ever writes true.
			if err := m.SetWidgetProperty("lv", "editable", false); err != nil {
				t.Fatalf("set Editable = false: %v", err)
			}
			if got := bsonnav.DGet(findBsonWidget(rawData, "lv").widget, "Editable"); got != false {
				t.Fatalf("Editable = %#v after set Editable = false, want false", got)
			}
			// The input-widget enum is not a list view's vocabulary: refuse, never no-op.
			if err := m.SetWidgetProperty("lv", "Editable", "Never"); err == nil {
				t.Fatal("set Editable = 'Never' on a list view succeeded; want an error naming true/false")
			}
		})
	}
}

// An input widget's Editable is the Always/Never/Conditional enum. A boolean
// used to be dropped silently; a lower-case enum was stored verbatim, which is
// not a value Studio Pro reads.
func TestSetWidgetProperty_Editable_InputEnum(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Name", Value: "tb"},
		{Key: "Editable", Value: "Always"},
		{Key: "ConditionalEditabilitySettings", Value: nil},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("tb", "Editable", "never"); err != nil {
		t.Fatalf("set Editable = 'never': %v", err)
	}
	if got := bsonnav.DGet(findBsonWidget(rawData, "tb").widget, "Editable"); got != "Never" {
		t.Fatalf("Editable = %#v, want the canonical \"Never\"", got)
	}
	if err := m.SetWidgetProperty("tb", "Editable", true); err == nil {
		t.Fatal("set Editable = true on a text box succeeded; want an error naming Always/Never")
	}
	if err := m.SetWidgetProperty("tb", "Editable", "Conditional"); err == nil {
		t.Fatal("set Editable = 'Conditional' succeeded with no expression; want an error pointing at [expr]")
	}

	// EDITABLE IF writes the settings element AND the enum that says it applies,
	// as CREATE does (pages.WidgetEditability). Setting a plain value afterwards
	// clears the element again, so the two never contradict each other.
	if err := m.SetWidgetProperty("tb", "EditableIf", "$currentObject/Active"); err != nil {
		t.Fatalf("set EditableIf: %v", err)
	}
	got := findBsonWidget(rawData, "tb").widget
	if e := bsonnav.DGet(got, "Editable"); e != "Conditional" {
		t.Fatalf("Editable = %#v after set Editable = [expr], want \"Conditional\"", e)
	}
	if err := m.SetWidgetProperty("tb", "Editable", "Always"); err != nil {
		t.Fatalf("set Editable = 'Always': %v", err)
	}
	got = findBsonWidget(rawData, "tb").widget
	if e := bsonnav.DGet(got, "Editable"); e != "Always" {
		t.Fatalf("Editable = %#v, want \"Always\"", e)
	}
	if s := bsonnav.DGet(got, "ConditionalEditabilitySettings"); s != nil {
		t.Fatalf("ConditionalEditabilitySettings = %#v after set Editable = 'Always', want null", s)
	}
}

// A widget with no Editable property at all (a container) refuses the set.
func TestSetWidgetProperty_Editable_NoSuchProperty(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "ctn"},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("ctn", "Editable", true); err == nil {
		t.Fatal("set Editable on a container succeeded; it has no Editable property")
	}
}

// ako/mxcli#1676: from Mendix 10.7 a data view stores the Always/Never/
// Conditional enum as `Editability` and has no `Editable` at all (Model SDK:
// Forms$DataView `editable` Deleted 10.7.0, `editability` Introduced 10.7.0;
// all 41 data views in the Studio Pro-authored PedApp fixture). The setter only
// looked at `Editable`, so `set Editable = Never on dataView1` was refused as
// "a data view has no Editable property".
func TestSetWidgetProperty_Editable_DataViewEditability(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dataView1"},
		{Key: "Editability", Value: "Always"},
		{Key: "ConditionalEditabilitySettings", Value: nil},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	stored := func() bson.D { return findBsonWidget(rawData, "dataView1").widget }

	if err := m.SetWidgetProperty("dataView1", "Editable", "Never"); err != nil {
		t.Fatalf("set Editable = Never on a data view: %v", err)
	}
	if got := bsonnav.DGet(stored(), "Editability"); got != "Never" {
		t.Fatalf("Editability = %#v, want \"Never\"", got)
	}
	if got := bsonnav.DGet(stored(), "Editable"); got != nil {
		t.Fatalf("the setter added an `Editable` key (%#v) that a 10.7+ data view does not declare", got)
	}
	if err := m.SetWidgetProperty("dataView1", "EditableIf", "$currentObject/Open"); err != nil {
		t.Fatalf("set EditableIf: %v", err)
	}
	if got := bsonnav.DGet(stored(), "Editability"); got != "Conditional" {
		t.Fatalf("Editability = %#v after set Editable = [expr], want \"Conditional\"", got)
	}
	if err := m.SetWidgetProperty("dataView1", "Editable", "Always"); err != nil {
		t.Fatalf("set Editable = Always: %v", err)
	}
	if got := bsonnav.DGet(stored(), "Editability"); got != "Always" {
		t.Fatalf("Editability = %#v, want \"Always\"", got)
	}
	if s := bsonnav.DGet(stored(), "ConditionalEditabilitySettings"); s != nil {
		t.Fatalf("ConditionalEditabilitySettings = %#v after set Editable = Always, want null", s)
	}
}

// ako/mxcli#1677: `alter page … set (NativeTextStyle: …)` on a dynamic text
// writes the stored enum in place, canonically spelled, and refuses a value
// outside it.
func TestSetWidgetProperty_NativeTextStyle(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Name", Value: "text4"},
		{Key: "RenderMode", Value: "Text"},
		{Key: "NativeTextStyle", Value: "Text"},
	}
	rawData := makeRawPage(w)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("text4", "NativeTextStyle", "heading3"); err != nil {
		t.Fatalf("set NativeTextStyle = heading3: %v", err)
	}
	if got := bsonnav.DGet(findBsonWidget(rawData, "text4").widget, "NativeTextStyle"); got != "Heading3" {
		t.Fatalf("NativeTextStyle = %#v, want \"Heading3\"", got)
	}
	if err := m.SetWidgetProperty("text4", "NativeTextStyle", "H3"); err == nil {
		t.Fatal("set NativeTextStyle = H3 succeeded; want an error naming the valid styles")
	}
}
