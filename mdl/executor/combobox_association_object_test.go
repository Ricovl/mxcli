// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1420: a combo box in association mode, replacing a widget of
// a snippet whose object comes from a snippet parameter, outside every data
// container —
//
//	combobox x (Association: M.A, DataSource: database M.E, CaptionAttribute: Attr)
//
// passed check and exec, and mx check (reproduced on 11.6.6) then reported
//
//	[CE7007] "Selected value is not valid for entity 'Asset_Customer/Customer'.
//	Please select the value again." at Combo box 'x'
//
// The binding that builds names the parameter: `Attribute: $Asset.M.Asset_Customer`
// (SourceVariable = the snippet parameter, as Studio Pro stores it; 0 errors),
// and so does wrapping it in `dataview (DataSource: $Asset)`.

const unanchoredComboBox = `combobox x (Association: M.Asset_Customer, DataSource: database M.Customer, CaptionAttribute: CustomerName)`

// Exec: the widget engine refuses the association when the widget's Build
// started with no entity in scope.
func TestExec1420_ComboBoxAssociationWithoutObjectIsRefused(t *testing.T) {
	reg := LoadWidgetRegistry("")
	def, ok := reg.Get("COMBOBOX")
	if !ok {
		t.Fatal("no COMBOBOX definition in the built-in registry")
	}
	w := parseSnippetWidget(t, unanchoredComboBox)
	var assoc PropertyMapping
	e := &PluggableWidgetEngine{currentDef: def}
	mappings, _, err := e.selectMappings(def, w)
	if err != nil {
		t.Fatalf("selectMappings: %v", err)
	}
	for _, m := range mappings {
		if m.Source == "Association" {
			assoc = m
		}
	}
	if assoc.PropertyKey == "" {
		t.Fatal("association mode has no Association mapping")
	}

	resolve := func(outer string) error {
		// The option list's DataSource mapping has already moved entityContext
		// to M.Customer, as it does in Build.
		e := &PluggableWidgetEngine{currentDef: def, pageBuilder: &pageBuilder{entityContext: "M.Customer"},
			outerEntityContext: outer, inBuild: true}
		_, err := e.resolveMapping(assoc, w)
		return err
	}
	err = resolve("")
	if err == nil || !strings.Contains(err.Error(), "CE7007") || !strings.Contains(err.Error(), "$Param.M.Asset_Customer") {
		t.Fatalf("no enclosing object: want a CE7007 refusal naming the parameter spelling, got %v", err)
	}
	// Control: inside a data container over M.Asset the same widget builds.
	if err := resolve("M.Asset"); err != nil {
		t.Errorf("inside a data container: unexpected refusal %v", err)
	}
}

// Check, no project: the widget-tree walk refuses it at a document root.
func TestCheck1420_ComboBoxAssociationAtDocumentRoot(t *testing.T) {
	count := func(src string) (n int, msg string) {
		for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
			if v.RuleID == "MDL-WIDGET46" && strings.Contains(v.Message, "CE7007") {
				n++
				msg = v.Message
			}
		}
		return
	}
	if n, msg := count(`create snippet M.S (Params: ($Asset: M.Asset)) { ` + unanchoredComboBox + ` };`); n != 1 {
		t.Errorf("combo box at the snippet root: %d refusals, want 1", n)
	} else if !strings.Contains(msg, "`Association: M.Asset_Customer`") {
		t.Errorf("refusal does not quote the binding: %s", msg)
	}
	// Controls: the parameter spelling, a data view around it, and the
	// enumeration mode are not this rule's.
	for _, src := range []string{
		`create snippet M.S (Params: ($Asset: M.Asset)) { combobox x (Attribute: $Asset.M.Asset_Customer, DataSource: database M.Customer, CaptionAttribute: CustomerName) };`,
		`create snippet M.S (Params: ($Asset: M.Asset)) { dataview dv (DataSource: $Asset) { ` + unanchoredComboBox + ` } };`,
		`create snippet M.S (Params: ($Asset: M.Asset)) { dataview dv (DataSource: $Asset) { combobox x (Attribute: Status) } };`,
	} {
		if n, msg := count(src); n != 0 {
			t.Errorf("%s:\n  unexpected refusal %s", src, msg)
		}
	}
}

// Check with a project: an ALTER's REPLACE at the top of a stored snippet —
// the reported shape — is refused against the stored insertion point.
func TestCheck1420_AlterReplaceAtSnippetRoot(t *testing.T) {
	mod := mkModule("M")
	snip := mkSnippet(mod.ID, "S")
	h := mkHierarchy(mod)
	withContainer(h, snip.ID, mod.ID)
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Snippet"},
		{Key: "Parameters", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$SnippetParameter"},
			{Key: "Name", Value: "Asset"},
			{Key: "ParameterType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "M.Asset"}}},
		}}},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "placeholder"}}}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		ListSnippetsFunc:    func() ([]*pages.Snippet, error) { return []*pages.Snippet{snip}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(stored, unitID, &countingDeps{}), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	check := func(widgets string) []error {
		prog := parseMDL(t, "alter snippet M.S { replace placeholder with { "+widgets+" } };")
		sc := newScriptContext()
		sc.collectDefinitions(prog)
		return validateAlterUnscopedBindings(ctx, prog, sc)
	}
	errs := check(unanchoredComboBox)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "alter snippet M.S: combobox `x`") ||
		!strings.Contains(errs[0].Error(), "CE7007") {
		t.Fatalf("want one CE7007 refusal for combobox x, got %v", errs)
	}
	for _, ok := range []string{
		`combobox x (Attribute: $Asset.M.Asset_Customer, DataSource: database M.Customer, CaptionAttribute: CustomerName)`,
		`dataview dv (DataSource: $Asset) { ` + unanchoredComboBox + ` }`,
	} {
		if errs := check(ok); len(errs) != 0 {
			t.Errorf("%s: unexpected refusal %v", ok, errs)
		}
	}
}

func parseSnippetWidget(t *testing.T, widget string) *ast.WidgetV3 {
	t.Helper()
	prog := parseMDL(t, "create snippet M.S { "+widget+" };")
	s, ok := prog.Statements[0].(*ast.CreateSnippetStmtV3)
	if !ok || len(s.Widgets) != 1 {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s.Widgets[0]
}
