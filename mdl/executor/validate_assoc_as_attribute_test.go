// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1440: a combo box bound to an association through the
// `Attribute:` key passed check and was refused by exec —
//
//	alter snippet M.S { replace <widget> with {
//	  combobox x (Attribute: $Asset.Asset_Customer, CaptionAttribute: …)
//	} };
//	check: "Check passed!"
//	exec:  "property attributeEnumeration binds Asset_Customer, which is an
//	        association … not an attribute"
//
// The fixture is the report's shape: snippet M.S with parameter $Asset of
// M.Asset, an association M.Asset_Customer from M.Asset to M.Customer, and a
// placeholder widget at the top of the snippet to replace.
func assocAsAttributeCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	snip := mkSnippet(mod.ID, "S")
	asset := &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID("e-asset")}, Name: "Asset",
		Attributes: []*domainmodel.Attribute{{Name: "Code"}}}
	customer := &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID("e-customer")}, Name: "Customer",
		Attributes: []*domainmodel.Attribute{{Name: "CustomerName"}}}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: model.ID("dm-m")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{asset, customer},
		Associations: []*domainmodel.Association{{Name: "Asset_Customer", ParentID: asset.ID, ChildID: customer.ID,
			Type: domainmodel.AssociationTypeReference}},
	}
	h := mkHierarchy(mod)
	withContainer(h, snip.ID, mod.ID)
	withContainer(h, dm.ID, mod.ID)

	stored := bson.D{
		{Key: "$Type", Value: "Forms$Snippet"},
		{Key: "Parameters", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$SnippetParameter"},
			{Key: "Name", Value: "Asset"},
			{Key: "ParameterType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "M.Asset"}}},
		}}},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "placeholder"}}}},
	}
	real := &modelsdkbackend.Backend{}
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:    func(string) (*model.Module, error) { return mod, nil },
		ListSnippetsFunc:       func() ([]*pages.Snippet, error) { return []*pages.Snippet{snip}, nil },
		ListDomainModelsFunc:   func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:     func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(stored, unitID, &countingDeps{}), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ctx.widgetRegistry = reg
	ctx.widgetRegistryLoaded = true
	return ctx
}

func checkAssocAsAttributeAlter(t *testing.T, ctx *ExecContext, widget string) []error {
	t.Helper()
	prog := parseMDL(t, "alter snippet M.S { replace placeholder with { "+widget+" } };")
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateAlterUnscopedBindings(ctx, prog, sc)
}

// The reported script: check now refuses what exec refuses, and the remedy
// names the association mode that builds (0 errors on mx 11.6.6).
func TestCheck1440_AlterComboBoxAttributeNamingAnAssociation(t *testing.T) {
	ctx := assocAsAttributeCtx(t)
	errs := checkAssocAsAttributeAlter(t, ctx,
		`combobox x (Attribute: $Asset.Asset_Customer, CaptionAttribute: M.Customer.CustomerName)`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	for _, want := range []string{"alter snippet M.S", "attributeEnumeration", "`Asset_Customer`",
		"association (M.Asset → M.Customer)", "CE1613",
		"`combobox x (Attribute: $Asset.M.Asset_Customer, DataSource: database M.Customer"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not name %q:\n%s", want, msg)
		}
	}
}

// Controls: an attribute of the parameter, and the association mode the
// remedy suggests, are not refused.
func TestCheck1440_AlterComboBoxControls(t *testing.T) {
	ctx := assocAsAttributeCtx(t)
	for _, w := range []string{
		`combobox x (Attribute: $Asset.Code)`,
		`combobox x (Attribute: $Asset.M.Asset_Customer, DataSource: database M.Customer, CaptionAttribute: CustomerName)`,
	} {
		if errs := checkAssocAsAttributeAlter(t, ctx, w); len(errs) != 0 {
			t.Errorf("%s: unexpected refusal %v", w, errs)
		}
	}
}

// CREATE SNIPPET had the same gap, in the pluggable attribute-scope walk.
func TestCheck1440_CreateSnippetComboBoxAttributeNamingAnAssociation(t *testing.T) {
	ctx := assocAsAttributeCtx(t)
	for _, tc := range []struct {
		src     string
		refused bool
	}{
		{`create snippet M.S2 (Params: ($Asset: M.Asset)) { combobox x (Attribute: $Asset.Asset_Customer) };`, true},
		{`create snippet M.S2 (Params: ($Asset: M.Asset)) { dataview dv (DataSource: $Asset) { combobox x (Attribute: Asset_Customer) } };`, true},
		{`create snippet M.S2 (Params: ($Asset: M.Asset)) { combobox x (Attribute: $Asset.Code) };`, false},
	} {
		prog := parseMDL(t, tc.src)
		s, ok := prog.Statements[0].(*ast.CreateSnippetStmtV3)
		if !ok {
			t.Fatalf("got %T", prog.Statements[0])
		}
		errs := validatePluggableAttributeScopes(ctx, "", s.Parameters, s.Widgets, newScriptContext())
		if got := len(errs) == 1 && strings.Contains(errs[0], "which is an association"); got != tc.refused {
			t.Errorf("%s:\n  refused=%v, want %v (errors %q)", tc.src, got, tc.refused, errs)
		}
	}
}
