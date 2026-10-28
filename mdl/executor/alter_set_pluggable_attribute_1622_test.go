// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1622: ALTER PAGE SET on a pluggable widget's attribute property.
// The mutator reports the property's scope; the executor resolves the binding
// by the rule CREATE uses — resolveBindingScope, misboundAttributeError,
// `$Param.Attr` — and hands the mutator an AttributeBinding. Before, every
// spelling reached the mutator as a plain value and was refused.

// scopedMutator is a MockPageMutator that also describes pluggable properties.
type scopedMutator struct {
	*mock.MockPageMutator
	scopes map[string]backend.PluggablePropertyScope
}

func (m *scopedMutator) PluggablePropertyScope(widgetRef, propKey string) (backend.PluggablePropertyScope, bool) {
	s, ok := m.scopes[widgetRef+"."+strings.ToLower(propKey)]
	return s, ok
}

// chartSetMutator: chart1 inside a data view over Sales.Invoice, on a page with
// $Ctx: Sales.Customer. eventDataAttribute is linked to no datasource;
// dataAttribute to the chart's own datasource over Sales.Order; title is a
// String property.
func chartSetMutator(got *any) *scopedMutator {
	invoice := backend.SourceScope{Entity: "Sales.Invoice"}
	return &scopedMutator{
		MockPageMutator: &mock.MockPageMutator{
			ParamScopeFunc: func() (map[string]model.ID, map[string]string) {
				return map[string]model.ID{"Ctx": "e-customer"}, map[string]string{"Ctx": "Sales.Customer"}
			},
			SetWidgetPropertyFunc: func(_, _ string, v any) error { *got = v; return nil },
		},
		scopes: map[string]backend.PluggablePropertyScope{
			"chart1.eventdataattribute": {Key: "eventDataAttribute", Kind: "Attribute", Enclosing: invoice},
			"chart1.dataattribute": {Key: "dataAttribute", Kind: "Attribute", DataSource: "datasource",
				Linked: backend.SourceScope{Entity: "Sales.Order"}, Enclosing: invoice},
			"chart1.title": {Key: "title", Kind: "String", Enclosing: invoice},
		},
	}
}

func setOnChart(t *testing.T, prop string, value any) (any, error) {
	t.Helper()
	var got any
	m := chartSetMutator(&got)
	op := &ast.SetPropertyOp{Target: ast.WidgetRef{Widget: "chart1"}, Properties: map[string]any{prop: value}}
	err := applySetProperties(scopeCheckCtx(t), m, op, "Sales", model.ID("mod-sales"))
	return got, err
}

func TestAlterSet_PluggableAttributeIsResolved(t *testing.T) {
	for _, tc := range []struct {
		prop  string
		value any
		path  string
		param string
	}{
		// Bare: against the property's own scope — the enclosing data view for
		// an unlinked property, the linked datasource for a linked one.
		{"eventDataAttribute", "Reference", "Sales.Invoice.Reference", ""},
		{"dataAttribute", "Number", "Sales.Order.Number", ""},
		// Qualified with the bound entity: as written.
		{"eventDataAttribute", "Sales.Invoice.Label", "Sales.Invoice.Label", ""},
		// `$Param.Attr`: the parameter's entity, and the parameter itself.
		{"eventDataAttribute", &ast.WidgetExpressionV3{Text: "$Ctx.Name"}, "Sales.Customer.Name", "Ctx"},
	} {
		got, err := setOnChart(t, tc.prop, tc.value)
		if err != nil {
			t.Errorf("%s = %v: %v", tc.prop, tc.value, err)
			continue
		}
		b, ok := got.(backend.AttributeBinding)
		if !ok {
			t.Errorf("%s = %v: mutator got %#v, want a backend.AttributeBinding", tc.prop, tc.value, got)
			continue
		}
		if b.Path != tc.path {
			t.Errorf("%s = %v: Path = %q, want %q", tc.prop, tc.value, b.Path, tc.path)
		}
		gotParam := ""
		if b.SourceVariable != nil {
			gotParam = b.SourceVariable.Variable
		}
		if gotParam != tc.param {
			t.Errorf("%s = %v: SourceVariable = %+v, want parameter %q", tc.prop, tc.value, b.SourceVariable, tc.param)
		}
	}
}

// The refusals are CREATE's, word for word: a bare name the bound entity lacks
// (CE1613), a qualified attribute of another entity (CE7247), a `$name` that is
// no parameter.
func TestAlterSet_PluggableAttributeRefusals(t *testing.T) {
	for _, tc := range []struct {
		prop  string
		value any
		want  []string
	}{
		{"eventDataAttribute", "Name", []string{"`Name`", "Sales.Invoice", "CE1613"}},
		{"dataAttribute", "Reference", []string{"`Reference`", "Sales.Order", "Sales.Invoice (the enclosing data container)"}},
		{"eventDataAttribute", "Sales.Customer.Name", []string{"CE7247", "$Param.Name"}},
		{"eventDataAttribute", &ast.WidgetExpressionV3{Text: "$Nope.Name"}, []string{"$Nope", "not a parameter"}},
		// An expression on a property that is not an attribute.
		{"title", &ast.WidgetExpressionV3{Text: "$Ctx.Name"}, []string{"title", "not an expression"}},
	} {
		got, err := setOnChart(t, tc.prop, tc.value)
		if err == nil {
			t.Errorf("%s = %v: accepted (%#v), want a refusal", tc.prop, tc.value, got)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s = %v: error should mention %q, got: %v", tc.prop, tc.value, w, err)
			}
		}
		if got != nil {
			t.Errorf("%s = %v: refused, but the mutator was still called with %#v", tc.prop, tc.value, got)
		}
	}
}

// Control: a property of another kind passes through unchanged.
func TestAlterSet_PluggableNonAttributePassesThrough(t *testing.T) {
	got, err := setOnChart(t, "title", "Sales")
	if err != nil || got != "Sales" {
		t.Errorf("title = 'Sales': got %#v, %v; want the plain value", got, err)
	}
}
