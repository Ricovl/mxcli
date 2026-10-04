// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func textDoc(s string) map[string]any {
	return map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
		map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": s}}}
}

// ako/mxcli#980: a menu item's stored action is described by the renderer a
// button's goes through, so each kind Studio Pro stores on a menu item —
// shapes measured on ako/TestApp — prints with its settings, and the output
// parses back to the same kind.
func TestDescribeNavigation_EveryMenuActionKind(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	cases := []struct {
		name string
		doc  map[string]any
		want string
		kind string
	}{
		{"nanoflow", map[string]any{"$Type": "Forms$CallNanoflowClientAction", "DisabledDuringExecution": true,
			"Nanoflow": "MyFirstModule.Nanoflow", "ProgressBar": "Blocking", "ProgressMessage": textDoc("Working"),
			"OutputMappings": []any{int32(3)}, "ParameterMappings": []any{int32(2)}},
			"call nanoflow MyFirstModule.Nanoflow with (ProgressBar: Blocking, ProgressMessage: 'Working')", "nanoflow"},
		{"microflow", map[string]any{"$Type": "Forms$MicroflowAction", "DisabledDuringExecution": true,
			"MicroflowSettings": map[string]any{"$Type": "Forms$MicroflowSettings", "Microflow": "M.F",
				"Asynchronous": true, "FormValidations": "None", "ProgressBar": "None",
				"ConfirmationInfo": map[string]any{"$Type": "Forms$ConfirmationInfo", "Question": textDoc("Sure?"),
					"ProceedButtonCaption": textDoc("Yes"), "CancelButtonCaption": textDoc("No")}}},
			"call microflow M.F with (Confirmation: 'Sure?', ProceedCaption: 'Yes', CancelCaption: 'No', Asynchronous: true, FormValidations: None)", "microflow"},
		{"open link", map[string]any{"$Type": "Forms$OpenLinkClientAction", "DisabledDuringExecution": true, "LinkType": "Web",
			"Address": map[string]any{"$Type": "Forms$StaticOrDynamicString", "IsDynamic": false, "Value": "https://x.org"}},
			"open link 'https://x.org'", "openLink"},
		{"create object", map[string]any{"$Type": "Forms$CreateObjectClientAction", "DisabledDuringExecution": true,
			"EntityRef":    map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "M.E"},
			"PageSettings": map[string]any{"$Type": "Forms$FormSettings", "Form": "M.E_New"}},
			"create object M.E then show page M.E_New", "create"},
		{"show page", map[string]any{"$Type": "Forms$FormAction", "DisabledDuringExecution": false,
			"FormSettings": map[string]any{"$Type": "Forms$FormSettings", "Form": "M.P"}},
			"show page M.P with (DisabledDuringExecution: false)", "showPage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			printMenuMDL(ctx, &buf, []*types.NavMenuItem{{Caption: "X", StoredAction: []byte("x"), ActionDoc: c.doc}}, 1, "CREATE NAVIGATION")
			want := "menu item 'X' ( OnClick: " + c.want + " )"
			if !strings.Contains(buf.String(), want) {
				t.Fatalf("described\n%s\nwant %s", buf.String(), want)
			}
			if strings.Contains(buf.String(), "--") {
				t.Errorf("an expressible action was flagged:\n%s", buf.String())
			}
			prog, errs := visitor.Build("create or modify navigation Responsive {\n" + buf.String() + "};")
			if len(errs) > 0 {
				t.Fatalf("does not parse: %v", errs[0])
			}
			if got := prog.Statements[0].(*ast.AlterNavigationStmt).MenuItems[0].Action; got == nil || got.Type != c.kind {
				t.Errorf("re-parsed as %+v, want %s", got, c.kind)
			}
		})
	}
}

// A stated action is built by the page builder and paired with the stored one
// it replaces, so the writer can keep what MDL cannot say about it; a stated
// action never takes the KeepAction path.
func TestNavigationRewrite_BuildsTheStatedAction(t *testing.T) {
	var got types.NavigationProfileSpec
	stored := []byte("stored")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{Name: "Responsive", Kind: "Responsive",
				MenuItems: []*types.NavMenuItem{{Caption: "Docs", StoredAction: stored,
					ActionDoc: map[string]any{"$Type": "Forms$OpenLinkClientAction", "LinkType": "Email"}}}}}}, nil
		},
		UpdateNavigationProfileFunc: func(_ model.ID, _ string, spec types.NavigationProfileSpec) error {
			got = spec
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	prog, errs := visitor.Build("create or modify navigation Responsive {\n  menu item 'Docs' ( OnClick: open link 'mailto:x' )\n};")
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	assertNoError(t, execAlterNavigation(ctx, prog.Statements[0].(*ast.AlterNavigationStmt)))
	it := got.MenuItems[0]
	if it.Action == nil || it.KeepAction != nil || !bytes.Equal(it.StoredAction, stored) {
		t.Errorf("stated action: Action=%T KeepAction=%q StoredAction=%q", it.Action, it.KeepAction, it.StoredAction)
	}
}

// A stored action whose target is unset renders as a bare `show page`, which
// does not parse; it is flagged and kept, not printed as an OnClick.
func TestDescribeNavigation_UntargetedActionIsFlagged(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	item := &types.NavMenuItem{Caption: "X", StoredAction: []byte("x"), ActionDoc: map[string]any{
		"$Type": "Forms$FormAction", "FormSettings": map[string]any{"$Type": "Forms$FormSettings", "Form": ""}}}
	onClick, note := menuItemActionMDL(ctx, item)
	if onClick != "" || !strings.Contains(note, "has no target MDL can name") {
		t.Errorf("onClick %q note %q", onClick, note)
	}
	if keep, _ := keepStoredMenuAction(ctx, ast.NavMenuItemDef{Caption: "X"}, item); !keep {
		t.Error("the untargeted stored action must be kept by a rewrite that states none")
	}
}
