// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// mendixlabs/mxcli#1424: a Data grid 2 written by mxcli to a blank 10.24.0
// project (Data Widgets 2.30.3) failed `mx check` with CE0463 until
// `fix widgets` ran. The embedded template is Data Widgets 3.4-shaped, so
// augmentation has to rebuild the 2.30.3 definition, and diffing mxcli's Type
// against `mx update-widgets` output showed it got three things wrong:
//
//  1. A column's `filter` (widgets) took the definition of the `filterList`
//     item's `filter` (attribute): the reconcile passes indexed PropertyDefs by
//     bare key across every nesting level, so the later declaration won.
//  2. The new `filterList` object list had no ObjectType: exemplar indices were
//     taken before the stale properties were removed, so the clone came from
//     whatever property had shifted into the Object exemplar's slot (an Integer).
//  3. Association definition fields were never reconciled: the parser did not
//     read <associationTypes> or selectableObjects, so the column's
//     `filterAssociation` had none and `filterAssociationOptionLabelAttr` kept
//     the [Reference, ReferenceSet] of the attribute it was cloned from.
//
// The test builds that package shape from the real embedded template: same
// properties, minus one stale top-level property AHEAD of `columns`, plus the
// 2.30.3-only properties.
func TestAugmentTemplate_DataWidgets2303Shape(t *testing.T) {
	const widgetID = "com.mendix.widget.web.datagrid.Datagrid"
	tmpl, err := GetTemplate(widgetID)
	if err != nil || tmpl == nil {
		t.Fatalf("GetTemplate(%s) = %v, %v", widgetID, tmpl, err)
	}
	ResetPlaceholderCounter()
	clone, err := deepCloneTemplate(tmpl)
	if err != nil {
		t.Fatal(err)
	}

	def := &mpk.WidgetDefinition{ID: widgetID, Version: "2.30.3"}
	propTypes := clone.Type["ObjectType"].(map[string]any)["PropertyTypes"].([]any)
	droppedStale := ""
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		pd := propertyDefFromType(ptMap)
		if pd.IsSystem {
			def.SystemProps = append(def.SystemProps, pd)
			continue
		}
		// Drop the first ordinary property before `columns`: stale in this
		// package, and its removal shifts every exemplar index behind it.
		if droppedStale == "" && pd.Key != "columns" {
			droppedStale = pd.Key
			continue
		}
		if pd.Key == "exportDialogLabel" {
			pd.Translations = []mpk.Translation{{Lang: "en_US", Text: "Export progress"}}
		}
		if pd.Key == "columns" {
			pd.Children = append(pd.Children,
				mpk.PropertyDef{Key: "filterAssociation", Type: "association", Caption: "Entity",
					DataSource: "../datasource", SelectableObjects: "filterAssociationOptions",
					AssociationTypes: []string{"Reference", "ReferenceSet"}},
				mpk.PropertyDef{Key: "filterAssociationOptions", Type: "datasource", Caption: "Selectable objects", IsList: true},
				mpk.PropertyDef{Key: "filterAssociationOptionLabelAttr", Type: "attribute", Caption: "Option caption",
					DataSource: "filterAssociationOptions", AllowedTypes: []string{"String"}},
			)
		}
		def.Properties = append(def.Properties, pd)
	}
	if droppedStale == "" {
		t.Fatal("no top-level property found ahead of columns")
	}
	def.Properties = append(def.Properties, mpk.PropertyDef{
		Key: "filterList", Type: "object", Caption: "Filters", IsList: true,
		Children: []mpk.PropertyDef{{
			Key: "filter", Type: "attribute", Caption: "Filter attribute", Required: true,
			DataSource: "../datasource", AllowedTypes: []string{"String", "Integer"},
		}},
	})

	if err := AugmentTemplate(clone, def); err != nil {
		t.Fatalf("AugmentTemplate: %v", err)
	}

	got := map[string]map[string]any{}
	collectByPath(clone.Type["ObjectType"].(map[string]any), "", got)
	vt := func(scoped string) map[string]any {
		t.Helper()
		pt, ok := got[scoped]
		if !ok {
			t.Errorf("PropertyType %q missing after augmentation", scoped)
			return map[string]any{}
		}
		return pt["ValueType"].(map[string]any)
	}

	// 1. The column's filter keeps ITS definition.
	if typ := vt("columns/filter")["Type"]; typ != "Widgets" {
		t.Errorf("columns/filter Type = %v, want Widgets (took filterList/filter's definition)", typ)
	}
	if c := got["columns/filter"]["Caption"]; c == "Filter attribute" {
		t.Errorf("columns/filter Caption = %q, the filterList item's caption", c)
	}
	if typ := vt("filterList/filter")["Type"]; typ != "Attribute" {
		t.Errorf("filterList/filter Type = %v, want Attribute", typ)
	}

	// 2. The new object list has its nested ObjectType.
	if vt("filterList")["ObjectType"] == nil {
		t.Error("filterList ObjectType = nil: cloned from a shifted (non-Object) exemplar")
	}

	// 3. Association definition fields come from the package.
	if at := vt("columns/filterAssociation")["AssociationTypes"]; !reflect.DeepEqual(at, []any{float64(1), "Reference", "ReferenceSet"}) {
		t.Errorf("columns/filterAssociation AssociationTypes = %v, want [1 Reference ReferenceSet]", at)
	}
	if so := vt("columns/filterAssociation")["SelectableObjectsProperty"]; so != "filterAssociationOptions" {
		t.Errorf("columns/filterAssociation SelectableObjectsProperty = %q, want filterAssociationOptions", so)
	}
	if at := vt("columns/filterAssociationOptionLabelAttr")["AssociationTypes"]; !reflect.DeepEqual(at, []any{float64(1)}) {
		t.Errorf("columns/filterAssociationOptionLabelAttr AssociationTypes = %v, want [1] (none declared)", at)
	}

	// 4. Translations follow the package in both directions: the template's are
	// dropped where the package ships none (a 2.30.3 drop-down filter's
	// emptyOptionCaption), replaced where it ships its own.
	if tr := vt("loadMoreButtonCaption")["Translations"]; !reflect.DeepEqual(tr, []any{float64(2)}) {
		t.Errorf("loadMoreButtonCaption Translations = %v, want [2] (package declares none)", tr)
	}
	if tr, _ := vt("exportDialogLabel")["Translations"].([]any); len(tr) != 2 || tr[1].(map[string]any)["Text"] != "Export progress" {
		t.Errorf("exportDialogLabel Translations = %v, want the package's one translation", tr)
	}
}

// collectByPath indexes an ObjectType's PropertyTypes as "key" / "list/key".
func collectByPath(objType map[string]any, prefix string, out map[string]map[string]any) {
	pts, _ := objType["PropertyTypes"].([]any)
	for _, pt := range pts {
		m, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["PropertyKey"].(string)
		if prefix != "" {
			path = prefix + "/" + path
		}
		out[path] = m
		if vt, ok := m["ValueType"].(map[string]any); ok {
			if ot, ok := vt["ObjectType"].(map[string]any); ok {
				collectByPath(ot, path, out)
			}
		}
	}
}

// propertyDefFromType mirrors a template PropertyType as the PropertyDef a
// package declaring it unchanged would yield, recursing into object lists.
func propertyDefFromType(pt map[string]any) mpk.PropertyDef {
	key, _ := pt["PropertyKey"].(string)
	vt, _ := pt["ValueType"].(map[string]any)
	vtType, _ := vt["Type"].(string)
	if vtType == "System" {
		return mpk.PropertyDef{Key: key, IsSystem: true}
	}
	pd := mpk.PropertyDef{Key: key, Type: bsonTypeToXmlType(vtType)}
	pd.Caption, _ = pt["Caption"].(string)
	pd.IsList, _ = vt["IsList"].(bool)
	pd.Required, _ = vt["Required"].(bool)
	pd.DataSource, _ = vt["DataSourceProperty"].(string)
	pd.SelectableObjects, _ = vt["SelectableObjectsProperty"].(string)
	if at, ok := vt["AssociationTypes"].([]any); ok {
		for _, a := range at {
			if s, ok := a.(string); ok {
				pd.AssociationTypes = append(pd.AssociationTypes, s)
			}
		}
	}
	if ot, ok := vt["ObjectType"].(map[string]any); ok {
		for _, child := range ot["PropertyTypes"].([]any) {
			if cm, ok := child.(map[string]any); ok {
				pd.Children = append(pd.Children, propertyDefFromType(cm))
			}
		}
	}
	return pd
}
