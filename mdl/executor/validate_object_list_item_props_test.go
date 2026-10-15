// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// An object-list item (an Accordion GROUP, a DataGrid COLUMN, a chart SERIES)
// is built by walking the widget's schema and looking each key up in the MDL
// property bag, so a key that matches nothing is never visited: `GROUP g1
// (Caption: 'Group one')` passed check, exec wrote the group with the shipped
// default `headerText: 'Header'`, and the caption was gone (#891 follow-up).

func TestValidateObjectListItemUnknownProps(t *testing.T) {
	mapping := &ObjectListMapping{
		MDLContainer: "COLUMN",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "header", Operation: "texttemplate", MdlAliases: []string{"Caption"}},
			{PropertyKey: "attribute", Operation: "attribute"},
			{PropertyKey: "visible", Operation: "expression", MdlAliases: []string{"VisibleIf"}},
		},
		ItemSlots: []ItemSlotMapping{{PropertyKey: "content", Operation: "widgets"}},
	}
	item := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "column", Name: "c1", Properties: props}
	}
	cases := []struct {
		name    string
		props   map[string]any
		wantBad string // the key the violation must name; "" = no violation
	}{
		{"schema key", map[string]any{"header": "x"}, ""},
		{"schema key, other casing", map[string]any{"Attribute": "Name"}, ""},
		{"alias", map[string]any{"Caption": "x"}, ""},
		{"lowered visitor key is an alias", map[string]any{"VisibleIf": "true"}, ""},
		{"params companion of the schema key", map[string]any{"headerParams": []ast.ParamAssignmentV3{}}, ""},
		{"params companion of an alias", map[string]any{"CaptionParams": []ast.ParamAssignmentV3{}}, ""},
		{"a key the schema does not have", map[string]any{"Class": "x"}, "Class"},
		{"a typo", map[string]any{"Atribute": "Name"}, "Atribute"},
		{"params of a non-template property", map[string]any{"attributeParams": []ast.ParamAssignmentV3{}}, "attributeParams"},
		{"designproperties: a column has none", map[string]any{"DesignProperties": nil}, "DesignProperties"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := validateObjectListItemUnknownProps(item(c.props), mapping, "page X")
			if c.wantBad == "" {
				if len(vs) > 0 {
					t.Fatalf("unexpected violation: %+v", vs)
				}
				return
			}
			if len(vs) != 1 {
				t.Fatalf("want one violation naming %q, got %+v", c.wantBad, vs)
			}
			if vs[0].RuleID != "MDL-WIDGET01" || !strings.Contains(vs[0].Message, "`"+c.wantBad+"`") {
				t.Errorf("violation = %s %q, want MDL-WIDGET01 naming %q", vs[0].RuleID, vs[0].Message, c.wantBad)
			}
		})
	}
}

// accordionRegistry loads a registry whose Accordion definition is generated
// from the real package, as `check -p` does. The definition comes from the
// project's .mpk, not the embedded set, so with no project there is no mapping
// to check against. The package is copied into a temp project because the
// loader writes generated definitions beside the project it reads.
func accordionRegistry(t *testing.T) *WidgetRegistry {
	t.Helper()
	const mpk = "com.mendix.widget.web.Accordion.mpk"
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pedapp", "widgets", mpk))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", mpk), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadWidgetRegistry(filepath.Join(dir, "App.mpr"))
}

// The reported shape, through the real parser and the shipped Accordion
// definition: `Caption` must be refused with the property that works.
func TestAccordionGroupCaptionIsRefused(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.accordion.Accordion' acc1 {
    group g1 (Caption: 'Group one') {
      dynamictext t1 (content: 'x')
    }
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	registry := accordionRegistry(t)
	var got []string
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], registry) {
		got = append(got, v.RuleID+": "+v.Message)
		if v.RuleID == "MDL-WIDGET01" && strings.Contains(v.Message, "`Caption`") {
			if !strings.Contains(v.Message, "headerText") {
				t.Errorf("the message does not name the property that works: %s", v.Message)
			}
			return
		}
	}
	t.Fatalf("no MDL-WIDGET01 for `Caption` on an Accordion group; got %v", got)
}

// The documented spelling stays clean — the guard must not refuse what the
// examples teach.
func TestAccordionGroupHeaderTextIsAccepted(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.accordion.Accordion' acc1 {
    group g1 (HeaderText: 'Group one', HeaderRenderMode: 'text') {
      dynamictext t1 (content: 'x')
    }
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], accordionRegistry(t)) {
		if v.RuleID == "MDL-WIDGET01" {
			t.Errorf("unexpected: %s", v.Message)
		}
	}
}

// A truncated key is answered with the keys that extend it, not the nearest by
// edit distance: `attributeValue` on an HTMLElement attribute drew "did you mean
// `attributeName`", the one key certain to be wrong.
func TestValidateObjectListItemUnknownProps_SuggestsExtensions(t *testing.T) {
	mapping := &ObjectListMapping{
		MDLContainer: "ATTRIBUTE",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "attributeName", Operation: "primitive"},
			{PropertyKey: "attributeValueType", Operation: "primitive"},
			{PropertyKey: "attributeValueTemplate", Operation: "texttemplate"},
			{PropertyKey: "attributeValueExpression", Operation: "expression"},
		},
	}
	w := &ast.WidgetV3{Type: "attribute", Name: "a1", Properties: map[string]any{"attributeValue": "y"}}
	vs := validateObjectListItemUnknownProps(w, mapping, "page X")
	if len(vs) != 1 {
		t.Fatalf("want one violation, got %+v", vs)
	}
	msg := vs[0].Message
	if strings.Contains(msg, "`attributeName`") || !strings.Contains(msg, "attributeValueTemplate") {
		t.Errorf("hint should name the attributeValue… keys, not attributeName: %s", msg)
	}
}
