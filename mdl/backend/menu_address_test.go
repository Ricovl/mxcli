// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func node(caption string, items ...*MenuTreeNode) *MenuTreeNode {
	return &MenuTreeNode{Caption: caption, NoAction: len(items) > 0, Items: items}
}

func addr(segs ...any) types.MenuItemAddress {
	var a types.MenuItemAddress
	for _, s := range segs {
		switch v := s.(type) {
		case string:
			a.Segments = append(a.Segments, types.MenuItemAddressSegment{Caption: v})
		case int:
			a.Segments[len(a.Segments)-1].Ordinal = v
		}
	}
	return a
}

// The proposal's example menu: two items captioned Users under different
// parents.
func exampleMenu() []*MenuTreeNode {
	return []*MenuTreeNode{
		node("Home"),
		node("Admin", node("Users"), node("Old import")),
		node("Reports", node("Users")),
		node("Help"),
		node("Log out"),
	}
}

func TestResolveMenuItemAddress_Forms(t *testing.T) {
	tree := []*MenuTreeNode{
		node("Home"),
		node("Admin", node("Users"), node("Users"), node("Deep", node("Users"))),
		node("Admin", node("Roles")),
		node("Reports", node("Users")),
		node(""),
		node(""),
	}
	cases := []struct {
		name string
		a    types.MenuItemAddress
		want MenuItemPath
	}{
		{"bare caption, top level", addr("Home"), MenuItemPath{0}},
		{"bare caption, any depth", addr("Roles"), MenuItemPath{2, 0}},
		{"path is a suffix", addr("Deep", "Users"), MenuItemPath{1, 2, 0}},
		{"longer path", addr("Admin", "Deep", "Users"), MenuItemPath{1, 2, 0}},
		{"ambiguous intermediate segment is fine when the whole path is unique", addr("Admin", "Roles"), MenuItemPath{2, 0}},
		{"@n on the last segment", addr("Admin", "Users", 2), MenuItemPath{1, 1}},
		{"@n on the first segment", addr("Admin", 2, "Roles"), MenuItemPath{2, 0}},
		{"@n over the whole tree, describe order", addr("Users", 3), MenuItemPath{1, 2, 0}},
		{"@n over the whole tree, last", addr("Users", 4), MenuItemPath{3, 0}},
		{"empty caption with @n", addr("", 2), MenuItemPath{5}},
	}
	for _, c := range cases {
		got, err := ResolveMenuItemAddress(c.a, tree)
		if err != nil {
			t.Errorf("%s: %s: %v", c.name, c.a, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %s resolved to %v, want %v", c.name, c.a, got, c.want)
		}
	}
}

func TestResolveMenuItemAddress_DirectStepOnly(t *testing.T) {
	// `'Admin' > 'Users'` is one direct step: a Users two levels below Admin
	// is not it.
	tree := []*MenuTreeNode{node("Admin", node("Deep", node("Users")))}
	if _, err := ResolveMenuItemAddress(addr("Admin", "Users"), tree); err == nil {
		t.Fatal("'Admin' > 'Users' resolved through an intermediate level; `>` must be a direct parent/child step")
	}
}

func TestResolveMenuItemAddress_Ambiguous(t *testing.T) {
	_, err := ResolveMenuItemAddress(addr("Users"), exampleMenu())
	var ae *MenuAddressError
	if !errors.As(err, &ae) {
		t.Fatalf("want a *MenuAddressError, got %v", err)
	}
	var got []string
	for _, c := range ae.Candidates {
		got = append(got, c.String())
	}
	want := []string{"'Admin' > 'Users'", "'Reports' > 'Users'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %q, want %q", got, want)
	}
	msg := err.Error()
	for _, w := range []string{"menu item 'Users' is ambiguous: it matches 2 items", "  'Admin' > 'Users'", "  'Reports' > 'Users'"} {
		if !strings.Contains(msg, w) {
			t.Errorf("message %q lacks %q", msg, w)
		}
	}
}

func TestResolveMenuItemAddress_Misses(t *testing.T) {
	tree := exampleMenu()
	cases := []struct {
		a    types.MenuItemAddress
		want []string
	}{
		{addr("Userz"), []string{"menu item 'Userz' not found", "'Home', 'Admin', 'Reports', 'Help', 'Log out'"}},
		{addr("users"), []string{"differ only in letter case: 'Admin' > 'Users', 'Reports' > 'Users'"}},
		{addr("Admin", "Roles"), []string{"'Admin' has the sub-items 'Users', 'Old import'"}},
		{addr("Home", "Roles"), []string{"'Home' has no sub-items"}},
		{addr("Users", 3), []string{"has no @3: only 2 items match 'Users'"}},
		{addr("Admin", "Users", 2), []string{"has no @2: only 1 item matches 'Admin' > 'Users'"}},
	}
	for _, c := range cases {
		_, err := ResolveMenuItemAddress(c.a, tree)
		if err == nil {
			t.Errorf("%s resolved; want a refusal", c.a)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: message %q lacks %q", c.a, err.Error(), w)
			}
		}
	}
	if _, err := ResolveMenuItemAddress(addr("Home"), nil); err == nil || !strings.Contains(err.Error(), "the menu is empty") {
		t.Errorf("empty menu: %v", err)
	}
}

func TestMenuItemAddressString(t *testing.T) {
	a := addr("it's", 2, "Users")
	if got, want := a.String(), "'it''s'@2 > 'Users'"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestShortestMenuItemAddress_Preferences(t *testing.T) {
	tree := []*MenuTreeNode{
		node("Admin", node("Users"), node("Users")),
		node("Reports", node("Users")),
		node("Home"),
		node("Home"),
	}
	cases := []struct {
		path MenuItemPath
		want string
	}{
		{MenuItemPath{1, 0}, "'Reports' > 'Users'"},
		{MenuItemPath{0, 1}, "'Admin' > 'Users'@2"},
		{MenuItemPath{3}, "'Home'@2"},
		{MenuItemPath{0}, "'Admin'"},
	}
	for _, c := range cases {
		if got := ShortestMenuItemAddress(tree, c.path).String(); got != c.want {
			t.Errorf("ShortestMenuItemAddress(%v) = %s, want %s", c.path, got, c.want)
		}
	}
}

// Every address ShortestMenuItemAddress prints resolves back to exactly the
// item it was computed for — over generated trees with heavy caption reuse,
// which is where an ordinal or a too-short suffix would go wrong.
func TestShortestMenuItemAddress_RoundTripsOnGeneratedTrees(t *testing.T) {
	rng := rand.New(rand.NewSource(1522))
	captions := []string{"A", "B", "C", ""}
	var gen func(depth int) []*MenuTreeNode
	gen = func(depth int) []*MenuTreeNode {
		n := rng.Intn(4)
		if depth == 0 {
			n = 1 + rng.Intn(4)
		}
		var out []*MenuTreeNode
		for i := 0; i < n; i++ {
			nd := &MenuTreeNode{Caption: captions[rng.Intn(len(captions))]}
			if depth < 3 && rng.Intn(2) == 0 {
				nd.Items = gen(depth + 1)
			}
			out = append(out, nd)
		}
		return out
	}
	checked := 0
	for round := 0; round < 300; round++ {
		tree := gen(0)
		for _, e := range preorder(tree) {
			a := ShortestMenuItemAddress(tree, e.path)
			got, err := ResolveMenuItemAddress(a, tree)
			if err != nil {
				t.Fatalf("round %d: address %s of %v does not resolve: %v", round, a, e.path, err)
			}
			if !reflect.DeepEqual(got, e.path) {
				t.Fatalf("round %d: address %s of %v resolves to %v", round, a, e.path, got)
			}
			checked++
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d items checked; the generator is too small to mean anything", checked)
	}
}
