// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// textBoxAsCreated is a Forms$TextBox in the shape CREATE writes for one with no
// placeholder and no validation: an empty PlaceholderTemplate, a
// Forms$WidgetValidation with an empty Expression and Message.
func textBoxAsCreated() bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Name", Value: "txtName"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		{Key: "IsPasswordBox", Value: false},
		{Key: "PlaceholderTemplate", Value: bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Fallback", Value: textsText()},
			{Key: "Parameters", Value: bson.A{int32(2)}},
			{Key: "Template", Value: textsText()},
		}},
		{Key: "Validation", Value: bson.D{
			{Key: "$Type", Value: "Forms$WidgetValidation"},
			{Key: "Expression", Value: ""},
			{Key: "Message", Value: textsText()},
		}},
	}
}

// mendixlabs/mxcli#414: `alter page … set Placeholder = '…' on <textbox>` was
// refused with "property \"Placeholder\" is not a property of this built-in
// widget", though CREATE writes `Placeholder:` and DESCRIBE prints it. The same
// held for the other textbox properties CREATE writes — Password, Validation,
// ValidationMessage — none of which had a case in setRawWidgetPropertyMut.
func TestSetTextBoxInputProperties(t *testing.T) {
	withAuthoringLanguage(t, "en_US")

	t.Run("Placeholder", func(t *testing.T) {
		raw := makeRawPage(textBoxAsCreated())
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("txtName", "Placeholder", "Enter your name"); err != nil {
			t.Fatal(err)
		}
		tmpl := bsonnav.DGetDoc(bsonnav.DGetDoc(findBsonWidget(raw, "txtName").widget, "PlaceholderTemplate"), "Template")
		if got := translationsOf(t, tmpl); got["en_US"] != "Enter your name" {
			t.Fatalf("placeholder translations = %v", got)
		}
	})

	t.Run("Password", func(t *testing.T) {
		raw := makeRawPage(textBoxAsCreated())
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("txtName", "Password", true); err != nil {
			t.Fatal(err)
		}
		if got := bsonnav.DGet(findBsonWidget(raw, "txtName").widget, "IsPasswordBox"); got != true {
			t.Fatalf("IsPasswordBox = %v, want true", got)
		}
		if err := m.SetWidgetProperty("txtName", "Password", "maybe"); err == nil {
			t.Fatal("set Password = 'maybe' returned nil; a password flag is a boolean")
		}
	})

	t.Run("Validation and ValidationMessage", func(t *testing.T) {
		raw := makeRawPage(textBoxAsCreated())
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("txtName", "Validation", "$value != empty"); err != nil {
			t.Fatal(err)
		}
		if err := m.SetWidgetProperty("txtName", "ValidationMessage", "Name is required"); err != nil {
			t.Fatal(err)
		}
		v := bsonnav.DGetDoc(findBsonWidget(raw, "txtName").widget, "Validation")
		if got := bsonnav.DGetString(v, "Expression"); got != "$value != empty" {
			t.Fatalf("Validation.Expression = %q", got)
		}
		if got := translationsOf(t, bsonnav.DGetDoc(v, "Message")); got["en_US"] != "Name is required" {
			t.Fatalf("Validation.Message translations = %v", got)
		}
	})

	// A built-in widget with no placeholder keeps the refusal that names what
	// it does take — the new case must not turn it into a silent success.
	t.Run("Placeholder on a widget without one is refused", func(t *testing.T) {
		cb := bson.D{
			{Key: "$Type", Value: "Forms$CheckBox"},
			{Key: "Name", Value: "chk"},
			{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		}
		m := &Mutator{rawData: makeRawPage(cb), widgetFinder: findBsonWidget}
		err := m.SetWidgetProperty("chk", "Placeholder", "x")
		if err == nil || !strings.Contains(err.Error(), "not a property of this built-in widget") {
			t.Fatalf("err = %v, want the built-in widget refusal", err)
		}
	})
}
