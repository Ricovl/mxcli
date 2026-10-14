// SPDX-License-Identifier: Apache-2.0

package backend

import "testing"

func TestNavItemSelector_RoundTrip(t *testing.T) {
	for _, s := range []NavItemSelector{
		{List: "nav", Widget: "t1"},
		{List: "nav", Text: "Home"},
		{List: "nav", Text: "it's"},
		{List: "nav", Ordinal: 2},
		{List: "nav", Text: "Home", Ordinal: 3},
		{List: "nav", Widget: "t1", Ordinal: 1},
	} {
		got, ok := ParseNavItemSelector(s.String())
		if !ok || got != s {
			t.Errorf("ParseNavItemSelector(%q) = %+v, %v; want %+v", s.String(), got, ok, s)
		}
	}
}

func TestNavItemSelector_NotAWidgetName(t *testing.T) {
	for _, ref := range []string{"nav", "navItem", "nav item", "nav.item", "nav item()", "nav item@0", "nav item('Home'", "nav itemX@2", "grid column(Name)"} {
		if s, ok := ParseNavItemSelector(ref); ok {
			t.Errorf("ParseNavItemSelector(%q) = %+v; want not a selector", ref, s)
		}
	}
}
