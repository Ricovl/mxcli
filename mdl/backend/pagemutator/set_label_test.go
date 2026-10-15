// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#292: "ALTER PAGE SET Title / SET Label silently succeeds but
// change does not persist". Title and a shown label were fixed earlier; these are
// the two shapes setWidgetLabelMut still answered with a bare nil — "Altered
// page", model unchanged.

// An input widget whose label is not shown stores `LabelTemplate: null` (the
// codec's registered null slot; Studio Pro writes the key on every input). SET
// Label has to create the template, not skip the widget.
func TestSetLabel_NullLabelTemplate_CreatesTemplate(t *testing.T) {
	withAuthoringLanguage(t, "en_US")
	tb := bson.D{
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Name", Value: "tbNoLabel"},
		{Key: "LabelTemplate", Value: nil},
	}
	raw := makeRawPage(tb)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("tbNoLabel", "Label", "Added"); err != nil {
		t.Fatalf("set Label on a label-less textbox: %v", err)
	}
	tmpl := bsonnav.DGetDoc(findBsonWidget(raw, "tbNoLabel").widget, "LabelTemplate")
	if tmpl == nil {
		t.Fatal("LabelTemplate still null after set Label: the label was silently dropped")
	}
	if typ := bsonnav.DGetString(tmpl, "$Type"); typ != "Forms$ClientTemplate" {
		t.Fatalf("LabelTemplate $Type = %q, want Forms$ClientTemplate", typ)
	}
	if got := translationsOf(t, bsonnav.DGetDoc(tmpl, "Template")); got["en_US"] != "Added" || len(got) != 1 {
		t.Fatalf("Template translations = %v, want en_US=Added", got)
	}
	// The shape create page writes: Fallback an empty Texts$Text, Parameters
	// the marker-2 empty list.
	if fb := bsonnav.DGetDoc(tmpl, "Fallback"); fb == nil || len(translationsOf(t, fb)) != 0 {
		t.Fatalf("Fallback = %v, want an empty Texts$Text", fb)
	}
	if p, ok := bsonnav.DGet(tmpl, "Parameters").(bson.A); !ok || len(p) != 1 || p[0] != int32(2) {
		t.Fatalf("Parameters = %v, want [2]", bsonnav.DGet(tmpl, "Parameters"))
	}
}

// A widget with no label slot at all (a button) used to report success with
// nothing written. It is refused by name, the way Caption is.
func TestSetLabel_WidgetWithoutLabel_IsRefused(t *testing.T) {
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnSave"},
	}
	m := &Mutator{rawData: makeRawPage(btn), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("btnSave", "Label", "X")
	if err == nil {
		t.Fatal("set Label on a button returned nil; it stored nothing")
	}
	if !strings.Contains(err.Error(), "no Label") {
		t.Fatalf("error = %v, want it to say the widget has no Label", err)
	}
}

// Every pluggable widget stores `LabelTemplate: null`, including ones whose
// definition has no Label system property, so the null is not license to add one.
func TestSetLabel_PluggableWithoutLabel_IsRefused(t *testing.T) {
	cw := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "accordion1"},
		{Key: "LabelTemplate", Value: nil},
	}
	raw := makeRawPage(cw)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("accordion1", "Label", "X"); err == nil {
		t.Fatal("set Label on a label-less pluggable widget returned nil")
	}
	if v := bsonnav.DGet(findBsonWidget(raw, "accordion1").widget, "LabelTemplate"); v != nil {
		t.Fatalf("LabelTemplate written on a refused set: %v", v)
	}
}
