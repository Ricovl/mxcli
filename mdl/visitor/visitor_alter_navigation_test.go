// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func buildOneStmt[T ast.Statement](t *testing.T, src string) T {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("want 1 statement, got %d", len(prog.Statements))
	}
	s, ok := prog.Statements[0].(T)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

func TestAlterNavigation_EveryOperation(t *testing.T) {
	s := buildOneStmt[*ast.AlterNavigationStmt](t, `alter navigation Responsive {
  insert after 'Home' { menu item 'Orders' ( OnClick: show page Shop.Orders, Icon: Atlas_Core.Atlas."list-bullets" ) }
  insert before 'Admin'@2 > 'Users' { menu item 'A' }
  insert into 'Admin' {
    menu 'Group' { menu item 'Child' ( OnClick: sign out ) }
  }
  insert { menu item 'Last' }
  replace 'Help' with { menu item 'Help' ( OnClick: open link 'https://example.com' ) }
  move 'Log out' after 'Help';
  move 'X' into 'Settings' > 'Y'@3
  drop 'Old import', 'Admin' > 'Legacy'
  set ( Caption: 'it''s', OnClick: nothing, Icon: none ) on 'Admin' > 'Users';
  set ( Icon: glyph 57377 ) on ''@2
  set home page Shop.Home
  set home microflow Shop.ACT_Home for Administrator;
  drop home page for Administrator
  drop home page
  set login page Shop.Login
  drop login page;
  set not found page Shop.Oops
  drop not found page
}`)
	if s.ProfileName != "Responsive" {
		t.Errorf("profile = %q", s.ProfileName)
	}
	var got []string
	for _, op := range s.Operations {
		got = append(got, op.String())
	}
	want := []string{
		"insert after 'Home'",
		"insert before 'Admin'@2 > 'Users'",
		"insert into 'Admin'",
		"insert { … }",
		"replace 'Help'",
		"move 'Log out' after 'Help'",
		"move 'X' into 'Settings' > 'Y'@3",
		"drop 'Old import', 'Admin' > 'Legacy'",
		"set ( Caption, OnClick, Icon ) on 'Admin' > 'Users'",
		"set ( Icon ) on ''@2",
		"set home page Shop.Home",
		"set home microflow Shop.ACT_Home for Administrator",
		"drop home page for Administrator",
		"drop home page",
		"set login page Shop.Login",
		"drop login page",
		"set not found page Shop.Oops",
		"drop not found page",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("operations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	ins := s.Operations[0].(*ast.InsertMenuItemsOp)
	if ins.Items[0].Caption != "Orders" || ins.Items[0].Page == nil || ins.Items[0].IconKind != types.MenuIconCollection {
		t.Errorf("fragment item = %+v", ins.Items[0])
	}
	into := s.Operations[2].(*ast.InsertMenuItemsOp)
	if len(into.Items) != 1 || len(into.Items[0].Items) != 1 || !into.Items[0].Items[0].SignOut {
		t.Errorf("nested fragment = %+v", into.Items)
	}
	set := s.Operations[8].(*ast.SetMenuItemOp)
	if set.Caption == nil || *set.Caption != "it's" {
		t.Errorf("caption = %v", set.Caption)
	}
	if !set.SetAction || set.Item.Action == nil || set.Item.Action.Type != "none" {
		t.Errorf("OnClick: nothing = %+v", set.Item.Action)
	}
	if !set.SetIcon || set.Item.IconKind != types.MenuIconNone {
		t.Errorf("Icon: none = %v %v", set.SetIcon, set.Item.IconKind)
	}
	glyph := s.Operations[9].(*ast.SetMenuItemOp)
	if glyph.Item.IconKind != types.MenuIconGlyph || glyph.Item.IconCode != 57377 || glyph.Target.Segments[0].Ordinal != 2 {
		t.Errorf("glyph set = %+v", glyph)
	}
	home := s.Operations[11].(*ast.SetNavPageClauseOp)
	if home.Home.IsPage || home.Home.ForRole == nil || home.Home.ForRole.String() != "Administrator" {
		t.Errorf("role microflow home = %+v", home.Home)
	}
}

func TestAlterMenu_ItemOperations(t *testing.T) {
	s := buildOneStmt[*ast.AlterMenuStmt](t, `alter menu Atlas_Core.Phone_Menu {
  insert after 'Home' { menu item 'Orders' ( OnClick: show page Shop.Orders ) }
  drop 'Old'
}`)
	if s.Name.String() != "Atlas_Core.Phone_Menu" || len(s.Operations) != 2 {
		t.Fatalf("got %s with %d operations", s.Name, len(s.Operations))
	}
}

func TestAlterNavigation_Refusals(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`alter navigation Responsive { set ( Caption: 'a', Caption: 'b' ) on 'X' }`, "Caption is written twice"},
		{`alter navigation Responsive { set ( OnClick: save changes ) on 'X' }`, "a menu item cannot save changes"},
		{`alter navigation Responsive { set ( Title: 'x' ) on 'X' }`, ""},
		// page clauses are navigation-only
		{`alter menu M.Menu { set login page M.Login }`, ""},
		// positional addresses do not exist
		{`alter navigation Responsive { drop item@3 }`, ""},
	}
	for _, c := range cases {
		_, errs := Build(c.src)
		if len(errs) == 0 {
			t.Errorf("%s: accepted", c.src)
			continue
		}
		if c.want != "" && !strings.Contains(errs[0].Error(), c.want) {
			t.Errorf("%s: error %q lacks %q", c.src, errs[0].Error(), c.want)
		}
	}
}
