// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1209: "describe drops widgets' design properties on 9.24, so a
// describe → exec round trip removes them", and rebuilt text boxes lose their
// placeholder.
//
// Studio Pro stored three properties in another shape before a version
// boundary — measured on the pages of a Studio Pro-authored 9.24.30 app — and
// describe read only the newer one:
//
//	design property   {Type: "Toggle"|"DropDown", StringValue, BooleanValue}   below 10.2
//	data view         Editable: true|false                                     below 10.7
//	text box          Placeholder: Texts$Text                                  below 10.11
//	data view source  PageParameter / SnippetParameter names                   below 10.0
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func mendix9Text(s string) map[string]any {
	return map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
		map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": s}}}
}

func flatDesignProperty(key, typ, str string, on bool) map[string]any {
	return map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": key,
		"Type": typ, "StringValue": str, "BooleanValue": on}
}

// mendix9DataView is a read-only data view holding a text box, both as Studio
// Pro 9.24.30 stores them.
func mendix9DataView() map[string]any {
	textBox := map[string]any{
		"$Type": "Forms$TextBox", "Name": "txtEmail",
		"AttributeRef": map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "M.Contact.Email"},
		"Appearance": map[string]any{"$Type": "Forms$Appearance", "DesignProperties": []any{int32(3),
			flatDesignProperty("Spacing bottom", "DropDown", "Outer small", false),
			flatDesignProperty("Hover style", "Toggle", "", true),
			flatDesignProperty("Full width", "Toggle", "", false), // a toggle that is off
		}},
		"Placeholder": mendix9Text("you@example.com"),
	}
	return map[string]any{
		"$Type": "Forms$DataView", "Name": "dv", "Editable": false,
		"DataSource": map[string]any{"$Type": "Forms$DataViewSource",
			"EntityRef":     map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "M.Contact"},
			"PageParameter": "Contact", "SnippetParameter": ""},
		"Widgets": []any{int32(2), textBox},
	}
}

func TestDescribe_ReadsTheMendix9Shapes(t *testing.T) {
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, mendix9DataView()) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	out := buf.String()
	for _, want := range []string{
		"DataSource: $Contact",
		"Editable: Never",
		"Placeholder: 'you@example.com'",
		"'Spacing bottom': 'Outer small'",
		"'Hover style': on",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe lacks %s, so describe → exec drops it on 9.24. got:\n%s", want, out)
		}
	}
	// A toggle stored off is not on.
	if strings.Contains(out, "Full width") {
		t.Errorf("describe printed a toggle that is stored off:\n%s", out)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + out + "}\n"); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, out)
	}
}

// Control: the same widgets in the 10.11+ shape describe the same way.
func TestDescribe_CurrentShapesUnchanged(t *testing.T) {
	dv := mendix9DataView()
	delete(dv, "Editable")
	dv["Editability"] = "Never"
	src := dv["DataSource"].(map[string]any)
	delete(src, "PageParameter")
	delete(src, "SnippetParameter")
	src["SourceVariable"] = map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Contact"}
	tb := dv["Widgets"].([]any)[1].(map[string]any)
	delete(tb, "Placeholder")
	tb["PlaceholderTemplate"] = map[string]any{"$Type": "Forms$ClientTemplate", "Template": mendix9Text("you@example.com")}
	tb["Appearance"].(map[string]any)["DesignProperties"] = []any{int32(3),
		map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": "Spacing bottom",
			"Value": map[string]any{"$Type": "Forms$OptionDesignPropertyValue", "Option": "Outer small"}},
		map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": "Hover style",
			"Value": map[string]any{"$Type": "Forms$ToggleDesignPropertyValue"}},
	}
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, dv) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	out := buf.String()
	for _, want := range []string{"DataSource: $Contact", "Editable: Never", "Placeholder: 'you@example.com'", "'Spacing bottom': 'Outer small'", "'Hover style': on"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe lacks %s for the 10.11+ shape. got:\n%s", want, out)
		}
	}
}
