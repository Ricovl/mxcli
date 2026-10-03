// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The check-time half of ako/mxcli#647: `check` judges a pluggable widget's
// unqualified attribute names by the rule exec writes them by, and refuses the
// same names exec refuses — with the same message, because it is the same
// function (misboundAttributeError). Before, check never asked, so the
// two-datasource VoxelViewer passed it and failed mx check ten times.

func scopeCheckCtx(t *testing.T) *ExecContext {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
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
	}))
	// The two-datasource definition the engine tests build with.
	def := multiSourceDef()
	ctx.widgetRegistry = &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{def.MDLName: def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}
	ctx.widgetRegistryLoaded = true
	return ctx
}

func scopeCheckPage(t *testing.T, props string) *ast.CreatePageStmtV3 {
	t.Helper()
	src := `create page Sales.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: database from Sales.Invoice) {
    pluggablewidget 'com.mendix.widget.web.combobox.Combobox' viewer (
      optionsSourceAssociationDataSource: database from Sales.Order,
      DatabaseSource: database from Sales.Customer,
      ` + props + `
    )
  }
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

func TestCheck_AttributeOfAnotherScopeIsRefused(t *testing.T) {
	ctx := scopeCheckCtx(t)
	s := scopeCheckPage(t, `staticAttribute: Name`)
	errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), newScriptContext())
	if len(errs) != 1 {
		t.Fatalf("want one binding error, got %q", errs)
	}
	for _, want := range []string{"viewer", "staticAttribute", "`Name`", "Sales.Invoice",
		"Sales.Customer (datasource `optionsSourceDatabaseDataSource`)"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}

	// And it is exec's refusal, word for word.
	e := scopeEngine(t)
	_, err := e.Build(multiSourceDef(), twoSourceWidget(map[string]any{"staticAttribute": "Name"}))
	if err == nil || !strings.Contains(err.Error(), errs[0]) {
		t.Errorf("check and exec disagree:\n check: %s\n exec:  %v", errs[0], err)
	}
}

// The control: the names the rule places are not refused — the enclosing
// object's attribute on the unlinked property, each datasource's own on the
// linked ones, and a name every candidate has.
func TestCheck_AttributesTheRulePlacesPass(t *testing.T) {
	ctx := scopeCheckCtx(t)
	for _, props := range []string{
		`staticAttribute: Reference`,
		`staticAttribute: Label`,
		`optionsSourceAssociationCaptionAttribute: Number`,
		`optionsSourceDatabaseValueAttribute: Name`,
	} {
		s := scopeCheckPage(t, props)
		if errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), newScriptContext()); len(errs) > 0 {
			t.Errorf("%s: unexpected refusal %q", props, errs)
		}
	}
}

// A linked property given the ENCLOSING object's attribute is refused too: the
// widget binds it to its datasource's items.
func TestCheck_LinkedPropertyGivenTheEnclosingAttributeIsRefused(t *testing.T) {
	ctx := scopeCheckCtx(t)
	s := scopeCheckPage(t, `optionsSourceAssociationCaptionAttribute: Reference`)
	errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "Sales.Order") ||
		!strings.Contains(errs[0], "Sales.Invoice (the enclosing data container)") {
		t.Fatalf("want one refusal naming Sales.Order and the enclosing Sales.Invoice, got %q", errs)
	}
}

// Wired into the reference pass a page goes through.
func TestCheck_AttributeScopeIsPartOfThePageReferencePass(t *testing.T) {
	ctx := scopeCheckCtx(t)
	sc := newScriptContext()
	sc.modules["Sales"] = true
	err := validateWithContext(ctx, scopeCheckPage(t, `staticAttribute: Name`), sc)
	if err == nil || !strings.Contains(err.Error(), "attribute binding errors") {
		t.Fatalf("want the page refused for its attribute binding, got %v", err)
	}
}
