// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// leaf / sub build the mock menu: a leaf has an action, a sub-menu does not.
func leaf(c string) *backend.MenuTreeNode { return &backend.MenuTreeNode{Caption: c} }
func sub(c string, items ...*backend.MenuTreeNode) *backend.MenuTreeNode {
	return &backend.MenuTreeNode{Caption: c, NoAction: true, Items: items}
}

// The proposal's example menu.
func exampleNavTree() []*backend.MenuTreeNode {
	return []*backend.MenuTreeNode{
		leaf("Home"),
		sub("Admin", leaf("Users"), leaf("Old import")),
		sub("Reports", leaf("Users")),
		leaf("Help"),
		leaf("Log out"),
	}
}

func treeString(tree []*backend.MenuTreeNode) string {
	var parts []string
	for _, n := range tree {
		s := "'" + n.Caption + "'"
		if len(n.Items) > 0 {
			s += "{" + treeString(n.Items) + "}"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// alterNavCtx is a mock project with a web profile Responsive holding the
// example menu, a native profile NativePhone and a menu document M.Menu; the
// returned mutators record what reaches them.
func alterNavCtx(t *testing.T) (*ExecContext, *mock.MenuMutator, *mock.MenuMutator) {
	t.Helper()
	nav := &mock.MenuMutator{Tree: exampleNavTree(), RoleHomes: map[string]types.NavHomePageSpec{
		"Administrator": {IsPage: true, Target: "Shop.AdminHome", ForRole: "Administrator"}}}
	menu := &mock.MenuMutator{Tree: []*backend.MenuTreeNode{leaf("One"), leaf("Two")}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{
				{Name: "Responsive", Kind: "Responsive"},
				{Name: "NativePhone", Kind: "NativePhone", IsNative: true},
			}}, nil
		},
		OpenNavigationProfileForMutationFunc: func(name string) (backend.NavigationProfileMutator, error) {
			return nav, nil
		},
		GetMenuDocumentByQualifiedNameFunc: func(module, name string) (*types.MenuDocument, error) {
			if module+"."+name != "M.Menu" {
				return nil, nil
			}
			return &types.MenuDocument{ID: model.ID("menu-1"), Name: name}, nil
		},
		OpenMenuDocumentForMutationFunc: func(model.ID) (backend.MenuMutator, error) { return menu, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx, nav, menu
}

func execAlter(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog := parseMDL(t, src)
	switch s := prog.Statements[0].(type) {
	case *ast.AlterNavigationStmt:
		return execAlterNavigation(ctx, s)
	case *ast.AlterMenuStmt:
		return execAlterMenu(ctx, s)
	}
	t.Fatalf("not an alter: %T", prog.Statements[0])
	return nil
}

func TestAlterNavigation_AppliesEveryOperationInOrder(t *testing.T) {
	ctx, nav, _ := alterNavCtx(t)
	err := execAlter(t, ctx, `alter navigation Responsive {
  insert after 'Home' { menu item 'Orders' ( OnClick: sign out ) }
  insert into 'Admin' { menu item 'Audit log' ( OnClick: open link 'https://example.com/audit' ) }
  set ( Caption: 'User accounts' ) on 'Admin' > 'Users';
  replace 'Help' with {
    menu 'Help' {
      menu item 'Documentation' ( OnClick: open link 'https://example.com/docs' )
    }
  }
  move 'Log out' before 'Help';
  drop 'Old import';
  insert into 'Help' { menu item 'Contact' ( OnClick: nothing ) }
  insert { menu item 'Last' }
  set login page Shop.CustomLogin;
  drop not found page;
  drop home page for Administrator;
}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "'Home' 'Orders' 'Admin'{'User accounts' 'Audit log'} 'Reports'{'Users'} 'Log out' 'Help'{'Documentation' 'Contact'} 'Last'"
	if got := treeString(nav.Tree); got != want {
		t.Errorf("menu after alter:\n got %s\nwant %s", got, want)
	}
	if !nav.Saved {
		t.Error("the statement was not saved")
	}
	if nav.LoginPage != "Shop.CustomLogin" || nav.NotFound != "" || len(nav.RoleHomes) != 0 {
		t.Errorf("page clauses: login %q notfound %q roles %v", nav.LoginPage, nav.NotFound, nav.RoleHomes)
	}
}

// Each refusal names the statement, the operation and the target, and nothing
// is saved — including the operations before the refused one (atomic).
func TestAlterNavigation_Refusals(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"ambiguous", `alter navigation Responsive { insert after 'Home' { menu item 'X' } drop 'Users' }`,
			[]string{"alter navigation Responsive: drop 'Users': menu item 'Users' is ambiguous", "'Admin' > 'Users'", "'Reports' > 'Users'"}},
		{"missing", `alter navigation Responsive { drop 'Userz' }`,
			[]string{"menu item 'Userz' not found", "'Home', 'Admin', 'Reports', 'Help', 'Log out'"}},
		{"ordinal out of range", `alter navigation Responsive { drop 'Users'@3 }`, []string{"has no @3: only 2 items match 'Users'"}},
		{"insert into an item with an action", `alter navigation Responsive { insert into 'Home' { menu item 'X' } }`,
			[]string{"insert into 'Home'", "CE0548", "set ( OnClick: nothing ) on 'Home'"}},
		{"move into an item with an action", `alter navigation Responsive { move 'Help' into 'Home' }`, []string{"CE0548"}},
		{"move into own subtree", `alter navigation Responsive { move 'Admin' into 'Admin' > 'Users' }`,
			[]string{"cannot be moved into its own sub-items"}},
		{"move relative to itself", `alter navigation Responsive { move 'Help' after 'Help' }`, []string{"the same item"}},
		{"OnClick on a sub-menu", `alter navigation Responsive { set ( OnClick: sign out ) on 'Admin' }`,
			[]string{"'Admin' has sub-items", "CE0548", "OnClick: nothing"}},
		{"bare drop home page", `alter navigation Responsive { drop home page }`, []string{"CE0527", "drop home page for <Role>"}},
		{"role without a home page", `alter navigation Responsive { drop home page for User }`, []string{"role User has no home page"}},
		{"native profile", `alter navigation NativePhone { drop 'Home' }`, []string{"native profile", "create or modify navigation NativePhone home page"}},
		{"unknown profile", `alter navigation Tablet { drop 'Home' }`, []string{"no navigation profile Tablet", "Responsive, NativePhone", "create or modify navigation Tablet"}},
		{"unknown menu", `alter menu M.Nope { drop 'One' }`, []string{"menu not found", "create menu M.Nope"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, nav, menu := alterNavCtx(t)
			err := execAlter(t, ctx, c.src)
			if err == nil {
				t.Fatalf("accepted: %s", c.src)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err.Error(), w)
				}
			}
			if nav.Saved || menu.Saved {
				t.Error("a refused statement was saved")
			}
		})
	}
}

// check -p --references runs the function exec runs over an unsaved copy, so
// it reports exactly what exec refuses, with exec's message, and saves
// nothing. A later alter of the same profile sees what an earlier one
// inserted; an alter of a profile the script rewrites first is not judged.
func TestAlterNavigation_CheckAgreesWithExec(t *testing.T) {
	src := `alter navigation Responsive { insert after 'Home' { menu item 'New' } }
alter navigation Responsive { drop 'New' }
alter navigation Responsive { drop 'Users' }
create or modify navigation Responsive home page Shop.Home;
alter navigation Responsive { drop 'Nothing here' }`
	ctx, nav, _ := alterNavCtx(t)
	errs := validateAlterNavigationReferences(ctx, parseMDL(t, src))
	if len(errs) != 1 {
		t.Fatalf("check reported %d problems, want 1 (statement 3): %v", len(errs), errs)
	}
	if nav.Saved {
		t.Error("check saved the menu")
	}

	ctx2, _, _ := alterNavCtx(t)
	execErr := execAlter(t, ctx2, `alter navigation Responsive { drop 'Users' }`)
	if execErr == nil {
		t.Fatal("exec accepted the ambiguous drop")
	}
	if got, want := errs[0].Error(), "statement 3: "+execErr.Error(); got != want {
		t.Errorf("check and exec disagree:\ncheck %s\nexec  %s", got, want)
	}
}

func TestAlterMenu_ItemOperationsOnAMenuDocument(t *testing.T) {
	ctx, nav, menu := alterNavCtx(t)
	if err := execAlter(t, ctx, `alter menu M.Menu {
  insert before 'Two' { menu item 'Between' ( OnClick: sign out ) }
  drop 'One'
}`); err != nil {
		t.Fatal(err)
	}
	if got := treeString(menu.Tree); got != "'Between' 'Two'" {
		t.Errorf("menu = %s", got)
	}
	if !menu.Saved || nav.Saved {
		t.Errorf("saved: menu %v, navigation %v", menu.Saved, nav.Saved)
	}
}

// `list navigation menu` prints the address alter takes for every item.
func TestListNavigationMenu_AddressColumn(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
				Name: "Responsive", Kind: "Responsive",
				MenuItems: []*types.NavMenuItem{
					{Caption: "Admin", Items: []*types.NavMenuItem{{Caption: "Users", Page: "Admin.Users"}}},
					{Caption: "Reports", Items: []*types.NavMenuItem{{Caption: "Users", Page: "Shop.Users"}}},
					{Caption: "Home", Page: "Shop.Home"},
					{Caption: "Home", Page: "Shop.Home2"},
				},
			}}}, nil
		},
	}
	ctx, out := newMockCtx(t, withBackend(mb))
	if err := listNavigationMenu(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"Address", "'Admin' > 'Users'", "'Reports' > 'Users'", "'Home'@2", "Admin.Users"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("listing lacks %q:\n%s", w, out.String())
		}
	}
}
