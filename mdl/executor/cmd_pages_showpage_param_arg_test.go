// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1403: "Describe emits a `show page` argument taken from the
// page's own parameter, which check refuses (MDL-PAGEARG01)". Studio Pro stores
// such a button — outside every data container, passing the enclosing page's
// parameter — as an EXPLICIT Forms$PageParameterMapping whose Variable is a
// Forms$PageVariable naming the parameter (ako/TestApp WorkflowCommons
// TaskAssignmentHelper_UserTask_Reassign: PageParameter "TaskAssignmentHelper";
// Snip_UserTask_Detail: SnippetParameter "WorkflowUserTask"). DESCRIBE prints it
// as `Task = $Task`, and check refused that spelling as "no context object".
//
// A document parameter needs no context object: it is bound by name. Only what
// the context object would have to supply ($currentObject, a literal, a foreign
// variable) is still refused at the root.
func TestValidateShowPageArguments_DocumentParameterIsBoundByName(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}
	count := func(tree []*ast.WidgetV3, params []string) int {
		n := 0
		for _, v := range validateDocumentWidgetTree(tree, registry, "page Mod.P", params) {
			if v.RuleID == "MDL-PAGEARG01" {
				n++
			}
		}
		return n
	}
	params := []string{"SomeRef"}

	if n := count([]*ast.WidgetV3{showPageButton("$SomeRef")}, params); n != 0 {
		t.Errorf("page-level show page passing the page's own parameter: %d MDL-PAGEARG01, want 0", n)
	}
	if n := count([]*ast.WidgetV3{showPageButton("$someref")}, params); n != 0 {
		t.Errorf("parameter names are case-insensitive: %d MDL-PAGEARG01, want 0", n)
	}
	inDataView := []*ast.WidgetV3{{
		Type:       "dataview",
		Name:       "dv1",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Car"}},
		Children:   []*ast.WidgetV3{showPageButton("$SomeRef")},
	}}
	if n := count(inDataView, params); n != 0 {
		t.Errorf("a page parameter passed from inside a data view of another object: %d MDL-PAGEARG01, want 0", n)
	}

	// Controls: what still has no binding is still refused.
	for _, arg := range []string{"$Other", "$currentObject", "'literal'", "$SomeRef/Mod.Assoc"} {
		if n := count([]*ast.WidgetV3{showPageButton(arg)}, params); n != 1 {
			t.Errorf("page-level show page passing %s: %d MDL-PAGEARG01, want 1", arg, n)
		}
	}
	if n := count([]*ast.WidgetV3{showPageButton("$SomeRef")}, nil); n != 1 {
		t.Errorf("$SomeRef on a page that declares no such parameter: %d MDL-PAGEARG01, want 1", n)
	}
}

// The exec half: the builder writes the argument as the explicit mapping Studio
// Pro stores, instead of refusing it (and instead of the empty mapping, which
// would build to CE1571 as #1029 measured).
func TestBuildShowPage_DocumentParameterArgumentIsAnExplicitMapping(t *testing.T) {
	build := func(isSnippet bool, ctx pageArgContext, arg string) (*pages.PageClientAction, error) {
		pb := &pageBuilder{
			execCache:  &executorCache{createdPages: map[string]*createdPageInfo{"Mod.Detail": {ID: "detail-id"}}},
			paramScope: map[string]model.ID{"SomeRef": "entity-id"},
			isSnippet:  isSnippet,
			argCtx:     ctx,
		}
		built, err := pb.buildClientActionV3(showPageButton(arg).GetAction())
		if err != nil {
			return nil, err
		}
		return built.(*pages.PageClientAction), nil
	}

	for _, tc := range []struct {
		name      string
		isSnippet bool
		ctx       pageArgContext
		wantKind  string
	}{
		{"page root", false, atDocumentRoot(), "parameter"},
		{"snippet root", true, atDocumentRoot(), "snippet"},
		{"inside a data view on another object", false, enteringDataWidget(&ast.DataSourceV3{Type: "parameter", Reference: "$Car"}, "Mod.Car"), "parameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			act, err := build(tc.isSnippet, tc.ctx, "$SomeRef")
			if err != nil {
				t.Fatalf("show page passing the document parameter refused: %v", err)
			}
			if len(act.ParameterMappings) != 1 {
				t.Fatalf("mappings = %d, want 1", len(act.ParameterMappings))
			}
			m := act.ParameterMappings[0]
			if m.ParameterName != "Car" || m.Variable != "$SomeRef" || m.VariableKind != tc.wantKind {
				t.Errorf("mapping = %+v, want Car <- $SomeRef as %s", m, tc.wantKind)
			}
		})
	}

	// Control: the context object stays inferred — no VariableKind, so the
	// writer keeps the empty mapping #296 needs (an explicit $currentObject is
	// CE0115).
	act, err := build(false, enteringDataWidget(&ast.DataSourceV3{Type: "parameter", Reference: "$Car"}, "Mod.Car"), "$currentObject")
	if err != nil {
		t.Fatalf("$currentObject inside a data view refused: %v", err)
	}
	if kind := act.ParameterMappings[0].VariableKind; kind != "" {
		t.Errorf("$currentObject written as an explicit %q mapping, want inferred", kind)
	}
	// Control: a variable that is no parameter is still refused at the root.
	if _, err := build(false, atDocumentRoot(), "$Other"); err == nil {
		t.Error("$Other at the page root was accepted; it binds nothing")
	}
}
