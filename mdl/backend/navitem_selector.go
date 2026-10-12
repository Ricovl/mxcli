// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"strconv"
	"strings"
)

// NavItemSelector is the address of a navigation list item (mendixlabs/mxcli#1388):
//
//	nav item(t1)          the item holding the widget named t1
//	nav item('Home')      the item whose text is Home
//	nav item@2            the second item
//	nav item('Home')@2    the second of several items whose text is Home
//
// Studio Pro stores no name on an item, so — like a DataGrid 2 column
// (ColumnSelector) — an item is addressed by what it shows, or by position.
// An address that matches more than one item is refused unless @n picks one.
//
// It travels through the PageMutator interface as the widgetRef string, spelled
// by String, so every operation that takes a widget reaches an item without a
// second set of methods. That spelling cannot be mistaken for a widget name: no
// widget name contains a space.
type NavItemSelector struct {
	// List is the navigation list's name.
	List string
	// Widget is the name of a widget inside the item. At most one of Widget
	// and Text is set; with neither, Ordinal counts every item.
	Widget string
	// Text is a text the item shows: a text or dynamic text's content, in any
	// language.
	Text string
	// Ordinal is @n, 1-based; 0 when absent.
	Ordinal int
}

const navItemSelectorInfix = " item"

// String spells the selector as MDL writes it.
func (s NavItemSelector) String() string {
	var b strings.Builder
	b.WriteString(s.List)
	b.WriteString(navItemSelectorInfix)
	switch {
	case s.Text != "":
		b.WriteString("('" + strings.ReplaceAll(s.Text, "'", "''") + "')")
	case s.Widget != "":
		b.WriteString("(" + s.Widget + ")")
	}
	if s.Ordinal > 0 {
		b.WriteString("@" + strconv.Itoa(s.Ordinal))
	}
	return b.String()
}

// ParseNavItemSelector reads a widgetRef spelled by NavItemSelector.String. ok
// is false for anything else, including every widget name.
func ParseNavItemSelector(ref string) (NavItemSelector, bool) {
	list, rest, found := strings.Cut(ref, navItemSelectorInfix)
	if !found || list == "" || strings.ContainsAny(list, " ()'") {
		return NavItemSelector{}, false
	}
	s := NavItemSelector{List: list}
	switch {
	case strings.HasPrefix(rest, "('"):
		var b strings.Builder
		i := 2
		for {
			if i >= len(rest) {
				return NavItemSelector{}, false
			}
			if rest[i] == '\'' {
				if i+1 < len(rest) && rest[i+1] == '\'' {
					b.WriteByte('\'')
					i += 2
					continue
				}
				break
			}
			b.WriteByte(rest[i])
			i++
		}
		s.Text = b.String()
		rest = rest[i+1:]
		if s.Text == "" || !strings.HasPrefix(rest, ")") {
			return NavItemSelector{}, false
		}
		rest = rest[1:]
	case strings.HasPrefix(rest, "("):
		end := strings.IndexByte(rest, ')')
		if end <= 1 {
			return NavItemSelector{}, false
		}
		s.Widget = rest[1:end]
		rest = rest[end+1:]
	}
	if rest == "" {
		if s.Widget == "" && s.Text == "" {
			return NavItemSelector{}, false // `nav item` alone addresses nothing
		}
		return s, true
	}
	n, found := strings.CutPrefix(rest, "@")
	if !found {
		return NavItemSelector{}, false
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 {
		return NavItemSelector{}, false
	}
	s.Ordinal = v
	return s, true
}
