// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mendixlabs/mxcli#1519, item 3b:
//
//	set (ColumnWidth: manual, Size: 220) on dg column(Name)
//	set (ColumnWidth: autoFill) on dg column(Name)
//
// stored width=autoFill with size=220 — CE0463, measured on 11.6.6 and 11.15.0.
// The column setter changed one property and left the ones the widget hides
// because of it at whatever they held. The executor now hands the column's
// visibility rules — lifted from the installed Data grid 2 package, not written
// down here — to the mutator after a column SET.

type columnResetRecorder struct {
	backend.PageMutator
	set        []string
	resetCalls int
	rules      []types.WidgetVisibilityRule
	named      []string
}

func (r *columnResetRecorder) SetColumnProperty(_, _ string, prop string, _ any) error {
	r.set = append(r.set, prop)
	return nil
}

func (r *columnResetRecorder) ResetHiddenColumnProperties(_, _ string, rules []types.WidgetVisibilityRule, setProps []string) ([]string, error) {
	r.resetCalls++
	r.rules = rules
	r.named = setProps
	return nil, nil
}

func TestAlterColumnSet1519_ResetsWhatTheWidgetHides(t *testing.T) {
	mpr, err := filepath.Abs("../../testdata/expr-checker/minimal.mpr")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &ExecContext{MprPath: mpr}
	rec := &columnResetRecorder{}
	op := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "dg", ColumnAttribute: "Name"},
		Properties: map[string]any{"ColumnWidth": "autoFill"},
	}
	if !op.Target.IsColumn() {
		t.Fatalf("fixture target is not a column address: %+v", op.Target)
	}
	if err := applySetPropertyMutator(ctx, rec, op, "M", ""); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rec.resetCalls != 1 {
		t.Fatalf("hidden-property reset called %d times after a column SET, want 1", rec.resetCalls)
	}
	if len(rec.named) != 1 || rec.named[0] != "ColumnWidth" {
		t.Errorf("named properties = %v, want [ColumnWidth]", rec.named)
	}
	// The rule that matters, as the installed package states it.
	found := false
	for _, r := range rec.rules {
		if r.ListPropertyKey == "columns" && r.PropertyKey == "size" && r.HiddenWhen != nil &&
			r.HiddenWhen.PropertyKey == "width" && r.HiddenWhen.Operator == "ne" && r.HiddenWhen.Value == "manual" &&
			r.HiddenWhen.Scope == types.ConditionScopeItem {
			found = true
		}
	}
	if !found {
		t.Errorf("the Data grid 2 rule `size hidden unless width is manual` was not passed; got %d rules", len(rec.rules))
	}
}
