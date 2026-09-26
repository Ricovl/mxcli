// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// A Java or JavaScript action's ExportLevel and ActionDefaultReturnName have no
// MDL spelling — DESCRIBE prints them as comments at most — so a rewrite must
// carry them. Both create paths hardcoded ExportLevel "Public", which is not a
// member of either metamodel enum (JavaActionsExportLevel and
// JavaScriptActionsExportLevel are both API | Hidden), and the Java path wrote
// ActionDefaultReturnName "". Measured on the Blank template (ako/mxcli#705):
// describe -> exec of FeedbackModule.XSS_Sanitizer turned Hidden into Public
// and dropped "ReturnValueName". Studio Pro writes Hidden and ReturnValueName on
// all 69 Java and JavaScript actions in that template, so those are also the
// values for a new action.

func TestJavaActionRewriteCarriesExportLevelAndReturnName(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedExposedAction(mod)
	stored.ExportLevel = "API"
	stored.ActionDefaultReturnName = "IsValid"
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	assertNoError(t, execCreateJavaAction(ctx, rewriteStmt(false)))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	if got.ExportLevel != "API" {
		t.Errorf("ExportLevel = %q, want the stored API — MDL cannot express it, so a rewrite must carry it", got.ExportLevel)
	}
	if got.ActionDefaultReturnName != "IsValid" {
		t.Errorf("ActionDefaultReturnName = %q, want the stored IsValid", got.ActionDefaultReturnName)
	}
}

func TestCreateJavaActionWritesStudioProDefaults(t *testing.T) {
	mod := mkModule("MyModule")
	var captured *javaactions.JavaAction
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return nil, nil },
		CreateJavaActionFunc: func(ja *javaactions.JavaAction) error {
			captured = ja
			return nil
		},
		WriteJavaSourceFileFunc: func(string, string, string, []*javaactions.JavaActionParameter,
			javaactions.CodeActionReturnType, []string, string) error {
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	s := rewriteStmt(false)
	s.CreateOrModify = false
	assertNoError(t, execCreateJavaAction(ctx, s))

	if captured == nil {
		t.Fatal("CreateJavaAction was never called")
	}
	if captured.ExportLevel != "Hidden" {
		t.Errorf("ExportLevel = %q, want Hidden — \"Public\" is not a JavaActionsExportLevel", captured.ExportLevel)
	}
	if captured.ActionDefaultReturnName != "ReturnValueName" {
		t.Errorf("ActionDefaultReturnName = %q, want Studio Pro's ReturnValueName", captured.ActionDefaultReturnName)
	}
}

func TestJavaScriptActionExportLevel(t *testing.T) {
	mod := mkModule("WebMod")
	stored := &types.JavaScriptAction{
		BaseElement:             model.BaseElement{ID: nextID("jsa")},
		ContainerID:             mod.ID,
		Name:                    "DoThing",
		ExportLevel:             "API",
		ActionDefaultReturnName: "IsPictureTaken",
	}
	for _, tc := range []struct {
		name       string
		existing   []*types.JavaScriptAction
		wantLevel  string
		wantReturn string
	}{
		{"create writes Studio Pro's defaults", nil, "Hidden", "ReturnValueName"},
		{"rewrite carries the stored values", []*types.JavaScriptAction{stored}, "API", "IsPictureTaken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured *types.JavaScriptAction
			capture := func(jsa *types.JavaScriptAction) error {
				captured = jsa
				return nil
			}
			mb := &mock.MockBackend{
				IsConnectedFunc:            func() bool { return true },
				ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListJavaScriptActionsFunc:  func() ([]*types.JavaScriptAction, error) { return tc.existing, nil },
				CreateJavaScriptActionFunc: capture,
				UpdateJavaScriptActionFunc: capture,
				WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
					return nil
				},
			}
			h := mkHierarchy(mod)
			withContainer(h, stored.ContainerID, mod.ID)
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, execCreateJavaScriptAction(ctx, &ast.CreateJavaScriptActionStmt{
				Name:           ast.QualifiedName{Module: "WebMod", Name: "DoThing"},
				JavaScriptCode: "return Promise.resolve();",
				CreateOrModify: true,
			}))
			if captured == nil {
				t.Fatal("no write")
			}
			if captured.ExportLevel != tc.wantLevel {
				t.Errorf("ExportLevel = %q, want %q", captured.ExportLevel, tc.wantLevel)
			}
			if captured.ActionDefaultReturnName != tc.wantReturn {
				t.Errorf("ActionDefaultReturnName = %q, want %q", captured.ActionDefaultReturnName, tc.wantReturn)
			}
		})
	}
}
