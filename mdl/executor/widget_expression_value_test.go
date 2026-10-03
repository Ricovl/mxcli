// SPDX-License-Identifier: Apache-2.0

// A bare Mendix expression on a pluggable widget's Expression-typed property —
// `dynamicBarColor: $currentObject/ColorHex` on a chart series,
// `attributeValueExpression: $currentObject/ColorHex` on an HTML element
// attribute — parsed as a variable-led data source, was stringified to "" by the
// builder and written as an empty expression. check and exec were clean, DESCRIBE
// showed nothing, and the chart drew its default colours. A longer expression
// (`if … then … else`) was refused by the visitor for every generic key, which
// is wrong for exactly these properties.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

const seriesExprPage = `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  columnchart chart1 {
    series s1 (
      dataSet: 'dynamic',
      dynamicDataSource: database from M.Sale,
      dynamicBarColor: %s
    )
  }
}`

func seriesItem(t *testing.T, value string) *ast.WidgetV3 {
	t.Helper()
	ws := pageWidgets(t, strings.Replace(seriesExprPage, "%s", value, 1))
	s1 := ws["s1"]
	if s1 == nil {
		t.Fatalf("series s1 not parsed from value %s", value)
	}
	return s1
}

var seriesExprMapping = &ObjectListMapping{
	PropertyKey:  "series",
	MDLContainer: "SERIES",
	ItemProperties: []ItemPropertyMapping{
		{PropertyKey: "dynamicBarColor", Operation: "expression"},
		{PropertyKey: "dynamicName", Operation: "texttemplate"},
	},
}

func TestBuildObjectListItem_BareExpressionReachesAnExpressionProperty(t *testing.T) {
	for _, value := range []string{
		`$currentObject/ColorHex`,
		`$currentObject/M.Sale_Region/M.Region/Hex`,
		`$Color`,
		`if $currentObject/Hot then 'red' else 'blue'`,
		`$currentObject/ColorHex + ''`,
		`'#ff0000'`, // control: a string literal is a constant expression, as before
	} {
		e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
		spec, err := e.buildObjectListItem(seriesExprMapping, seriesItem(t, value))
		if err != nil {
			t.Fatalf("%s: buildObjectListItem: %v", value, err)
		}
		prop, ok := itemPropertyByKey(spec.Properties, "dynamicBarColor")
		if !ok {
			t.Fatalf("%s: no dynamicBarColor property in the spec", value)
		}
		want := value
		if value == `'#ff0000'` {
			want = "#ff0000" // the quoted spelling stores its content, unchanged by this fix
		}
		if prop.Expression != want {
			t.Errorf("%s: expression = %q, want %q — the value was dropped", value, prop.Expression, want)
		}
	}
}

// The other side of the same decision: a scalar property that is NOT
// Expression-typed refuses an expression rather than writing it empty.
func TestBuildObjectListItem_ExpressionOnATextTemplateIsRefused(t *testing.T) {
	for _, value := range []string{`$currentObject/Region`, `if $currentObject/Hot then 'a' else 'b'`} {
		src := strings.Replace(seriesExprPage, "dynamicBarColor: %s", "dynamicName: "+value, 1)
		s1 := pageWidgets(t, src)["s1"]
		e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
		_, err := e.buildObjectListItem(seriesExprMapping, s1)
		if err == nil {
			t.Fatalf("%s on a text-template property was built — it is written as an empty template", value)
		}
		if !strings.Contains(err.Error(), "dynamicName") {
			t.Errorf("refusal does not name the property: %v", err)
		}
	}
}

func TestMDLWIDGET39_ExpressionOnANonExpressionProperty(t *testing.T) {
	cases := []struct {
		name, prop, value string
		want              int
	}{
		{"bare path on an expression property", "dynamicBarColor", `$currentObject/ColorHex`, 0},
		{"if-expression on an expression property", "dynamicBarColor", `if $currentObject/Hot then 'red' else 'blue'`, 0},
		{"bare path on a text template", "dynamicName", `$currentObject/Region`, 1},
		{"if-expression on a text template", "dynamicName", `if $currentObject/Hot then 'a' else 'b'`, 1},
		{"control: a plain text template", "dynamicName", `'Sales'`, 0},
	}
	for _, tc := range cases {
		src := strings.Replace(seriesExprPage, "dynamicBarColor: %s", tc.prop+": "+tc.value, 1)
		s1 := pageWidgets(t, src)["s1"]
		got := validateWidgetExpressionValues(s1, nil, seriesExprMapping, true, "page M.P")
		if len(got) != tc.want {
			t.Errorf("%s: %d violations, want %d: %v", tc.name, len(got), tc.want, got)
			continue
		}
		for _, v := range got {
			if v.RuleID != "MDL-WIDGET42" || !strings.Contains(v.Message, tc.prop) {
				t.Errorf("%s: got %s %q", tc.name, v.RuleID, v.Message)
			}
		}
	}
}

// A built-in widget has no Expression-typed generic property at all, so an
// expression on its generic key is refused — the error the visitor used to
// raise, now raised where the schema is known.
func TestMDLWIDGET39_ExpressionOnABuiltinWidget(t *testing.T) {
	ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (renderMode: 'a' + 'b') { }
}`)
	got := validateWidgetExpressionValues(ws["c1"], nil, nil, false, "page M.P")
	if len(got) != 1 || got[0].RuleID != "MDL-WIDGET42" {
		t.Fatalf("got %v, want one MDL-WIDGET42", got)
	}
}

// The top-level pass (4.6 in Build) routes on the template's ValueType.
func TestScalarPropertyText_TopLevelValueTypes(t *testing.T) {
	w := &ast.WidgetV3{Name: "w", Properties: map[string]any{
		"colorExpr": &ast.DataSourceV3{Type: "association", ContextVariable: "currentObject", Reference: "ColorHex"},
	}, ValueSource: map[string]string{"colorExpr": "$currentObject/ColorHex"}}
	got, err := scalarPropertyText(w, "colorexpr", w.Properties["colorExpr"], operationForValueType("Expression"))
	if err != nil || got != "$currentObject/ColorHex" {
		t.Errorf("Expression: got %q, %v", got, err)
	}
	if _, err := scalarPropertyText(w, "colorExpr", w.Properties["colorExpr"], operationForValueType("String")); err == nil {
		t.Error("String: an expression was accepted as a plain value")
	}
	if op := operationForValueType("DataSource"); op != "" {
		t.Errorf("DataSource maps to %q; a data source must keep its own path", op)
	}
}
