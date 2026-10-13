// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The embedded pluggable-widget templates are extracted from a Mendix 11.6
// project, so their CustomWidgets$WidgetValueType carries six keys Mendix
// introduced after 9.24 (mendixmodelsdk WidgetValueType.versionInfo):
//
//	SetLabel 10.5 · IsLinked, IsMetaData 10.14 · DefaultType 10.15 ·
//	ActionVariables 10.21 · AllowUpload 11.8
//
// DefaultType holds "None" on every property, and IsLinked / IsMetaData /
// SetLabel are true on the combo box and the data grid filters, so the property
// floor (canon.StripUndeclaredProperties) refused every page with a pluggable
// widget on a 9.24 project:
//
//	this Mendix 9.24.30 project cannot store
//	CustomWidgets$WidgetValueType.DefaultType (Mendix 10.15.0+)
//
// Found running the doctype suite on Mendix 9.24.30 (no issue filed).

// studioPro924ValueTypeKeys is the CustomWidgets$WidgetValueType key set
// Studio Pro 9.24.30 stores: Atlas_Core.LanguageSelectorWidget in a Studio
// Pro-authored 9.24.30 app (`mxcli bson dump --type snippet`).
var studioPro924ValueTypeKeys = []string{
	"$ID", "$Type",
	"AllowNonPersistableEntities", "AllowedTypes", "AssociationTypes",
	"DataSourceProperty", "DefaultValue", "EntityProperty", "EnumerationValues",
	"IsList", "IsPath", "Multiline", "ObjectType", "OnChangeProperty",
	"ParameterIsList", "PathType", "Required", "ReturnType",
	"SelectableObjectsProperty", "SelectionTypes", "Translations", "Type",
}

// encodeTemplateWidget builds a widget's Type from its template the way
// LoadWidgetTemplate / BuildFilterWidget do for a project of version pv, and
// encodes the widget. The Object is left out: the template's is a sample
// configuration the builder rewrites from the statement (a data grid's sample
// columns are replaced by the declared ones), so it is not what gets written.
func encodeTemplateWidget(t *testing.T, widgetID string, pv *types.ProjectVersion) []byte {
	t.Helper()
	typ, _, _, _, err := templateFullBSONIDs(widgetID, "", pv)
	if err != nil {
		t.Fatalf("%s: template: %v", widgetID, err)
	}
	if typ == nil {
		t.Fatalf("%s: no template", widgetID)
	}
	cw := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "CustomWidgets$CustomWidget"},
			Name:        "w",
		},
		RawType: v2ToV1BSON(typ),
	}
	g, err := customWidgetToGen(cw)
	if err != nil {
		t.Fatalf("%s: customWidgetToGen: %v", widgetID, err)
	}
	b, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("%s: encode: %v", widgetID, err)
	}
	return b
}

// valueTypeKeySets returns the key set of every CustomWidgets$WidgetValueType
// in an encoded document.
func valueTypeKeySets(t *testing.T, b []byte) [][]string {
	t.Helper()
	var doc bsonv2.D
	if err := bsonv2.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var out [][]string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bsonv2.D:
			var keys []string
			isVT := false
			for _, e := range x {
				keys = append(keys, e.Key)
				if e.Key == "$Type" && e.Value == "CustomWidgets$WidgetValueType" {
					isVT = true
				}
				walk(e.Value)
			}
			if isVT {
				sort.Strings(keys)
				out = append(out, keys)
			}
		case bsonv2.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func TestTemplateWidgetMatchesMendix924ValueTypeKeys(t *testing.T) {
	pv := v(9, 24, 30)
	floor := version.Version{Major: 9, Minor: 24, Patch: 30}
	want := append([]string(nil), studioPro924ValueTypeKeys...)
	sort.Strings(want)

	for _, id := range mwidgets.ListAvailableTemplates() {
		t.Run(id, func(t *testing.T) {
			b := encodeTemplateWidget(t, id, pv)
			// The symptom: the storage write refuses the unit.
			if _, err := canon.StripUndeclaredProperties(b, &floor); err != nil {
				t.Fatalf("Mendix 9.24.30 write refused: %v", err)
			}
			sets := valueTypeKeySets(t, b)
			if len(sets) == 0 {
				t.Fatal("no WidgetValueType in the encoded widget")
			}
			for _, got := range sets {
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Fatalf("WidgetValueType keys on 9.24.30:\n got  %v\n want %v (Studio Pro 9.24.30)", got, want)
				}
			}
		})
	}
}

// A project that declares the keys keeps the template verbatim — DefaultType
// on every value type, and the same key sets an unreadable (newest) version
// writes.
func TestTemplateWidgetKeepsValueTypeKeysFrom11_8(t *testing.T) {
	for _, id := range []string{
		"com.mendix.widget.web.combobox.Combobox",
		"com.mendix.widget.web.datagrid.Datagrid",
		"com.mendix.widget.web.datagridtextfilter.DatagridTextFilter",
	} {
		t.Run(id, func(t *testing.T) {
			newest := valueTypeKeySets(t, encodeTemplateWidget(t, id, nil))
			for _, pv := range []*types.ProjectVersion{v(11, 8, 0), v(11, 13, 0)} {
				got := valueTypeKeySets(t, encodeTemplateWidget(t, id, pv))
				if len(got) != len(newest) {
					t.Fatalf("%d.%d: %d value types, want %d", pv.MajorVersion, pv.MinorVersion, len(got), len(newest))
				}
				for i := range got {
					if strings.Join(got[i], ",") != strings.Join(newest[i], ",") {
						t.Fatalf("%d.%d: value type %d keys changed:\n got  %v\n want %v", pv.MajorVersion, pv.MinorVersion, i, got[i], newest[i])
					}
					if !containsKey(got[i], "DefaultType") {
						t.Fatalf("%d.%d: value type %d lost DefaultType: %v", pv.MajorVersion, pv.MinorVersion, i, got[i])
					}
				}
			}
		})
	}
}

// Between the floors, only the keys the version lacks go: 10.15 has DefaultType
// but not ActionVariables (10.21) or AllowUpload (11.8).
func TestTemplateWidgetValueTypeKeysAt10_15(t *testing.T) {
	sets := valueTypeKeySets(t, encodeTemplateWidget(t, "com.mendix.widget.web.combobox.Combobox", v(10, 15, 0)))
	for _, s := range sets {
		if !containsKey(s, "DefaultType") || !containsKey(s, "IsLinked") || !containsKey(s, "SetLabel") {
			t.Fatalf("10.15.0 lost a key it declares: %v", s)
		}
		if containsKey(s, "ActionVariables") || containsKey(s, "AllowUpload") {
			t.Fatalf("10.15.0 kept a key it does not declare: %v", s)
		}
	}
}

func containsKey(s []string, k string) bool {
	for _, x := range s {
		if x == k {
			return true
		}
	}
	return false
}
