// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// MenuTreeNode is one menu item as an ALTER NAVIGATION / ALTER MENU target
// sees it: the caption describe prints (model.SelectTranslation), whether its
// action is Studio Pro's "Do nothing" (so it may take sub-items without
// CE0548), and its sub-items. Both a navigation profile's menu and a menu
// document are read into this shape, so one resolver serves both.
type MenuTreeNode struct {
	Caption  string
	NoAction bool
	Items    []*MenuTreeNode
}

// MenuItemPath locates an item by its index at each level, outermost first.
// It is how a resolved address is handed to a MenuMutator, never something MDL
// spells: ADR-0012 rules out positional addresses.
type MenuItemPath []int

// Parent is the path of the item's parent; nil for a top-level item.
func (p MenuItemPath) Parent() MenuItemPath {
	if len(p) <= 1 {
		return nil
	}
	return append(MenuItemPath(nil), p[:len(p)-1]...)
}

// Index is the item's position among its siblings.
func (p MenuItemPath) Index() int {
	if len(p) == 0 {
		return -1
	}
	return p[len(p)-1]
}

// HasPrefix reports whether p lies in the subtree of prefix (or is it).
func (p MenuItemPath) HasPrefix(prefix MenuItemPath) bool {
	if len(prefix) > len(p) {
		return false
	}
	for i := range prefix {
		if p[i] != prefix[i] {
			return false
		}
	}
	return true
}

// MenuNodeAt returns the node at path, or nil.
func MenuNodeAt(roots []*MenuTreeNode, path MenuItemPath) *MenuTreeNode {
	level := roots
	var n *MenuTreeNode
	for _, i := range path {
		if i < 0 || i >= len(level) {
			return nil
		}
		n = level[i]
		level = n.Items
	}
	return n
}

// MenuTreeFromNavItems builds the target tree from the semantic menu items a
// backend reads, for listings.
func MenuTreeFromNavItems(items []*types.NavMenuItem) []*MenuTreeNode {
	out := make([]*MenuTreeNode, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, &MenuTreeNode{
			Caption:  it.Caption,
			NoAction: it.ActionType == "" || it.ActionType == "NoAction",
			Items:    MenuTreeFromNavItems(it.Items),
		})
	}
	return out
}

// menuEntry is a node with its path and its place in describe order.
type menuEntry struct {
	node  *MenuTreeNode
	path  MenuItemPath
	order int
}

// preorder lists every node in describe order: an item, then its sub-items,
// then its next sibling.
func preorder(roots []*MenuTreeNode) []menuEntry {
	var out []menuEntry
	var walk func(level []*MenuTreeNode, prefix MenuItemPath)
	walk = func(level []*MenuTreeNode, prefix MenuItemPath) {
		for i, n := range level {
			p := append(append(MenuItemPath(nil), prefix...), i)
			out = append(out, menuEntry{node: n, path: p, order: len(out)})
			walk(n.Items, p)
		}
	}
	walk(roots, nil)
	return out
}

// MenuAddressError is a menu item address that did not resolve to exactly one
// item. Its message names the address in MDL and, for an ambiguity, lists the
// shortest unique address of every candidate — so the author can copy one.
type MenuAddressError struct {
	Address types.MenuItemAddress
	// Candidates are the shortest unique addresses of the matches of an
	// ambiguous address; empty for a miss.
	Candidates []types.MenuItemAddress
	// Detail is the miss's or the out-of-range ordinal's explanation.
	Detail string
}

func (e *MenuAddressError) Error() string {
	if len(e.Candidates) > 0 {
		lines := make([]string, len(e.Candidates))
		for i, c := range e.Candidates {
			lines[i] = "  " + c.String()
		}
		return fmt.Sprintf("menu item %s is ambiguous: it matches %d items — write one of these:\n%s",
			e.Address, len(e.Candidates), strings.Join(lines, "\n"))
	}
	return fmt.Sprintf("menu item %s %s", e.Address, e.Detail)
}

// ResolveMenuItemAddress finds the one item an address names.
//
// The first segment matches any item in the tree; each following segment
// matches only the direct sub-items of what the segment before it matched, so
// a path matches as a suffix of an item's caption path. A segment's @n picks
// the n-th of its matches in describe order; without @n every match is kept,
// so an intermediate segment may be ambiguous as long as the whole path is not.
// Captions compare exactly, as describe prints them.
//
// It never guesses: a miss, an ambiguity and an @n beyond the matches are each
// a *MenuAddressError.
func ResolveMenuItemAddress(addr types.MenuItemAddress, roots []*MenuTreeNode) (MenuItemPath, error) {
	matches, err := resolveMenuMatches(addr, roots)
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return matches[0].path, nil
	}
	cands := make([]types.MenuItemAddress, len(matches))
	for i, m := range matches {
		cands[i] = ShortestMenuItemAddress(roots, m.path)
	}
	return nil, &MenuAddressError{Address: addr, Candidates: cands}
}

// resolveMenuMatches applies the address's segments and returns every item
// the whole address matches, in describe order. A miss or an @n out of range
// is an error; several matches are not (the caller decides).
func resolveMenuMatches(addr types.MenuItemAddress, roots []*MenuTreeNode) ([]menuEntry, error) {
	if len(addr.Segments) == 0 {
		return nil, &MenuAddressError{Address: addr, Detail: "is empty"}
	}
	all := preorder(roots)
	order := make(map[*MenuTreeNode]menuEntry, len(all))
	for _, e := range all {
		order[e.node] = e
	}

	var current []menuEntry
	for i, seg := range addr.Segments {
		var next []menuEntry
		if i == 0 {
			for _, e := range all {
				if e.node.Caption == seg.Caption {
					next = append(next, e)
				}
			}
		} else {
			for _, c := range current {
				for _, child := range c.node.Items {
					if child.Caption == seg.Caption {
						next = append(next, order[child])
					}
				}
			}
			sort.Slice(next, func(a, b int) bool { return next[a].order < next[b].order })
		}
		if len(next) == 0 {
			return nil, &MenuAddressError{Address: addr, Detail: menuMissDetail(addr, i, current, all, roots)}
		}
		if seg.Ordinal > 0 {
			if seg.Ordinal > len(next) {
				noun := "items match"
				if len(next) == 1 {
					noun = "item matches"
				}
				return nil, &MenuAddressError{Address: addr, Detail: fmt.Sprintf(
					"has no @%d: only %d %s %s", seg.Ordinal, len(next), noun,
					types.MenuItemAddress{Segments: prefixWithoutOrdinal(addr.Segments[:i+1])})}
			}
			next = []menuEntry{next[seg.Ordinal-1]}
		}
		current = next
	}
	return current, nil
}

// prefixWithoutOrdinal is segs with the last segment's @n removed, for a
// message about how many items that segment matched.
func prefixWithoutOrdinal(segs []types.MenuItemAddressSegment) []types.MenuItemAddressSegment {
	out := append([]types.MenuItemAddressSegment(nil), segs...)
	out[len(out)-1].Ordinal = 0
	return out
}

// menuMissDetail explains a miss at segment i: what is at the level that was
// searched, and any item whose caption differs only in letter case.
func menuMissDetail(addr types.MenuItemAddress, i int, context []menuEntry, all []menuEntry, roots []*MenuTreeNode) string {
	seg := addr.Segments[i]
	var level []string
	seen := map[string]bool{}
	add := func(c string) {
		q := types.QuoteMenuCaption(c)
		if !seen[q] {
			seen[q] = true
			level = append(level, q)
		}
	}
	where := "the menu has"
	if i == 0 {
		for _, n := range roots {
			add(n.Caption)
		}
		if len(roots) == 0 {
			return "not found: the menu is empty — `insert { … }` adds its first items"
		}
	} else {
		where = types.MenuItemAddress{Segments: addr.Segments[:i]}.String() + " has the sub-items"
		for _, c := range context {
			for _, ch := range c.node.Items {
				add(ch.Caption)
			}
		}
		if len(level) == 0 {
			return fmt.Sprintf("not found: %s has no sub-items",
				types.MenuItemAddress{Segments: addr.Segments[:i]})
		}
	}
	msg := fmt.Sprintf("not found: %s %s", where, strings.Join(level, ", "))
	if i == 0 {
		msg += " at the top level, and a caption matches at any depth"
	}
	var folded []string
	for _, e := range all {
		if e.node.Caption != seg.Caption && strings.EqualFold(e.node.Caption, seg.Caption) {
			folded = append(folded, ShortestMenuItemAddress(roots, e.path).String())
		}
	}
	if len(folded) > 0 {
		msg += "; captions compare exactly, and these differ only in letter case: " + strings.Join(folded, ", ")
	}
	return msg
}

// ShortestMenuItemAddress is the address describe-order readers should write
// for the item at path: the shortest suffix of its caption path that resolves
// to it alone; when none does (siblings sharing a caption), its parent's
// shortest address followed by `> 'Caption'@n`, n counting the parent's
// sub-items with that caption — or `'Caption'@n` over the whole tree for a
// top-level item. It always resolves back to the item.
func ShortestMenuItemAddress(roots []*MenuTreeNode, path MenuItemPath) types.MenuItemAddress {
	captions := make([]string, 0, len(path))
	level := roots
	for _, i := range path {
		if i < 0 || i >= len(level) {
			return types.MenuItemAddress{}
		}
		captions = append(captions, level[i].Caption)
		level = level[i].Items
	}
	target := MenuNodeAt(roots, path)
	for l := 1; l <= len(captions); l++ {
		addr := types.MenuItemAddress{}
		for _, c := range captions[len(captions)-l:] {
			addr.Segments = append(addr.Segments, types.MenuItemAddressSegment{Caption: c})
		}
		if m, err := resolveMenuMatches(addr, roots); err == nil && len(m) == 1 && m[0].node == target {
			return addr
		}
	}
	caption := captions[len(captions)-1]
	if len(path) == 1 {
		n := 0
		for _, e := range preorder(roots) {
			if e.node.Caption == caption {
				n++
			}
			if e.node == target {
				break
			}
		}
		return types.MenuItemAddress{Segments: []types.MenuItemAddressSegment{{Caption: caption, Ordinal: n}}}
	}
	parent := path.Parent()
	addr := ShortestMenuItemAddress(roots, parent)
	n := 0
	for i, sib := range MenuNodeAt(roots, parent).Items {
		if sib.Caption == caption {
			n++
		}
		if i == path.Index() {
			break
		}
	}
	addr.Segments = append(addr.Segments, types.MenuItemAddressSegment{Caption: caption, Ordinal: n})
	return addr
}
