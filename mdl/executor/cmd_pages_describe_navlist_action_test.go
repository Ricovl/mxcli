// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#950 item 3: a navigation-list item's page action was described in
// the legacy `show_page 'Mod.Page'` form, which does not parse — TestApp's
// Rules.Entity_Menu gave three syntax errors. It now goes through the shared
// client-action renderer the buttons use, so every stored shape (FormSettings,
// PageSettings) and every other action kind reads back.
func TestNavigationListItemAction_DescribesAsParseableMDL(t *testing.T) {
	// The target pages take no parameters (TestApp's overview pages).
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	cases := []struct {
		name   string
		action map[string]any
		want   string
	}{
		{"FormSettings", map[string]any{"$Type": "Forms$FormAction",
			"FormSettings": map[string]any{"$Type": "Forms$FormSettings", "Form": "Rules.BusinessRule_Overview"}},
			"show page Rules.BusinessRule_Overview"},
		{"PageSettings", map[string]any{"$Type": "Forms$FormAction",
			"PageSettings": map[string]any{"$Type": "Forms$FormSettings", "Form": "Rules.RuleAction_Overview"}},
			"show page Rules.RuleAction_Overview"},
		{"microflow", map[string]any{"$Type": "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{"$Type": "Forms$MicroflowSettings", "Microflow": "Rules.ACT_Open"}},
			"call microflow Rules.ACT_Open"},
		{"sign out", map[string]any{"$Type": "Forms$SignOutClientAction"}, "sign out"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractNavigationListItemAction(ctx, map[string]any{"Action": c.action})
			if got != c.want {
				t.Fatalf("described %q, want %q", got, c.want)
			}
			prog, errs := visitor.Build("create snippet M.S {\n  navigationlist nl {\n    item i1 (Action: " + got +
				") {\n      dynamictext t (Content: 'x')\n    }\n  }\n}")
			if len(errs) > 0 {
				t.Fatalf("%q does not parse: %v", got, errs[0])
			}
			nl := prog.Statements[0].(*ast.CreateSnippetStmtV3).Widgets[0]
			if _, ok := nl.Children[0].Properties["Action"].(*ast.ActionV3); !ok {
				t.Errorf("item action is %T, want *ast.ActionV3", nl.Children[0].Properties["Action"])
			}
		})
	}
}
