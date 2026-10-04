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

// ako/mxcli#980: a menu item whose stored action MDL cannot spell (a stored
// $Type outside what describe prints) was described without its action, and the
// rewrite of that description stored Forms$NoAction in its place — TestApp's
// 'Item 4', a nanoflow call, lost its action at exit 0. The stored action is
// now carried when the script's item states none, and describe says so.
func TestNavigationRewrite_KeepsAnActionMDLCannotExpress(t *testing.T) {
	stored := []byte("stored-action-bytes")
	var got types.NavigationProfileSpec
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
				Name: "Responsive", Kind: "Responsive",
				MenuItems: []*types.NavMenuItem{
					{Caption: "Reports", ActionType: "Forms$UnknownFutureClientAction", StoredAction: stored},
					{Caption: "Admin", ActionType: "NoAction", Items: []*types.NavMenuItem{
						{Caption: "Logs", ActionType: "Forms$UnknownFutureClientAction", StoredAction: stored},
					}},
					{Caption: "Home", ActionType: "Forms$UnknownFutureClientAction", StoredAction: stored},
				},
			}}}, nil
		},
		UpdateNavigationProfileFunc: func(_ model.ID, _ string, spec types.NavigationProfileSpec) error {
			got = spec
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	prog, errs := visitor.Build(`create or modify navigation Responsive {
  menu item 'Reports'
  menu 'Admin' { menu item 'Logs' }
  menu item 'Home' ( OnClick: show page M.Home )
};`)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	assertNoError(t, execAlterNavigation(ctx, prog.Statements[0].(*ast.AlterNavigationStmt)))

	if len(got.MenuItems) != 3 {
		t.Fatalf("spec has %d items, want 3", len(got.MenuItems))
	}
	if !bytes.Equal(got.MenuItems[0].KeepAction, stored) {
		t.Errorf("'Reports' states no action: the stored one must be kept, got KeepAction=%q", got.MenuItems[0].KeepAction)
	}
	if len(got.MenuItems[1].Items) != 1 || !bytes.Equal(got.MenuItems[1].Items[0].KeepAction, stored) {
		t.Errorf("sub-item 'Admin' > 'Logs' must be paired by caption path, got %+v", got.MenuItems[1].Items)
	}
	// CONTROL: an item the script gives an action is written from the script.
	// A converter that kept every stored action would pass the two checks above.
	if got.MenuItems[2].KeepAction != nil || got.MenuItems[2].Page != "M.Home" {
		t.Errorf("'Home' states show page M.Home and must not keep the stored action: %+v", got.MenuItems[2])
	}
	if !strings.Contains(buf.String(), "kept the stored action of menu item 'Reports'") {
		t.Errorf("a kept action must be reported, got:\n%s", buf.String())
	}
}

// Describe says an action is there that it cannot print, rather than printing
// the item as if it had none.
func TestDescribeNavigation_FlagsAnActionMDLCannotExpress(t *testing.T) {
	var buf bytes.Buffer
	printMenuMDL(&buf, []*types.NavMenuItem{
		{Caption: "Reports", ActionType: "Forms$UnknownFutureClientAction", StoredAction: []byte("x")},
		{Caption: "Plain", ActionType: "NoAction", StoredAction: []byte("x")},
	}, 1, "CREATE NAVIGATION")
	out := buf.String()
	if !strings.Contains(out, "menu item 'Reports': its action (Forms$UnknownFutureClientAction) has no MDL form") {
		t.Errorf("unsupported action not flagged:\n%s", out)
	}
	// CONTROL: a plain item has nothing to flag.
	if strings.Contains(out, "'Plain': its action") {
		t.Errorf("a NoAction item was flagged:\n%s", out)
	}
}
