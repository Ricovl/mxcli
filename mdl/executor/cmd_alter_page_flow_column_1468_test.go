// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// A Data grid 2 whose data source is a microflow stores no entity in its own
// BSON — the entity is the flow's return type. The column INSERT/REPLACE paths
// read only the BSON, so a replacement column's `Attribute: Balance` resolved
// against no entity, stayed unqualified, and was not written: mxbuild then
// failed with "An attribute is required when column sorting is enabled" on the
// flow-sourced grids only (mendixlabs/mxcli#1468).

func flowSourcedGridMutator(onColumns func(cols []*backend.DataGridColumnSpec)) *mock.MockPageMutator {
	return &mock.MockPageMutator{
		FindWidgetFunc:  func(string) bool { return false },
		WidgetScopeFunc: func() map[string]model.ID { return map[string]model.ID{"dgMicroflow": "grid-id"} },
		ParamScopeFunc:  func() (map[string]model.ID, map[string]string) { return nil, nil },
		// The BSON walk finds the grid's own source, which names no entity.
		EnclosingEntityForChildrenFunc: func(string) string { return "" },
		EnclosingDataSourceFlowFunc: func(widgetRef string, forChildren bool) (string, string) {
			if widgetRef == "dgMicroflow" && forChildren {
				return "MyModule.DS_Accounts", ""
			}
			return "", ""
		},
		ReplaceColumnFunc: func(_, _ string, cols []*backend.DataGridColumnSpec) error {
			onColumns(cols)
			return nil
		},
		InsertColumnsFunc: func(_, _ string, _ backend.InsertPosition, cols []*backend.DataGridColumnSpec) error {
			onColumns(cols)
			return nil
		},
	}
}

func flowSourcedGridCtx(t *testing.T) *ExecContext {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withCache(&executorCache{
		createdMicroflows: map[string]*createdMicroflowInfo{
			"MyModule.DS_Accounts": {ID: "mf-1", Name: "DS_Accounts", ModuleName: "MyModule", ReturnEntityName: "MyModule.Account"},
		},
	}))
	return ctx
}

func customContentBalanceColumn() *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "column",
		Properties: map[string]any{
			"Attribute":     "Balance",
			"Caption":       "Amount",
			"ShowContentAs": "customContent",
		},
		Children: []*ast.WidgetV3{
			{Type: "dynamictext", Name: "txtAmountB", Properties: map[string]any{"Content": "{1}"}},
		},
	}
}

func TestAlterPageReplaceColumn_FlowSourcedGridKeepsAttribute_1468(t *testing.T) {
	var got []*backend.DataGridColumnSpec
	mutator := flowSourcedGridMutator(func(cols []*backend.DataGridColumnSpec) { got = cols })
	op := &ast.ReplaceWidgetOp{
		Target:     ast.WidgetRef{Widget: "dgMicroflow", Column: "Balance"},
		NewWidgets: []*ast.WidgetV3{customContentBalanceColumn()},
	}
	if err := applyReplaceWidgetMutator(flowSourcedGridCtx(t), mutator, op, "MyModule", "mod-id", nil); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 column, got %d", len(got))
	}
	if got[0].Attribute != "MyModule.Account.Balance" {
		t.Errorf("column Attribute = %q, want MyModule.Account.Balance (the microflow's return entity)", got[0].Attribute)
	}
}

func TestAlterPageInsertColumn_FlowSourcedGridKeepsAttribute_1468(t *testing.T) {
	var got []*backend.DataGridColumnSpec
	mutator := flowSourcedGridMutator(func(cols []*backend.DataGridColumnSpec) { got = cols })
	op := &ast.InsertWidgetOp{
		Position: "AFTER",
		Target:   ast.WidgetRef{Widget: "dgMicroflow", Column: "Code"},
		Widgets:  []*ast.WidgetV3{customContentBalanceColumn()},
	}
	if err := applyInsertWidgetMutator(flowSourcedGridCtx(t), mutator, op, "MyModule", "mod-id"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 column, got %d", len(got))
	}
	if got[0].Attribute != "MyModule.Account.Balance" {
		t.Errorf("column Attribute = %q, want MyModule.Account.Balance (the microflow's return entity)", got[0].Attribute)
	}
}
