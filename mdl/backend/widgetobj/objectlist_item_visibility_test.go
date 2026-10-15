// SPDX-License-Identifier: Apache-2.0

// Issue mendixlabs/mxcli#1198: a File Uploader `allowedfileformat` item in the
// default `configMode: 'simple'` passed check and exec, then failed `mx check`
// with CE0463 "the definition of this widget has changed".
//
// The widget's editorConfig hides the item's `typeFormatDescription`
// TextTemplate in simple mode, per item:
//
//	values.allowedFileFormats.forEach((p, index) => {
//	    if (p.configMode === "simple")
//	        hideNestedPropertiesIn(properties, values, "allowedFileFormats", index,
//	            ["mimeType", "extensions", "typeFormatDescription"]);
//	    ...
//
// The rule was extracted (ListPropertyKey "allowedFileFormats"), but
// ApplyVisibilityRules only looked at top-level properties, so the hidden
// item template kept the populated ClientTemplate the required-TextTemplate
// default (#891) gives it. `mx update-widgets` on a copy changes exactly that
// slot to null (11.6.6), which is the #574 rule one level down.
package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

const (
	fuListID       = "a0000000-0000-0000-0000-000000000001" // object list "allowedFileFormats"
	fuObjTypeID    = "a0000000-0000-0000-0000-000000000002"
	fuConfigModeID = "a0000000-0000-0000-0000-000000000003" // item enum "configMode"
	fuDescID       = "a0000000-0000-0000-0000-000000000004" // item TextTemplate "typeFormatDescription"
	fuTopDescID    = "a0000000-0000-0000-0000-000000000005" // top-level TextTemplate sharing the item key
	fuModeID       = "a0000000-0000-0000-0000-000000000006" // widget enum "uploadMode"
	fuCaptionID    = "a0000000-0000-0000-0000-000000000007" // item TextTemplate "caption" (widget-scoped rule)
)

func fuPropertyTypeIDs() map[string]pages.PropertyTypeIDEntry {
	return map[string]pages.PropertyTypeIDEntry{
		"uploadMode": {PropertyTypeID: fuModeID, ValueType: "Enumeration"},
		// A top-level property whose key collides with the item's: a nested rule
		// must never be applied to it.
		"typeFormatDescription": {PropertyTypeID: fuTopDescID, ValueType: "TextTemplate"},
		"allowedFileFormats": {
			PropertyTypeID: fuListID,
			ValueType:      "Object",
			ObjectTypeID:   fuObjTypeID,
			NestedKeyOrder: []string{"configMode", "typeFormatDescription", "caption"},
			NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
				"configMode":            {PropertyTypeID: fuConfigModeID, ValueType: "Enumeration", DefaultValue: "simple"},
				"typeFormatDescription": {PropertyTypeID: fuDescID, ValueType: "TextTemplate", Required: true},
				"caption":               {PropertyTypeID: fuCaptionID, ValueType: "TextTemplate", Required: true},
			},
		},
	}
}

func fuProp(id, primitive string, tt any, objects bson.A) bson.D {
	val := bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "PrimitiveValue", Value: primitive},
		{Key: "TextTemplate", Value: tt},
	}
	if objects != nil {
		val = append(val, bson.E{Key: "Objects", Value: objects})
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(id)},
		{Key: "Value", Value: val},
	}
}

func fuItem(configMode string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "TypePointer", Value: types.UUIDToBlob(fuObjTypeID)},
		{Key: "Properties", Value: bson.A{
			int32(2),
			fuProp(fuConfigModeID, configMode, nil, nil),
			fuProp(fuDescID, "", BuildEmptyClientTemplate(), nil),
			fuProp(fuCaptionID, "", BuildEmptyClientTemplate(), nil),
		}},
	}
}

func fuObject(uploadMode string, items ...bson.D) bson.D {
	objects := bson.A{int32(2)}
	for _, it := range items {
		objects = append(objects, it)
	}
	return bson.D{{Key: "Properties", Value: bson.A{
		int32(2),
		fuProp(fuModeID, uploadMode, nil, nil),
		fuProp(fuTopDescID, "", BuildEmptyClientTemplate(), nil),
		fuProp(fuListID, "", nil, objects),
	}}}
}

// itemTemplate returns the TextTemplate of the item sub-property propID in the
// n-th (0-based) item of the allowedFileFormats list.
func itemTemplate(t *testing.T, object bson.D, n int, propID string) any {
	t.Helper()
	for _, elem := range object {
		if elem.Key != "Properties" {
			continue
		}
		for _, p := range elem.Value.(bson.A) {
			prop, ok := p.(bson.D)
			if !ok || propertyTypePointerID(prop) != normalizeID(fuListID) {
				continue
			}
			val := findField(t, prop, "Value").(bson.D)
			objects := findField(t, val, "Objects").(bson.A)
			item := objects[n+1].(bson.D) // [0] is the list marker
			return templateOf(t, item, propID)
		}
	}
	t.Fatalf("allowedFileFormats not found")
	return nil
}

func TestApplyVisibilityRules_ObjectListItems(t *testing.T) {
	rules := []types.WidgetVisibilityRule{
		{
			PropertyKey:     "typeFormatDescription",
			ListPropertyKey: "allowedFileFormats",
			HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "configMode", Operator: "eq", Value: "simple", Scope: types.ConditionScopeItem,
			},
		},
		// A nested rule whose condition is about the WIDGET, not the item.
		{
			PropertyKey:     "caption",
			ListPropertyKey: "allowedFileFormats",
			HiddenWhen:      &types.WidgetVisibilityCondition{PropertyKey: "uploadMode", Operator: "ne", Value: "files"},
		},
	}

	got := ApplyVisibilityRules(fuObject("files", fuItem("simple"), fuItem("advanced")), fuPropertyTypeIDs(), rules)

	if tt := itemTemplate(t, got, 0, fuDescID); tt != nil {
		t.Errorf("simple-mode item: typeFormatDescription TextTemplate = %v, want nil "+
			"(hidden when configMode = simple; a populated template is CE0463, #1198)", tt)
	}
	if tt := itemTemplate(t, got, 1, fuDescID); tt == nil {
		t.Error("advanced-mode item: typeFormatDescription TextTemplate was nulled; it is visible there")
	}
	// The item-scoped condition is evaluated per ITEM, not against the widget.
	if tt := templateOf(t, got, fuTopDescID); tt == nil {
		t.Error("top-level typeFormatDescription was nulled by a NESTED rule; nested rules only address list items")
	}
	// Widget-scoped condition inside a nested rule: uploadMode is "files", so visible.
	if tt := itemTemplate(t, got, 0, fuCaptionID); tt == nil {
		t.Error("caption nulled although its widget-scoped condition (uploadMode ≠ files) does not hold")
	}

	got = ApplyVisibilityRules(fuObject("images", fuItem("advanced")), fuPropertyTypeIDs(), rules)
	if tt := itemTemplate(t, got, 0, fuCaptionID); tt != nil {
		t.Error("caption kept its template although its widget-scoped condition (uploadMode ≠ files) holds")
	}
}
