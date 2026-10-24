// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1560 (mendixlabs/mxcli#1628): "a grid or list view … calls
// M.ACT_Open whose parameter is filled in from the container; describe prints
// `Action: microflow M.ACT_Open` and check rejects it with CE1571".
//
// A list widget's OWN click action fires per row, and the row is what fills
// the parameter in: Studio Pro stores it with no parameter mappings at all
// (TestApp ConflictedWorkflowDefinitionView dataGrid2_2, ConflictResolution
// dataGrid2_1, WorkflowDefinition_Overview dataGrid21). The rule judged that
// action against the context AROUND the widget, as for a button. Measured on
// mxbuild 10.24.25, one project, one `mx check`, every action with no argument:
//
//	list view, Action: ACT_OpenItem($Item: Item), source Item  → no error
//	data grid 2, OnClick: the same                             → no error
//	gallery, OnClick: the same                                 → no CE1571
//	action button, no data context (control)                   → CE1571
//
// It is the rule argContextForOwnAction already applies to show page (#552).

// listWidget is a list widget of the given type bound to an entity, with its own
// click action under "Action" — where both `Action:` and `OnClick:` land.
func listWidget(typ, name, entity string, action *ast.ActionV3, extra map[string]any) *ast.WidgetV3 {
	props := map[string]any{"DataSource": &ast.DataSourceV3{Type: "database", Reference: entity}}
	if action != nil {
		props["Action"] = action
	}
	for k, v := range extra {
		props[k] = v
	}
	return &ast.WidgetV3{Name: name, Type: typ, Properties: props}
}

func openItemSigs() map[string]*flowSignature {
	return map[string]*flowSignature{
		"m.act_openitem": objSig("Item", "M.Item"),
		"m.mf_other":     {Returns: "M.Other"},
	}
}

func openItem() *ast.ActionV3 { return &ast.ActionV3{Type: "microflow", Target: "M.ACT_OpenItem"} }

func TestRowAction_ListWidgetsOwnClickIsRowScoped(t *testing.T) {
	for _, typ := range []string{"listview", "datagrid", "gallery"} {
		t.Run(typ, func(t *testing.T) {
			page := []*ast.WidgetV3{listWidget(typ, "lst", "M.Item", openItem(), nil)}
			if errs := validateFlowArgumentsIn(nil, page, openItemSigs(), sameName); len(errs) != 0 {
				t.Errorf("the row supplies the argument (measured: no CE1571); got %v", errs)
			}
		})
	}
}

// The nanoflow call and a row object of a specialization are the same case.
func TestRowAction_NanoflowAndSpecializationAreRowScoped(t *testing.T) {
	sigs := map[string]*flowSignature{"m.nf_openitem": objSig("Item", "M.Item")}
	page := []*ast.WidgetV3{
		listWidget("datagrid", "dg", "M.Item", &ast.ActionV3{Type: "nanoflow", Target: "M.NF_OpenItem"}, nil),
	}
	if errs := validateFlowArgumentsIn(nil, page, sigs, sameName); len(errs) != 0 {
		t.Errorf("nanoflow row action reported: %v", errs)
	}
}

// Controls. Each is the same widget with one thing changed, and each still
// reports — so the tests above cannot pass through the rule having gone quiet.

// The row is the wrong type: the row does not supply it, and nothing else does.
func TestRowAction_RowOfAnotherTypeIsReported(t *testing.T) {
	page := []*ast.WidgetV3{listWidget("listview", "lstOther", "M.Other", openItem(), nil)}
	errs := validateFlowArgumentsIn(nil, page, openItemSigs(), sameName)
	if len(errs) != 1 || !strings.Contains(errs[0], "CE1571") || !strings.Contains(errs[0], "lstOther") {
		t.Fatalf("a row of another type supplies nothing; got %v", errs)
	}
}

// A primitive parameter is never filled in from a row.
func TestRowAction_PrimitiveParameterIsReported(t *testing.T) {
	sigs := map[string]*flowSignature{
		"m.act_openitem": {Params: []flowParam{{Name: "Code", Object: false}}},
	}
	page := []*ast.WidgetV3{listWidget("datagrid", "dgCode", "M.Item", openItem(), nil)}
	if errs := validateFlowArgumentsIn(nil, page, sigs, sameName); len(errs) != 1 {
		t.Fatalf("a primitive parameter is never supplied by a row; got %v", errs)
	}
}

// A slot that does not fire per row — a gallery's onSelectionChange, whose
// template links no data source — is judged where the widget sits, as before.
func TestRowAction_NonRowSlotKeepsTheEnclosingContext(t *testing.T) {
	page := []*ast.WidgetV3{
		listWidget("gallery", "galSel", "M.Item", nil, map[string]any{"onSelectionChange": openItem()}),
	}
	errs := validateFlowArgumentsIn(nil, page, openItemSigs(), sameName)
	if len(errs) != 1 || !strings.Contains(errs[0], "onSelectionChange") {
		t.Fatalf("onSelectionChange is not row-scoped; got %v", errs)
	}
}

// A button outside every data context — the measured CE1571 control.
func TestRowAction_LooseButtonStillReported(t *testing.T) {
	page := []*ast.WidgetV3{{
		Name: "btnLoose", Type: "actionbutton",
		Properties: map[string]any{"Action": openItem()},
	}}
	if errs := validateFlowArgumentsIn(nil, page, openItemSigs(), sameName); len(errs) != 1 {
		t.Fatalf("measured CE1571; got %v", errs)
	}
}

// A data source is never its own context: a list widget whose microflow source
// takes an Item is still reported with no Item around it, row action or not.
func TestRowAction_DataSourceStaysOnTheEnclosingContext(t *testing.T) {
	sigs := map[string]*flowSignature{
		"m.ds_items":     {Params: []flowParam{{Name: "Item", Entity: "M.Item", Object: true}}, Returns: "M.Item"},
		"m.act_openitem": objSig("Item", "M.Item"),
	}
	w := &ast.WidgetV3{Name: "dgDS", Type: "datagrid", Properties: map[string]any{
		"DataSource": &ast.DataSourceV3{Type: "microflow", Reference: "M.DS_Items"},
		"Action":     openItem(),
	}}
	errs := validateFlowArgumentsIn(nil, []*ast.WidgetV3{w}, sigs, sameName)
	if len(errs) != 1 || !strings.Contains(errs[0], "data source") {
		t.Fatalf("want the data source reported and the row action not; got %v", errs)
	}
}
