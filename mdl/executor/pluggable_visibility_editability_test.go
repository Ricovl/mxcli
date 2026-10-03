// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Visibility and editability on a PLUGGABLE widget.
//
// A combo box declares both as system properties (<systemProperty
// key="Visibility"/> and key="Editability" in Combobox.xml), and Studio Pro
// stores them on the CustomWidgets$CustomWidget itself — ConditionalVisibility-
// Settings, Editable, ConditionalEditabilitySettings — exactly as on a text box;
// the widget's Object holds no WidgetProperty for a system property. mxcli
// parsed all four MDL forms and stored none of them: `visible: false` and
// `editable: Never` were dropped in silence, and the expression forms were
// refused as MDL-WIDGET01 "no property VisibleIf/EditableIf".

func comboWithProps(props map[string]any) *ast.WidgetV3 {
	all := map[string]any{"Attribute": "Reference", "Label": "Ref"}
	for k, v := range props {
		all[k] = v
	}
	return &ast.WidgetV3{Type: "combobox", Name: "cmb", Properties: all}
}

func comboDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	def, ok := reg.Get("COMBOBOX")
	if !ok {
		t.Fatal("no COMBOBOX definition")
	}
	return def
}

func TestBuildPluggable_WritesVisibilityAndEditability(t *testing.T) {
	const expr = "$currentObject/Reference != empty"
	cases := []struct {
		name         string
		props        map[string]any
		wantVisible  string // ConditionalVisibility expression, "" = none
		wantEditable string
		wantEditIf   string // ConditionalEditability expression, "" = none
	}{
		// The control: nothing authored, nothing stored.
		{"plain", nil, "", "Always", ""},
		{"visible false", map[string]any{"Visible": false}, "false", "Always", ""},
		{"visible expression", map[string]any{"VisibleIf": expr}, expr, "Always", ""},
		{"editable never", map[string]any{"Editable": "Never"}, "", "Never", ""},
		{"editable expression", map[string]any{"EditableIf": expr}, "", "Conditional", expr},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pb := scopeEngine(t).pageBuilder
			w, err := pb.buildPluggable(comboDef(t), comboWithProps(c.props))
			if err != nil {
				t.Fatalf("buildPluggable: %v", err)
			}
			cw, ok := w.(*pages.CustomWidget)
			if !ok {
				t.Fatalf("built %T, want *pages.CustomWidget", w)
			}
			gotVis := ""
			if cw.ConditionalVisibility != nil {
				gotVis = cw.ConditionalVisibility.Expression
			}
			if gotVis != c.wantVisible {
				t.Errorf("ConditionalVisibility expression = %q, want %q", gotVis, c.wantVisible)
			}
			// The writer stores CustomWidget.Editable, so that is the field
			// that has to carry the author's value.
			if cw.Editable != c.wantEditable {
				t.Errorf("Editable = %q, want %q", cw.Editable, c.wantEditable)
			}
			gotEd := ""
			if cw.ConditionalEditability != nil {
				gotEd = cw.ConditionalEditability.Expression
			}
			if gotEd != c.wantEditIf {
				t.Errorf("ConditionalEditability expression = %q, want %q", gotEd, c.wantEditIf)
			}
		})
	}
}

// typeDeclaring is a minimal CustomWidgetType whose PropertyTypes are the given
// system properties plus one regular property.
func typeDeclaring(system ...string) bson.D {
	pts := bson.A{int32(2), bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
		{Key: "PropertyKey", Value: "attributeEnumeration"},
		{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "Attribute"}}},
	}}
	for _, k := range system {
		pts = append(pts, bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
			{Key: "PropertyKey", Value: k},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "System"}}},
		})
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
		{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: pts}}},
	}
}

// A package that does not declare Editability has no place for the setting:
// Studio Pro shows no Editability section for it, and writing one would store
// a setting the widget never evaluates, so it is refused. Visibility is offered
// on every pluggable widget, declared or not.
func TestApplyPluggableSystemSettings_RefusesUndeclared(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		props    map[string]any
		wantErr  string
	}{
		{"visibility declared", []string{"Visibility"}, map[string]any{"Visible": false}, ""},
		// Visibility needs no declaration: Studio Pro stores it on a Datagrid
		// that declares none (TestApp WorkflowCommons.UserTask_Assign), and
		// mxbuild 11.14 accepts static and conditional visibility there.
		{"visibility undeclared", []string{"Editability"}, map[string]any{"Visible": false}, ""},
		{"visibility expr undeclared", nil, map[string]any{"VisibleIf": "true"}, ""},
		{"editability declared", []string{"Editability"}, map[string]any{"Editable": "Never"}, ""},
		{"editability undeclared", []string{"Visibility"}, map[string]any{"Editable": "Never"}, "Editability"},
		{"editability expr undeclared", []string{"Visibility"}, map[string]any{"EditableIf": "true"}, "Editability"},
		// Mendix's defaults ask for nothing to be stored, so they need nothing declared.
		{"visible true needs nothing", nil, map[string]any{"Visible": true}, ""},
		{"editable always needs nothing", nil, map[string]any{"Editable": "Always"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cw := &pages.CustomWidget{RawType: typeDeclaring(c.declared...), Editable: "Always"}
			err := applyPluggableSystemSettings(cw, &ast.WidgetV3{Type: "switch", Name: "w", Properties: c.props})
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a refusal naming %s, got none (ConditionalVisibility=%v Editable=%q)",
					c.wantErr, cw.ConditionalVisibility, cw.Editable)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error does not name %s: %v", c.wantErr, err)
			}
		})
	}
}

func TestValidatePluggableVisibilityEditability(t *testing.T) {
	// One page per case. Declared system properties, from the embedded
	// templates (which mirror each package's widget XML): the combo box has
	// Visibility and Editability, the image Visibility only, the drop-down
	// sort neither.
	cases := []struct {
		name     string
		widget   string
		wantRule string // "" = no visibility/editability diagnostic at all
		wantProp string
	}{
		{"combobox visible false", "combobox c (Attribute: Status, Visible: false)", "", ""},
		{"combobox visible expr", "combobox c (Attribute: Status, Visible: $currentObject/Title != empty)", "", ""},
		{"combobox editable never", "combobox c (Attribute: Status, Editable: Never)", "", ""},
		{"combobox editable expr", "combobox c (Attribute: Status, Editable: $currentObject/Title != empty)", "", ""},
		{"image visible false", "image i (Visible: false)", "", ""},
		{"image editable never", "image i (Editable: Never)", pluggableSystemPropRule, "Editability"},
		{"dropdownsort visible false", "dropdownsort s (Visible: false)", "", ""},
		{"dropdownsort visible expr", "dropdownsort s (Visible: $currentObject/Title != empty)", "", ""},
		{"dropdownsort editable never", "dropdownsort s (Editable: Never)", pluggableSystemPropRule, "Editability"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "mdl 1;\ncreate page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($E: M.E)) {\n" +
				"  dataview dv (DataSource: $E) {\n    " + c.widget + "\n  }\n};"
			prog := parseMDL(t, src)
			var got []string
			for _, v := range ValidateWidgetProperties(prog, "") {
				switch v.RuleID {
				case "MDL-WIDGET01", "MDL-WIDGET21", pluggableSystemPropRule:
					got = append(got, v.RuleID+": "+v.Message)
				}
			}
			if c.wantRule == "" {
				if len(got) > 0 {
					t.Fatalf("unexpected diagnostics: %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.HasPrefix(got[0], c.wantRule+":") {
				t.Fatalf("want one %s, got %v", c.wantRule, got)
			}
			if !strings.Contains(got[0], c.wantProp) {
				t.Errorf("message should name the undeclared system property %s: %s", c.wantProp, got[0])
			}
		})
	}
}

// describe → exec: what the builder stores, describe prints in a form the
// builder reads back.
func TestDescribePluggable_EmitsVisibilityAndEditability(t *testing.T) {
	const expr = "$currentObject/Reference != empty"
	// Stored shapes, as parseRawWidget sees a CustomWidget.
	raw := func(extra map[string]any) map[string]any {
		w := map[string]any{
			"$Type":    "CustomWidgets$CustomWidget",
			"Name":     "cmb",
			"Editable": "Always",
		}
		for k, v := range extra {
			w[k] = v
		}
		return w
	}
	cases := []struct {
		name string
		w    map[string]any
		want []string
	}{
		{"never", raw(map[string]any{"Editable": "Never"}), []string{"Editable: Never"}},
		{"conditional editable", raw(map[string]any{
			"Editable":                       "Conditional",
			"ConditionalEditabilitySettings": map[string]any{"Expression": expr},
		}), []string{"Editable: " + expr}},
		{"visible false", raw(map[string]any{
			"ConditionalVisibilitySettings": map[string]any{"Expression": "false"},
			// Printed bracketed, as for every other widget: a bare `false` parses
			// as a literal, not as the expression BareWidgetCondition looks for.
			// Either way it reads back as the expression "false".
		}), []string{"Visible: [false]"}},
		{"plain", raw(nil), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ws := parseRawWidget(nil, c.w)
			if len(ws) != 1 {
				t.Fatalf("parsed %d widgets", len(ws))
			}
			props := appendAppearanceProps(nil, nil, ws[0])
			for _, want := range c.want {
				found := false
				for _, p := range props {
					if p == want {
						found = true
					}
				}
				if !found {
					t.Errorf("describe props %v lack %q", props, want)
				}
			}
			for _, p := range props {
				if p == "Editable: Conditional" {
					t.Errorf("`Editable: Conditional` is not authorable — the expression is what makes it conditional: %v", props)
				}
				if strings.HasPrefix(p, "Editable") || strings.HasPrefix(p, "Visible") {
					if len(c.want) == 0 {
						t.Errorf("plain widget described with %q", p)
					}
				}
			}
		})
	}
}
