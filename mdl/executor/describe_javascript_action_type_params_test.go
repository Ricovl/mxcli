// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// describeJSA describes one mock JavaScript action and returns the output.
func describeJSA(t *testing.T, jsa *types.JavaScriptAction) string {
	t.Helper()
	mod := mkModule("NanoflowCommons")
	jsa.BaseElement = model.BaseElement{ID: nextID("jsa")}
	jsa.ContainerID = mod.ID
	jsa.Platform = "All"
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return jsa, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "NanoflowCommons", Name: jsa.Name}))
	return buf.String()
}

func typeParam(name string) *types.TypeParameterDef {
	return &types.TypeParameterDef{BaseElement: model.BaseElement{ID: nextID("tp")}, Name: name}
}

// NanoflowCommons.GetStorageItemObject: the type parameter is declared by an
// `entity <Entity>` parameter, as for Java actions. Describe printed it a second
// time as `Name<Entity>` after the action name, which the grammar has no slot
// for, so the output did not parse.
func TestDescribeJavaScriptAction_DeclaredTypeParameterReparses(t *testing.T) {
	tp := typeParam("Entity")
	out := describeJSA(t, &types.JavaScriptAction{
		Name:           "GetStorageItemObject",
		TypeParameters: []*types.TypeParameterDef{tp},
		Parameters: []*types.JavaActionParameter{
			{Name: "Key", IsRequired: true, ParameterType: &types.StringType{}},
			{Name: "Entity", IsRequired: true, ParameterType: &types.EntityTypeParameterType{TypeParameterID: tp.ID, TypeParameterName: "Entity"}},
		},
		ReturnType: &types.TypeParameter{TypeParameterID: tp.ID, TypeParameter: "Entity"},
	})
	stmt := findStmt[*ast.CreateJavaScriptActionStmt](t, reparse(t, out), out)
	if len(stmt.TypeParameters) != 1 || stmt.TypeParameters[0] != "Entity" {
		t.Errorf("re-parsed type parameters %q, want [Entity]\n%s", stmt.TypeParameters, out)
	}
	if strings.Contains(out, "NOT re-executable") {
		t.Errorf("an expressible action is marked NOT re-executable:\n%s", out)
	}
}

// The shapes MDL has no spelling for must not print a statement that parses
// into something else: commented out under a NOT re-executable note, the script
// stays parseable and re-running it leaves the stored action alone.
func TestDescribeJavaScriptAction_InexpressibleTypeParameterIsCommentedOut(t *testing.T) {
	cases := map[string]*types.JavaScriptAction{
		// NanoflowCommons.GetGuid: a parameter typed by a type parameter that no
		// `entity <T>` parameter declares (mendixlabs/mxcli#1404's shape).
		"undeclared type parameter": func() *types.JavaScriptAction {
			tp := typeParam("TypeParameter")
			return &types.JavaScriptAction{
				Name:           "GetGuid",
				TypeParameters: []*types.TypeParameterDef{tp},
				Parameters: []*types.JavaActionParameter{
					{Name: "EntityObject", IsRequired: true, ParameterType: &types.TypeParameter{TypeParameterID: tp.ID, TypeParameter: "TypeParameter"}},
				},
				ReturnType: &types.StringType{},
			}
		}(),
		// NanoflowCommons.RefreshEntity: an entity selector bound to no type
		// parameter (null TypeParameterPointer, empty TypeParameters) printed as
		// `entity <>`.
		"unbound entity selector": {
			Name: "RefreshEntity",
			Parameters: []*types.JavaActionParameter{
				{Name: "EntityToRefresh", IsRequired: true, ParameterType: &types.EntityTypeParameterType{}},
			},
			ReturnType: &types.VoidType{},
		},
	}
	for name, jsa := range cases {
		t.Run(name, func(t *testing.T) {
			out := describeJSA(t, jsa)
			prog := reparse(t, out)
			for _, s := range prog.Statements {
				if _, ok := s.(*ast.CreateJavaScriptActionStmt); ok {
					t.Errorf("an inexpressible action printed a live statement:\n%s", out)
				}
			}
			if !strings.Contains(out, "-- NOT re-executable:") {
				t.Errorf("no NOT re-executable note:\n%s", out)
			}
		})
	}
}

// NanoflowCommons.GetStorageItemObjectList returns `list of Entity`, Entity a
// type parameter. exec wrote it as a list of the entity `.Entity` — the
// JavaScript twin of the Java action's #1183 — so the describe output that now
// parses still did not round-trip.
func TestCreateJavaScriptAction_ListOfTypeParameter(t *testing.T) {
	mod := mkModule("NanoflowCommons")
	var captured *types.JavaScriptAction
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			captured = jsa
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	src := "create javascript action NanoflowCommons.GetList(Entity: entity <\"Entity\"> not null, Items: List of \"Entity\")\n" +
		"  returns List of \"Entity\"\n  platform All\nas $$\nreturn [];\n$$;"
	stmt := findStmt[*ast.CreateJavaScriptActionStmt](t, reparse(t, src), src)
	assertNoError(t, execCreateJavaScriptAction(ctx, stmt))
	if captured == nil || len(captured.TypeParameters) != 1 {
		t.Fatalf("type parameters not written: %+v", captured)
	}
	tpID := captured.TypeParameters[0].ID
	for what, got := range map[string]any{"parameter Items": captured.Parameters[1].ParameterType, "return type": captured.ReturnType} {
		l, ok := got.(*types.ListType)
		if !ok || l.TypeParameterID != tpID || l.Entity != "" {
			t.Errorf("%s = %#v, want a list of type parameter %s", what, got, tpID)
		}
	}
}
