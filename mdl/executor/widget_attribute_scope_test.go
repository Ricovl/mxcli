// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#647. A pluggable widget with TWO datasources, placed inside a data
// view — mxcli-demo-2's VoxelViewer shape — bound every unqualified attribute
// name to the LAST datasource's entity: the placement attributes (linked to the
// widget's own `placements` datasource) and the root attribute (linked to none,
// so bound to the enclosing data view's object) alike. check passed; mx check
// reported CE1613 on each.
//
// Studio Pro's rule is the widget's own: widget.xml's `dataSource="…"` on an
// attribute property names the datasource whose items it binds against, and an
// attribute property WITHOUT one binds against the context object — the nearest
// enclosing data container's entity. Neither says "the last datasource".
//
// The real ComboBox template stands in for the VoxelViewer: it declares two
// datasources, attributes linked to each, and attributes linked to neither, and
// its links come from the widget package rather than from this test.

// scopeEngine returns an engine building INSIDE a data view over Sales.Invoice
// (pageBuilder.entityContext is what an enclosing data container leaves there).
func scopeEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-invoice")}, Name: "Invoice",
						Attributes: []*domainmodel.Attribute{{Name: "Reference"}, {Name: "Label"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-order")}, Name: "Order",
						Attributes: []*domainmodel.Attribute{{Name: "Number"}, {Name: "Label"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-customer")}, Name: "Customer",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}, {Name: "Label"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    "Sales.Invoice",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

// twoSourceWidget is the VoxelViewer shape: both datasources named by their
// schema keys (the Customer one last), plus attributes authored by key.
func twoSourceWidget(props map[string]any) *ast.WidgetV3 {
	all := map[string]any{
		"WidgetType":                         "com.mendix.widget.web.combobox.Combobox",
		"optionsSourceAssociationDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
		"DatabaseSource":                     &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	}
	for k, v := range props {
		all[k] = v
	}
	return &ast.WidgetV3{Name: "viewer", Type: "pluggablewidget", Properties: all}
}

func TestBuild_UnqualifiedAttributesBindByTheWidgetsOwnScope(t *testing.T) {
	e := scopeEngine(t)
	w := twoSourceWidget(map[string]any{
		// Linked to optionsSourceAssociationDataSource (Sales.Order). Mapped in
		// multiSourceDef AND authored by key, so both passes see it.
		"optionsSourceAssociationCaptionAttribute": "Number",
		// Linked to NO datasource: the enclosing data view's object.
		"staticAttribute": "Reference",
	})

	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)

	for _, want := range []string{"Sales.Order.Number", "Sales.Invoice.Reference"} {
		if !strings.Contains(got, want) {
			t.Errorf("built widget lacks %q", want)
		}
	}
	// The reported symptom: every name against the last datasource's entity.
	for _, wrong := range []string{"Sales.Customer.Number", "Sales.Customer.Reference"} {
		if strings.Contains(got, wrong) {
			t.Errorf("built widget binds %q — the LAST datasource's entity (#647)", wrong)
		}
	}
}

// The same rule through a property MAPPING rather than the explicit-key pass: an
// unlinked attribute mapping binds to the enclosing object, not to whichever
// datasource mapping ran before it.
func TestBuild_UnlinkedAttributeMappingBindsToEnclosingContext(t *testing.T) {
	def := multiSourceDef()
	def.PropertyMappings = append(def.PropertyMappings,
		PropertyMapping{PropertyKey: "staticAttribute", Source: "Attribute", Operation: "attribute"})
	e := scopeEngine(t)
	w := twoSourceWidget(map[string]any{"staticAttribute": "Reference"})

	widget, err := e.Build(def, w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)
	if !strings.Contains(got, "Sales.Invoice.Reference") {
		t.Errorf("unlinked attribute mapping did not bind to the enclosing Sales.Invoice")
	}
	if strings.Contains(got, "Sales.Customer.Reference") {
		t.Errorf("unlinked attribute mapping bound to the last datasource's entity (#647)")
	}
}

// A name that is not an attribute of the entity its property binds to, but IS
// one of another entity in the widget's scope, is refused with the candidates —
// writing it could only produce CE1613, and the candidates are the fix.
func TestBuild_AttributeOfAnotherScopeIsRefusedNamingCandidates(t *testing.T) {
	e := scopeEngine(t)
	// `Name` is Sales.Customer's; staticAttribute binds to the enclosing Invoice.
	w := twoSourceWidget(map[string]any{"staticAttribute": "Name"})

	_, err := e.Build(multiSourceDef(), w)
	if err == nil {
		t.Fatal("expected a refusal: `Name` is not an attribute of the enclosing Sales.Invoice")
	}
	for _, want := range []string{"staticAttribute", "Name", "Sales.Invoice", "Sales.Customer", "DatabaseSource"} {
		if want == "DatabaseSource" {
			want = "optionsSourceDatabaseDataSource"
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q, got: %v", want, err)
		}
	}
}

// The control for the refusal: a name every candidate has binds by the rule and
// is not refused — the rule, not the name's popularity, decides.
func TestBuild_AttributeOnEveryCandidateBindsByTheRule(t *testing.T) {
	e := scopeEngine(t)
	w := twoSourceWidget(map[string]any{"staticAttribute": "Label"})

	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, widget); !strings.Contains(got, "Sales.Invoice.Label") {
		t.Errorf("`Label` should bind to the enclosing Sales.Invoice")
	}
}
