// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#977: `alter page … set (ReadOnlyStyle: Text) on <input>` fell
// through to the pluggable-widget path and was refused as "not a pluggable
// widget" on every widget that stores the property. It is now written in place,
// canonically spelled, on each type that stores it.
func TestSetWidgetProperty_ReadOnlyStyle(t *testing.T) {
	for _, typ := range []string{
		"Forms$TextBox", "Forms$TextArea", "Forms$DatePicker",
		"Forms$RadioButtonGroup", "Forms$CheckBox", "Forms$DataView",
	} {
		t.Run(typ, func(t *testing.T) {
			rawData := makeRawPage(bson.D{
				{Key: "$Type", Value: typ},
				{Key: "Name", Value: "w1"},
				{Key: "ReadOnlyStyle", Value: "Inherit"},
			})
			m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
			if err := m.SetWidgetProperty("w1", "ReadOnlyStyle", "text"); err != nil {
				t.Fatalf("set ReadOnlyStyle: %v", err)
			}
			if got := bsonnav.DGet(findBsonWidget(rawData, "w1").widget, "ReadOnlyStyle"); got != "Text" {
				t.Fatalf("ReadOnlyStyle = %#v, want the canonical \"Text\"", got)
			}
			// An unknown member is refused before the document is touched.
			err := m.SetWidgetProperty("w1", "ReadOnlyStyle", "ReadOnly")
			if err == nil || !strings.Contains(err.Error(), "Inherit, Control, Text") {
				t.Fatalf("set ReadOnlyStyle = ReadOnly: err = %v, want a refusal naming the members", err)
			}
			if got := bsonnav.DGet(findBsonWidget(rawData, "w1").widget, "ReadOnlyStyle"); got != "Text" {
				t.Fatalf("a refused set changed ReadOnlyStyle to %#v", got)
			}
		})
	}
}

// A widget that stores no ReadOnlyStyle (a container) refuses the set.
func TestSetWidgetProperty_ReadOnlyStyleNoSuchProperty(t *testing.T) {
	rawData := makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "c1"},
	})
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("c1", "ReadOnlyStyle", "Text"); err == nil {
		t.Fatal("set ReadOnlyStyle on a container succeeded")
	}
}
