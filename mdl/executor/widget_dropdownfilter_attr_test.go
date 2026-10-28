// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1519, item 2:
//
//	dropdownfilter ddf (attributes: [Status])
//
// passed check, exec and mx check, and stored attrChoice="auto" with an EMPTY
// `attr` — the attribute was gone. Describe printed a bare `dropdownfilter`, and
// in a gallery (no column to inherit an attribute from) the filter failed at
// runtime with "Unable to get filter store".
//
// The definition mapped `Attributes` onto `attributes`/`defaultFilter`, the text
// filter's keys. The DataWidgets drop-down filter (3.x; 3.4.0 in 11.15, the 11.6
// template here) has neither: it filters on ONE attribute, `attr`, which its
// editorConfig shows only when attrChoice is "linked" (Custom). And the builder
// dropped the value for the undeclared key without a word.

// dropdownFilterEngine builds inside a context over Tmp.Item (Name, Status) with
// the REAL drop-down filter template.
func dropdownFilterEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-tmp")}, Name: "Tmp"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-tmp")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-item")}, Name: "Item",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}, {Name: "Status"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    "Tmp.Item",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func realDropdownFilterDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get("DROPDOWNFILTER")
	if !ok {
		t.Fatal("no DROPDOWNFILTER definition")
	}
	return def
}

// customWidgetValue returns the WidgetValue stored for a property key.
func customWidgetValue(t *testing.T, cw *pages.CustomWidget, key string) bson.D {
	t.Helper()
	entry, ok := cw.PropertyTypeIDMap[key]
	if !ok {
		t.Fatalf("template declares no property %q", key)
	}
	want := strings.ReplaceAll(entry.PropertyTypeID, "-", "")
	for _, e := range cw.RawObject {
		if e.Key != "Properties" {
			continue
		}
		for _, item := range e.Value.(bson.A) {
			prop, ok := item.(bson.D)
			if !ok {
				continue
			}
			var ptr string
			var val bson.D
			for _, f := range prop {
				switch f.Key {
				case "TypePointer":
					switch b := f.Value.(type) {
					case bson.Binary:
						ptr = types.BlobToUUID(b.Data)
					case []byte:
						ptr = types.BlobToUUID(b)
					}
				case "Value":
					val, _ = f.Value.(bson.D)
				}
			}
			if strings.ReplaceAll(ptr, "-", "") == want {
				return val
			}
		}
	}
	t.Fatalf("property %q not found in the widget object", key)
	return nil
}

func storedAttribute(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	ref, _ := bsonField(customWidgetValue(t, cw, key), "AttributeRef").(bson.D)
	s, _ := bsonField(ref, "Attribute").(string)
	return s
}

func storedPrimitive(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	s, _ := bsonField(customWidgetValue(t, cw, key), "PrimitiveValue").(string)
	return s
}

func buildDropdownFilter(t *testing.T, src string) (*pages.CustomWidget, error) {
	t.Helper()
	w := parseSnippetWidget(t, src)
	widget, err := dropdownFilterEngine(t).Build(realDropdownFilterDef(t), w)
	if err != nil {
		return nil, err
	}
	return widget, nil
}

// The reported case: the attribute lands on `attr`, and attrChoice selects it.
func TestDropdownFilter1519_AttributeIsStoredOnAttr(t *testing.T) {
	cw, err := buildDropdownFilter(t, `dropdownfilter ddf (attributes: [Status])`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := storedAttribute(t, cw, "attr"); got != "Tmp.Item.Status" {
		t.Errorf("attr = %q, want Tmp.Item.Status — the filter has lost its attribute", got)
	}
	if got := storedPrimitive(t, cw, "attrChoice"); got != "linked" {
		t.Errorf("attrChoice = %q, want linked — under \"auto\" the editorConfig hides `attr`", got)
	}
}

// CONTROL: a bare filter in a column inherits the column's attribute, which is
// what attrChoice "auto" means. It must stay that way.
func TestDropdownFilter1519_BareFilterStaysAuto(t *testing.T) {
	cw, err := buildDropdownFilter(t, `dropdownfilter ddf`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := storedPrimitive(t, cw, "attrChoice"); got != "auto" {
		t.Errorf("attrChoice = %q, want auto for a filter given no attribute", got)
	}
	if got := storedAttribute(t, cw, "attr"); got != "" {
		t.Errorf("attr = %q, want empty for a filter given no attribute", got)
	}
}

// The widget has ONE attribute slot. Two attributes cannot be stored, and
// keeping the first while discarding the second is the same silent drop.
func TestDropdownFilter1519_SeveralAttributesAreRefused(t *testing.T) {
	_, err := buildDropdownFilter(t, `dropdownfilter ddf (attributes: [Status, Name])`)
	if err == nil {
		t.Fatal("two attributes on a drop-down filter were accepted; only one can be stored")
	}
	if !strings.Contains(err.Error(), "one attribute") {
		t.Errorf("refusal does not say the widget takes one attribute: %v", err)
	}
}

// Check refuses what exec refuses, and reports nothing for the single-attribute
// form — in particular no MDL-WIDGET10 "attr is hidden when attrChoice is auto":
// the constants of a mode the widget is not built in are not its configuration.
func TestCheck1519_DropdownFilterAttributes(t *testing.T) {
	rules := func(widget string) map[string]int {
		got := map[string]int{}
		src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { gallery g (DataSource: database from M.E) { filter f { ` +
			widget + ` } template t { dynamictext x (Content: 'x') } } };`
		for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
			got[v.RuleID]++
		}
		return got
	}
	if got := rules(`dropdownfilter ddf (attributes: [Status, Name])`); got["MDL-WIDGET47"] != 1 {
		t.Errorf("two attributes: MDL-WIDGET47 reported %d times, want 1 (all: %v)", got["MDL-WIDGET47"], got)
	}
	got := rules(`dropdownfilter ddf (attributes: [Status])`)
	if got["MDL-WIDGET47"] != 0 || got["MDL-WIDGET10"] != 0 {
		t.Errorf("one attribute: unexpected diagnostics %v", got)
	}
}

// The builder half: a definition that routes an AUTHORED value to a key the
// widget does not declare must fail the statement, not drop the value. This is
// the shape the old dropdownfilter definition had.
func TestPluggableEngine1519_AuthoredValueForUndeclaredKeyIsRefused(t *testing.T) {
	def := *realDropdownFilterDef(t)
	def.Modes = nil
	def.PropertyMappings = []PropertyMapping{
		{PropertyKey: "attributes", Source: "Attributes", Operation: "attributeObjects"},
	}
	w := parseSnippetWidget(t, `dropdownfilter ddf (attributes: [Status])`)
	_, err := dropdownFilterEngine(t).Build(&def, w)
	if err == nil {
		t.Fatal("a value for the undeclared key `attributes` was dropped without an error")
	}
	if !strings.Contains(err.Error(), "attributes") {
		t.Errorf("refusal does not name the undeclared key: %v", err)
	}
}

// dropdownFilterRaw is a stored drop-down filter as describe reads it: attrChoice
// and attr resolved through the widget's own PropertyTypes.
func dropdownFilterRaw(attrChoice, attr string) map[string]any {
	prop := func(id string, v map[string]any) map[string]any {
		return map[string]any{"TypePointer": id, "Value": v}
	}
	return map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "ddf",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": "pt-choice", "PropertyKey": "attrChoice",
					"ValueType": map[string]any{"Type": "Enumeration"}},
				map[string]any{"$ID": "pt-attr", "PropertyKey": "attr",
					"ValueType": map[string]any{"Type": "Attribute"}},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			prop("pt-choice", map[string]any{"PrimitiveValue": attrChoice}),
			prop("pt-attr", map[string]any{"AttributeRef": map[string]any{"Attribute": attr}}),
		}},
	}
}

// Describe prints the custom attribute back in the form exec reads.
func TestDescribe1519_DropdownFilterPrintsItsAttribute(t *testing.T) {
	got := parseRawWidget(nil, dropdownFilterRaw("linked", "Tmp.Item.Status"))
	if len(got) != 1 || len(got[0].FilterAttributes) != 1 || got[0].FilterAttributes[0] != "Status" {
		t.Fatalf("described %d widget(s), filter attributes %v; want one with [Status] — describe printed a bare dropdownfilter",
			len(got), filterAttrsOf(got))
	}
	// CONTROL: under "auto" the column supplies the attribute; nothing to print.
	got = parseRawWidget(nil, dropdownFilterRaw("auto", ""))
	if len(got) != 1 || len(got[0].FilterAttributes) != 0 {
		t.Errorf("auto filter described with attributes %v, want none", filterAttrsOf(got))
	}
}

func filterAttrsOf(ws []rawWidget) [][]string {
	out := make([][]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.FilterAttributes)
	}
	return out
}

// The configuration check reads is the mode the widget BUILDS in. Reading every
// mode's constants let the default mode's attrChoice "auto" and the association
// mode's baseType "ref" stand for a filter given `attributes:`, and MDL-WIDGET10
// then called its `attr` hidden twice over (seen with a project's lifted rules).
func TestWidgetValueMap1519_ReadsOnlyTheActiveMode(t *testing.T) {
	w := parseSnippetWidget(t, `dropdownfilter ddf (attributes: [Status])`)
	values, _ := widgetValueMap(w, realDropdownFilterDef(t))
	if got := values["attrchoice"]; got != "linked" {
		t.Errorf("attrChoice = %q, want linked (the attribute mode's constant)", got)
	}
	if got, ok := values["basetype"]; ok {
		t.Errorf("baseType = %q, but only the association mode writes it", got)
	}
}

// The same drop of #1519 item 2 on the ALTER path. A column built by ALTER PAGE
// INSERT or REPLACE (buildColumnSpecFromAST) built its filter from the widget id
// and name alone, never reading the filter's own properties:
//
//	alter page … { insert after dg column(Name) { column (…) { dropdownfilter ddf (attributes: [Status]) } } }
//
// stored attrChoice "auto" and an empty `attr`, check passed, and describe
// printed a bare `dropdownfilter ddf` — the CREATE path's symptom before #1422.
func TestAlterColumn1519_FilterKeepsItsProperties(t *testing.T) {
	e := dropdownFilterEngine(t)
	real := &modelsdkbackend.Backend{}
	b := e.pageBuilder.backend.(*mock.MockBackend)
	b.BuildFilterWidgetFunc = real.BuildFilterWidget
	pb := e.pageBuilder
	pb.widgetBackend = b
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	pb.widgetRegistry = reg

	grid := parseSnippetWidget(t, `datagrid dg (DataSource: database from Tmp.Item) {
    column (Attribute: Name, Caption: 'Name') { dropdownfilter ddf (attributes: [Status]) }
  }`)
	spec, err := pb.buildColumnSpecFromAST(grid.Children[0])
	if err != nil {
		t.Fatalf("buildColumnSpecFromAST: %v", err)
	}
	cw, ok := spec.FilterWidget.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("column filter = %T, want a drop-down filter", spec.FilterWidget)
	}
	if got, _ := bsonField(rawWidgetValue(t, cw, "attrChoice"), "PrimitiveValue").(string); got != "linked" {
		t.Errorf("attrChoice = %q, want linked", got)
	}
	ref, _ := bsonField(rawWidgetValue(t, cw, "attr"), "AttributeRef").(bson.D)
	if got, _ := bsonField(ref, "Attribute").(string); got != "Tmp.Item.Status" {
		t.Errorf("attr = %q, want Tmp.Item.Status", got)
	}
	if cw.Name != "ddf" {
		t.Errorf("filter name = %q, want ddf", cw.Name)
	}

	// The control: a bare filter keeps the column's attribute (auto).
	bare := parseSnippetWidget(t, `datagrid dg (DataSource: database from Tmp.Item) {
    column (Attribute: Name, Caption: 'Name') { dropdownfilter ddf2 }
  }`)
	spec, err = pb.buildColumnSpecFromAST(bare.Children[0])
	if err != nil {
		t.Fatalf("buildColumnSpecFromAST: %v", err)
	}
	cw, ok = spec.FilterWidget.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("column filter = %T, want a drop-down filter", spec.FilterWidget)
	}
	if got, _ := bsonField(rawWidgetValue(t, cw, "attrChoice"), "PrimitiveValue").(string); got != "auto" {
		t.Errorf("a bare drop-down filter: attrChoice = %q, want auto", got)
	}
}

// rawWidgetValue is customWidgetValue for a widget whose PropertyTypeIDMap is
// not filled in: the property type is found by key in the stored Type.
func rawWidgetValue(t *testing.T, cw *pages.CustomWidget, key string) bson.D {
	t.Helper()
	idOf := func(v any) string {
		switch b := v.(type) {
		case bson.Binary:
			return strings.ReplaceAll(types.BlobToUUID(b.Data), "-", "")
		case []byte:
			return strings.ReplaceAll(types.BlobToUUID(b), "-", "")
		case string:
			return strings.ReplaceAll(b, "-", "")
		}
		return ""
	}
	objType, _ := bsonField(cw.RawType, "ObjectType").(bson.D)
	propTypes, _ := bsonField(objType, "PropertyTypes").(bson.A)
	var typeID string
	for _, pt := range propTypes {
		if d, ok := pt.(bson.D); ok && bsonField(d, "PropertyKey") == key {
			typeID = idOf(bsonField(d, "$ID"))
		}
	}
	if typeID == "" {
		t.Fatalf("stored Type declares no property %q", key)
	}
	props, _ := bsonField(cw.RawObject, "Properties").(bson.A)
	for _, p := range props {
		if d, ok := p.(bson.D); ok && idOf(bsonField(d, "TypePointer")) == typeID {
			v, _ := bsonField(d, "Value").(bson.D)
			return v
		}
	}
	t.Fatalf("property %q not found in the widget object", key)
	return nil
}
