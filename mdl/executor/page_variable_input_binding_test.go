// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1235: an input widget bound to a page variable. Studio Pro
// stores it with no AttributeRef and a SourceVariable naming the variable —
// measured on TestApp (Studio Pro 11, Pages.Probe1235_pagevar, read back over
// MCP): `sourceVariable: {Pages$PageVariable, localVariable: "ShowAll"}` and no
// attributeRef, with ped_check_errors reporting no errors. MDL refused to write
// it (MDL-WIDGET34) and describe printed the widget unbound.

func pageVarBuilder(vars ...string) *pageBuilder {
	pb := &pageBuilder{
		widgetScope:      map[string]model.ID{},
		paramScope:       map[string]model.ID{},
		paramEntityNames: map[string]string{},
		localVariables:   map[string]bool{},
	}
	pb.argCtx = atDocumentRoot()
	for _, v := range vars {
		pb.localVariables[v] = true
	}
	return pb
}

func pageVarRef(name string) *ast.DataSourceV3 {
	return &ast.DataSourceV3{Type: "parameter", Reference: "$" + name}
}

func TestPageVariableBinding_Builders(t *testing.T) {
	want := &pages.WidgetVariable{Variable: "ShowAll", Kind: "local"}
	check := func(t *testing.T, path string, sv *pages.WidgetVariable) {
		t.Helper()
		if path != "" {
			t.Errorf("AttributePath = %q, want none — a page variable is not an attribute", path)
		}
		if sv == nil || *sv != *want {
			t.Errorf("SourceVariable = %+v, want %+v", sv, want)
		}
	}
	for _, typ := range []string{"checkbox", "textbox", "textarea", "datepicker", "dropdown", "radiobuttons"} {
		t.Run(typ, func(t *testing.T) {
			pb := pageVarBuilder("ShowAll")
			w := &ast.WidgetV3{Name: "in1", Type: typ, Properties: map[string]any{"Attribute": pageVarRef("ShowAll")}}
			switch typ {
			case "checkbox":
				x, err := pb.buildCheckBoxV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			case "textbox":
				x, err := pb.buildTextBoxV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			case "textarea":
				x, err := pb.buildTextAreaV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			case "datepicker":
				x, err := pb.buildDatePickerV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			case "dropdown":
				x, err := pb.buildDropdownV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			case "radiobuttons":
				x, err := pb.buildRadioButtonsV3(w)
				if err != nil {
					t.Fatal(err)
				}
				check(t, x.AttributePath, x.SourceVariable)
			}
		})
	}
}

// A `$name` that is not a page variable is still refused — writing it would
// store an input with no binding at all.
func TestPageVariableBinding_UndeclaredIsRefused(t *testing.T) {
	pb := pageVarBuilder("ShowAll")
	w := &ast.WidgetV3{Name: "cb", Type: "checkbox", Properties: map[string]any{"Attribute": pageVarRef("Other")}}
	if _, err := pb.buildCheckBoxV3(w); err == nil {
		t.Fatal("checkbox bound to an undeclared $Other was built; want a refusal")
	}
}

// check-time: the page's own Variables decide, with no project needed.
func TestPageVariableBinding_Check(t *testing.T) {
	page := func(attr string) *ast.CreatePageStmtV3 {
		return &ast.CreatePageStmtV3{
			Name:      ast.QualifiedName{Module: "M", Name: "P"},
			Variables: []ast.PageVariable{{Name: "ShowAll", DataType: "Boolean", DefaultValue: "true"}},
			Widgets: []*ast.WidgetV3{{Name: "cb", Type: "checkbox", Properties: map[string]any{
				"Label": "Show all", "Attribute": pageVarRef(attr)}}},
		}
	}
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range ValidateWidgetPropertiesForStatement(page("ShowAll"), reg) {
		if v.RuleID == "MDL-WIDGET34" {
			t.Errorf("a declared page variable was refused: %s", v.Message)
		}
	}
	found := false
	for _, v := range ValidateWidgetPropertiesForStatement(page("Other"), reg) {
		if v.RuleID == "MDL-WIDGET34" {
			found = true
		}
	}
	if !found {
		t.Error("an undeclared $Other passed check; exec refuses it")
	}
}

// describe reads the binding back in the spelling the builder takes.
func TestPageVariableBinding_Describe(t *testing.T) {
	w := map[string]any{
		"$Type":          "Forms$CheckBox",
		"Name":           "cbShowAll",
		"AttributeRef":   nil,
		"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "LocalVariable": "ShowAll", "PageParameter": "", "Widget": ""},
	}
	if got := extractInputAttribute(nil, w); got != "$ShowAll" {
		t.Errorf("describe binding = %q, want $ShowAll", got)
	}
	// Control: an input bound to its data context still reads as before.
	bound := map[string]any{"AttributeRef": map[string]any{"Attribute": "M.E.Name"}, "SourceVariable": nil}
	if got := extractInputAttribute(nil, bound); strings.HasPrefix(got, "$") {
		t.Errorf("a data-context binding described as %q", got)
	}
}
