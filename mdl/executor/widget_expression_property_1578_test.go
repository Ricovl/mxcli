// SPDX-License-Identifier: Apache-2.0

// A pluggable widget's own top-level expression properties
// (mendixlabs/mxcli#1578, the Marketplace Calendar).
//
// Reported: "`editable` cannot be written at all. The key is accepted with no
// warning, and nothing reaches the model", `showEventDate` drew MDL-WIDGET06
// "recognized but not yet persisted", "`describe page` prints neither
// property", and the build failed with
// `[CE0642] "Property 'Editable' is required."`.
//
// The generator gave a top-level `expression` property no mapping, so it was a
// known-but-unmapped key; and `editable:` is also the generic widget-level
// editability, which the Calendar's package does not declare — so the
// widget's own property of that name could never be reached.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

func calendarLikeMPK(systemProps ...string) *mpk.WidgetDefinition {
	def := &mpk.WidgetDefinition{
		ID: "com.mendix.widget.web.calendar.Calendar", Name: "Calendar", IsPluggable: true,
		Properties: []mpk.PropertyDef{
			{Key: "editable", Type: "expression", DefaultValue: "true"},
			{Key: "showEventDate", Type: "expression", DefaultValue: "true"},
			{Key: "titleExpression", Type: "expression"},
		},
	}
	for _, k := range systemProps {
		def.SystemProps = append(def.SystemProps, mpk.PropertyDef{Key: k})
	}
	return def
}

func mappingFor(def *WidgetDefinition, key string) (PropertyMapping, bool) {
	for _, m := range def.PropertyMappings {
		if m.PropertyKey == key {
			return m, true
		}
	}
	return PropertyMapping{}, false
}

func TestGenerateDefJSON_TopLevelExpressionIsMapped(t *testing.T) {
	def := GenerateDefJSON(calendarLikeMPK(), "CALENDAR")
	for _, key := range []string{"editable", "showEventDate", "titleExpression"} {
		m, ok := mappingFor(def, key)
		if !ok {
			t.Errorf("%s: no mapping — an unmapped key is dropped on write", key)
			continue
		}
		if m.Operation != "expression" {
			t.Errorf("%s: operation %q, want expression", key, m.Operation)
		}
	}
	for _, k := range def.KnownProperties {
		if strings.EqualFold(k, "editable") || strings.EqualFold(k, "showEventDate") {
			t.Errorf("%s is still listed as known-but-unmapped (MDL-WIDGET06 would warn it is dropped)", k)
		}
	}
}

// With no Editability system property, `editable:` can only mean the widget's
// own property; with one, it keeps its widget-level meaning.
func TestGenerateDefJSON_EditableRoutesToOwnPropertyWithoutEditability(t *testing.T) {
	own := GenerateDefJSON(calendarLikeMPK(), "CALENDAR")
	if !editableIsOwnProperty(own) {
		t.Error("a widget declaring `editable` and no Editability: `editable:` must reach its own property")
	}
	sys := GenerateDefJSON(calendarLikeMPK("Editability"), "CALENDAR")
	if editableIsOwnProperty(sys) {
		t.Error("a widget declaring Editability keeps `editable:` as the widget-level setting")
	}
}

func TestResolveMapping_OwnEditableExpression(t *testing.T) {
	def := GenerateDefJSON(calendarLikeMPK(), "CALENDAR")
	m, _ := mappingFor(def, "editable")
	engine := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	for _, tc := range []struct{ prop, want string }{
		{"editable: 'true'", "true"},
		{"editable: false", "false"},
		{"editable: '$currentObject/Reason != empty'", "$currentObject/Reason != empty"},
		{"editable: $currentObject/Reason != empty", "$currentObject/Reason != empty"},
	} {
		src := "create page Mod.P (Title: 'T') {\n  pluggablewidget 'com.mendix.widget.web.calendar.Calendar' cal (" + tc.prop + ")\n}"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", tc.prop, errs)
		}
		w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
		ctx, err := engine.resolveMapping(m, w)
		if err != nil {
			t.Fatalf("%s: %v", tc.prop, err)
		}
		if ctx.PrimitiveVal != tc.want {
			t.Errorf("%s: expression = %q, want %q", tc.prop, ctx.PrimitiveVal, tc.want)
		}
		// The widget-level editability must not be asked for as well: the
		// package declares none, and MDL-WIDGET41 would refuse the page.
		if widgetAsksSystemEditability(w, def) {
			t.Errorf("%s: also read as the widget-level editability", tc.prop)
		}
	}
}

// An unset expression mapping keeps the template's default instead of writing
// an empty expression over it — the CE0642 itself.
func TestApplyOperation_UnsetExpressionKeepsTemplateDefault(t *testing.T) {
	b := &recordingBuilder{}
	engine := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	if err := engine.applyOperation(b, "expression", "editable", &BuildContext{}); err != nil {
		t.Fatal(err)
	}
	if len(b.expressions) != 0 {
		t.Errorf("an unset expression was written: %v", b.expressions)
	}
}

func TestDescribe_PluggableExpressionProperty(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "cal",
		"Type": map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{"$ID": "pt1", "PropertyKey": "editable",
						"ValueType": map[string]any{"$ID": "vt1", "Type": "Expression"}},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{"TypePointer": "pt1", "Value": map[string]any{
					"Expression": "$currentObject/Reason != empty", "PrimitiveValue": "",
				}},
			},
		},
	}
	got := extractExplicitProperties(ctx, w)
	var buf bytes.Buffer
	out := appendExplicitProps(&ExecContext{Output: &buf}, nil, got)
	if len(out) != 1 || out[0] != "editable: '$currentObject/Reason != empty'" {
		t.Errorf("describe = %v, want [editable: '$currentObject/Reason != empty']", out)
	}
}

// recordingBuilder records SetExpression calls; nothing else is reached.
type recordingBuilder struct {
	backend.WidgetObjectBuilder
	expressions map[string]string
}

func (b *recordingBuilder) SetExpression(key, expr string) {
	if b.expressions == nil {
		b.expressions = map[string]string{}
	}
	b.expressions[key] = expr
}
