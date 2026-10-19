// SPDX-License-Identifier: Apache-2.0

// DESCRIBE has to print a Data grid 2's settings that MDL has no keyword for,
// under the key the widget declares, or a describe -> exec cycle resets each of
// them to the widget's default.
//
//	datagrid dg (DataSource: database Shop.Ord, configurationStorageType: localStorage)
//
// exec stores that, `describe page` printed only the keyword properties
// (PageSize, Pagination, …), and re-executing the description wrote the default
// back — silently, at `mx check` 0 errors.
//
// The fixtures here are the shipped Data grid 2 template, not a hand-written
// list of its properties, so "an untouched grid prints nothing" is checked
// against the widget's real schema and real default values.
package executor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/widgets"
)

const dataGridWidgetID = "com.mendix.widget.web.datagrid.Datagrid"

// normalizeTemplateJSON turns a decoded JSON template into the shape a decoded
// BSON document has: the typed-array marker that opens every array is an int,
// not the float64 encoding/json makes of it.
func normalizeTemplateJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeTemplateJSON(e)
		}
		return x
	case []any:
		for i, e := range x {
			if f, ok := e.(float64); ok && i == 0 {
				x[i] = int(f)
				continue
			}
			x[i] = normalizeTemplateJSON(e)
		}
		return x
	}
	return v
}

// dataGridFromTemplate is an untouched Data grid 2: the shipped template's
// schema and the values exec writes for a property left out.
func dataGridFromTemplate(t *testing.T) map[string]any {
	t.Helper()
	tmpl, err := widgets.GetTemplate(dataGridWidgetID)
	if err != nil || tmpl == nil {
		t.Fatalf("Data grid 2 template: %v, %v", tmpl, err)
	}
	raw, err := json.Marshal(map[string]any{"Type": tmpl.Type, "Object": tmpl.Object})
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	normalizeTemplateJSON(w)
	w["$Type"] = "CustomWidgets$CustomWidget"
	w["Name"] = "dg"
	return w
}

// gridValue is the stored Value of the grid-level property key.
func gridValue(t *testing.T, w map[string]any, key string) map[string]any {
	t.Helper()
	ids := buildPropertyTypeKeyMap(w, false)
	for _, p := range getBsonArrayElements(w["Object"].(map[string]any)["Properties"]) {
		pm := p.(map[string]any)
		if ids[extractBinaryID(pm["TypePointer"])] == key {
			return pm["Value"].(map[string]any)
		}
	}
	t.Fatalf("template has no property %q", key)
	return nil
}

func describeGrid(t *testing.T, w map[string]any, enclosing string) rawWidget {
	t.Helper()
	ctx := (&Executor{}).newExecContext(nil)
	got := parseRawWidget(ctx, w, enclosing)
	if len(got) != 1 {
		t.Fatalf("parseRawWidget returned %d widgets", len(got))
	}
	return got[0]
}

func explicitByKey(ps []rawExplicitProp) map[string]rawExplicitProp {
	m := map[string]rawExplicitProp{}
	for _, p := range ps {
		m[p.Key] = p
	}
	return m
}

func explicitKeyList(ps []rawExplicitProp) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Key)
	}
	return out
}

// An untouched grid describes exactly as before: every value the template holds
// is the widget's default, the text templates hold the text the widget ships
// for them, and a template hidden by the editor config is null.
func TestDataGridByKey_UntouchedGridPrintsNothing(t *testing.T) {
	got := describeGrid(t, dataGridFromTemplate(t), "")
	if len(got.ExplicitProperties) != 0 {
		t.Errorf("untouched grid printed %v; its description must not change", explicitKeyList(got.ExplicitProperties))
	}
	if len(got.OmittedProps) != 0 {
		t.Errorf("untouched grid reported omitted properties %v", got.OmittedProps)
	}
}

// Every kind of setting a grid has, set away from its default, is printed under
// its own key; the ones left alone are not.
func TestDataGridByKey_PrintsEachNonDefaultKind(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "configurationStorageType")["PrimitiveValue"] = "localStorage" // enumeration
	gridValue(t, w, "columnsHidable")["PrimitiveValue"] = "false"                  // boolean
	gridValue(t, w, "refreshInterval")["PrimitiveValue"] = "30"                    // integer
	gridValue(t, w, "rowClass")["Expression"] = "if $currentObject/Open then 'a' else 'b'"
	gridValue(t, w, "exportDialogLabel")["TextTemplate"].(map[string]any)["Template"].(map[string]any)["Items"].([]any)[1].(map[string]any)["Text"] = "Saving"
	gridValue(t, w, "configurationAttribute")["AttributeRef"] = map[string]any{"Attribute": "Shop.GridPref.Config"}

	got := describeGrid(t, w, "Shop.GridPref")
	props := explicitByKey(got.ExplicitProperties)

	want := map[string]string{
		"configurationStorageType": "'localStorage'",
		"columnsHidable":           "false",
		"refreshInterval":          "30",
		"rowClass":                 "if $currentObject/Open then 'a' else 'b'",
		"exportDialogLabel":        "'Saving'",
		"configurationAttribute":   "Config",
	}
	ctx := (&Executor{}).newExecContext(nil)
	for key, wantValue := range want {
		p, ok := props[key]
		if !ok {
			t.Errorf("%s was not printed; got %v", key, explicitKeyList(got.ExplicitProperties))
			continue
		}
		if v := explicitPropValue(ctx, p); v != wantValue {
			t.Errorf("%s printed as %s, want %s", key, v, wantValue)
		}
	}
	if len(got.ExplicitProperties) != len(want) {
		t.Errorf("printed %v, want exactly %d settings", explicitKeyList(got.ExplicitProperties), len(want))
	}
}

// The attribute is written bare where exec resolves a bare name — inside a data
// container — and qualified where it would not: there is nothing to resolve a
// bare name against on a page-level grid.
func TestDataGridByKey_AttributeIsBareOnlyInsideAContainer(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "configurationAttribute")["AttributeRef"] = map[string]any{"Attribute": "Shop.GridPref.Config"}

	if v := explicitByKey(describeGrid(t, w, "Shop.GridPref").ExplicitProperties)["configurationAttribute"].Value; v != "Config" {
		t.Errorf("inside a data view over the entity: %q, want Config", v)
	}
	// A page parameter's data view names no entity; the property still binds to
	// that container's entity.
	if v := explicitByKey(describeGrid(t, w, "Pref").ExplicitProperties)["configurationAttribute"].Value; v != "Config" {
		t.Errorf("inside a data view over a page parameter: %q, want Config", v)
	}
	if v := explicitByKey(describeGrid(t, w, "").ExplicitProperties)["configurationAttribute"].Value; v != "Shop.GridPref.Config" {
		t.Errorf("on a page-level grid: %q, want the qualified name", v)
	}
	if v := explicitByKey(describeGrid(t, w, "Shop.Other").ExplicitProperties)["configurationAttribute"].Value; v != "Shop.GridPref.Config" {
		t.Errorf("attribute of another entity than the container's: %q, want the qualified name", v)
	}
}

// What MDL already spells another way must not be printed twice: writing
// `PageSize: 25` and `pageSize: 25` would put two values on one property.
func TestDataGridByKey_SkipsWhatHasAKeywordOfItsOwn(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "pageSize")["PrimitiveValue"] = "25"
	gridValue(t, w, "pagination")["PrimitiveValue"] = "virtualScrolling"
	gridValue(t, w, "pagingPosition")["PrimitiveValue"] = "top"
	gridValue(t, w, "showPagingButtons")["PrimitiveValue"] = "auto"
	// A Selection-typed property is carried by `Selection:`; and a stray
	// PrimitiveValue on it is not a setting.
	gridValue(t, w, "itemSelection")["Selection"] = "Multi"

	got := describeGrid(t, w, "")
	if len(got.ExplicitProperties) != 0 {
		t.Errorf("printed %v for keys MDL already has a keyword for", explicitKeyList(got.ExplicitProperties))
	}
	if got.PageSize != "25" || got.Pagination != "virtualScrolling" || got.Selection != "Multi" {
		t.Errorf("the keyword properties were not read: PageSize=%q Pagination=%q Selection=%q",
			got.PageSize, got.Pagination, got.Selection)
	}
}

// An expression that is only a string literal would, unquoted, be read back as
// the string and stored without its quotes — a different expression. It is
// printed as the quoted text of the expression instead.
func TestDataGridByKey_ExpressionThatIsAStringLiteralIsQuoted(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "rowClass")["Expression"] = "'is-open'"

	ctx := (&Executor{}).newExecContext(nil)
	p := explicitByKey(describeGrid(t, w, "").ExplicitProperties)["rowClass"]
	if got, want := explicitPropValue(ctx, p), `'''is-open'''`; got != want {
		t.Errorf("rowClass printed as %s, want %s", got, want)
	}
}

// A path over associations has no spelling the explicit-key pass reads back. It
// must not be printed as if it had one, and must not vanish.
func TestDataGridByKey_AttributeOverAnAssociationIsNamedNotDropped(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "configurationAttribute")["AttributeRef"] = map[string]any{
		"Attribute": "Shop.GridPref.Config",
		"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef"},
	}

	got := describeGrid(t, w, "Shop.Ord")
	if _, printed := explicitByKey(got.ExplicitProperties)["configurationAttribute"]; printed {
		t.Errorf("printed an attribute path the writer cannot read back")
	}
	if len(got.OmittedProps) != 1 || got.OmittedProps[0] != "configurationAttribute" {
		t.Errorf("OmittedProps = %v, want [configurationAttribute]", got.OmittedProps)
	}

	var buf bytes.Buffer
	writeOmittedPropsNote(&buf, "  ", got.OmittedProps)
	if out := buf.String(); !strings.Contains(out, "-- NOT re-executable: configurationAttribute") {
		t.Errorf("note does not name the property:\n%s", out)
	}
}

// A column's own settings follow the same rule, minus the keys describe already
// reads under their MDL names.
func TestDataGridByKey_ColumnSettings(t *testing.T) {
	w := dataGridFromTemplate(t)

	schemas := objectListPropertySchemas(w, "columns")
	if len(schemas) == 0 {
		t.Fatal("no column schema in the template")
	}
	// The template's columns object-list holds one default column.
	var column map[string]any
	for _, p := range getBsonArrayElements(w["Object"].(map[string]any)["Properties"]) {
		pm := p.(map[string]any)
		if objs := getBsonArrayElements(pm["Value"].(map[string]any)["Objects"]); len(objs) > 0 {
			column = objs[0].(map[string]any)
		}
	}
	if column == nil {
		t.Skip("template carries no default column")
	}

	if props, omitted := extractDataGrid2ColumnByKeyProps(nil, column, schemas, "Shop.Ord"); len(props) != 0 || len(omitted) != 0 {
		t.Errorf("untouched column printed %v / omitted %v", explicitKeyList(props), omitted)
	}

	setColumn := func(key, field string, value any) {
		ids := map[string]string{}
		for id, sc := range schemas {
			ids[id] = sc.Key
		}
		for _, p := range getBsonArrayElements(column["Properties"]) {
			pm := p.(map[string]any)
			if ids[extractBinaryID(pm["TypePointer"])] == key {
				pm["Value"].(map[string]any)[field] = value
				return
			}
		}
		t.Fatalf("column has no property %q", key)
	}
	setColumn("minWidth", "PrimitiveValue", "manual")
	setColumn("minWidthLimit", "PrimitiveValue", "150")
	setColumn("allowEventPropagation", "PrimitiveValue", "false")
	// Read under their own names by extractDataGrid2Column: not repeated.
	setColumn("alignment", "PrimitiveValue", "right")
	setColumn("size", "PrimitiveValue", "3")

	props, _ := extractDataGrid2ColumnByKeyProps(nil, column, schemas, "Shop.Ord")
	got := explicitByKey(props)
	for key, want := range map[string]string{"minWidth": "manual", "minWidthLimit": "150", "allowEventPropagation": "false"} {
		if got[key].Value != want {
			t.Errorf("column %s = %q, want %q (printed %v)", key, got[key].Value, want, explicitKeyList(props))
		}
	}
	for _, key := range []string{"alignment", "size"} {
		if _, dup := got[key]; dup {
			t.Errorf("column %s printed by key as well as under its MDL name", key)
		}
	}

	// …and DESCRIBE of the whole grid carries them on the column.
	described := false
	for _, col := range describeGrid(t, w, "Shop.Ord").DataGridColumns {
		if explicitByKey(col.ExtraProps)["minWidth"].Value == "manual" {
			described = true
		}
	}
	if !described {
		t.Error("describe of the grid does not carry the column's minWidth")
	}
}

// The description has to parse, and carry each value under the key it was
// printed with — otherwise it is text that merely looks right.
func TestDataGridByKey_DescriptionReparses(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "configurationStorageType")["PrimitiveValue"] = "localStorage"
	gridValue(t, w, "columnsHidable")["PrimitiveValue"] = "false"
	gridValue(t, w, "exportDialogLabel")["TextTemplate"].(map[string]any)["Template"].(map[string]any)["Items"].([]any)[1].(map[string]any)["Text"] = "It's saving"

	raw := describeGrid(t, w, "")
	raw.DataSource = &rawDataSource{Type: "database", Reference: "Shop.Ord"}
	raw.Name = "dg"

	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(nil)
	ctx.Output = &buf
	outputWidgetMDLV3(ctx, raw, 1)
	described := buf.String()

	prog, errs := visitor.Build("create page Shop.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + described + "};")
	if len(errs) > 0 {
		t.Fatalf("description does not parse: %v\n%s", errs[0], described)
	}
	_ = prog
	for _, want := range []string{
		"configurationStorageType: 'localStorage'",
		"columnsHidable: false",
		"exportDialogLabel: 'It''s saving'",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("description lacks %q:\n%s", want, described)
		}
	}
	if errs := checkErrors(t, "create page Shop.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n"+described+"};"); len(errs) > 0 {
		t.Errorf("check refused the description:\n  %s\n%s", strings.Join(errs, "\n  "), described)
	}
}

// The empty-state placeholder is a widget slot the grid declares. It is
// printed, because showEmptyPlaceholder: custom is printed and means "look in
// the slot".
func TestDataGridByKey_EmptyPlaceholderSlotIsKept(t *testing.T) {
	w := dataGridFromTemplate(t)
	gridValue(t, w, "showEmptyPlaceholder")["PrimitiveValue"] = "custom"
	gridValue(t, w, "emptyPlaceholder")["Widgets"] = []any{2, map[string]any{
		"$Type": "Forms$DynamicText", "Name": "none_here",
		"Content": map[string]any{"Template": map[string]any{"Items": []any{2,
			map[string]any{"LanguageCode": "en_US", "Text": "Nothing here"}}}},
	}}

	got := describeGrid(t, w, "")
	if len(got.ChildSlots) != 1 || got.ChildSlots[0].Keyword != "emptyplaceholder" {
		t.Fatalf("ChildSlots = %+v, want the emptyplaceholder slot only (the control bar has its own block)", got.ChildSlots)
	}
}

// A Gallery takes its settings by key the same way, and had the same loss. Its
// column counts and click trigger already have MDL names and are not repeated.
func TestGalleryByKey_PrintsNonDefaultSettingsOnly(t *testing.T) {
	tmpl, err := widgets.GetTemplate("com.mendix.widget.web.gallery.Gallery")
	if err != nil || tmpl == nil {
		t.Fatalf("Gallery template: %v, %v", tmpl, err)
	}
	raw, _ := json.Marshal(map[string]any{"Type": tmpl.Type, "Object": tmpl.Object})
	var w map[string]any
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	normalizeTemplateJSON(w)
	w["$Type"] = "CustomWidgets$CustomWidget"
	w["Name"] = "gl"

	if got := describeGrid(t, w, ""); len(got.ExplicitProperties) != 0 || len(got.OmittedProps) != 0 {
		t.Errorf("untouched gallery printed %v / omitted %v", explicitKeyList(got.ExplicitProperties), got.OmittedProps)
	}

	gridValue(t, w, "pagination")["PrimitiveValue"] = "loadMore"
	gridValue(t, w, "pageSize")["PrimitiveValue"] = "5"
	gridValue(t, w, "desktopItems")["PrimitiveValue"] = "3"
	gridValue(t, w, "onClickTrigger")["PrimitiveValue"] = "double"

	got := describeGrid(t, w, "")
	props := explicitByKey(got.ExplicitProperties)
	for key, want := range map[string]string{"pagination": "loadMore", "pageSize": "5"} {
		if props[key].Value != want {
			t.Errorf("gallery %s = %q, want %q (printed %v)", key, props[key].Value, want, explicitKeyList(got.ExplicitProperties))
		}
	}
	for _, key := range []string{"desktopItems", "onClickTrigger"} {
		if _, dup := props[key]; dup {
			t.Errorf("gallery %s printed by key as well as under its MDL name", key)
		}
	}
	if got.DesktopColumns != "3" || got.OnClickTrigger != "double" {
		t.Errorf("MDL-named properties not read: DesktopColumns=%q OnClickTrigger=%q", got.DesktopColumns, got.OnClickTrigger)
	}
}
