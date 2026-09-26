// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ResolveAlterTarget is the page family's backend.AlterTargetResolver (page,
// snippet and layout alike — one widget tree, one address syntax).
//
// A page element is addressed by name: `btnSave`, `grid.Column`, or a scroll
// container's region, `layoutContainer.top`. The lookups are the ones the
// operations themselves make, so resolution cannot accept a target the
// operation then misses, and a miss reports what the operation would have
// reported.
func (m *Mutator) ResolveAlterTarget(t backend.AlterTarget) (backend.AlterTargetMatch, error) {
	if err := backend.CheckPageAlterTarget(t); err != nil {
		return backend.AlterTargetMatch{}, err
	}
	name := strings.Join(t.Path, ".")
	if len(t.Path) == 2 {
		container, member := t.Path[0], t.Path[1]
		if kind, ok, err := m.resolveScrollRegion(container, member); ok {
			return backend.AlterTargetMatch{Kind: kind, Name: name}, err
		}
		if _, err := findBsonColumn(m.rawData, container, member, m.widgetFinder); err != nil {
			return backend.AlterTargetMatch{}, err
		}
		return backend.AlterTargetMatch{Kind: "column", Name: name}, nil
	}

	result := m.widgetFinder(m.rawData, name)
	if result == nil {
		return backend.AlterTargetMatch{}, m.widgetNotFoundError(name)
	}
	if bsonnav.DGetString(result.widget, "$Type") != objectListItemType && len(result.colPropKeys) == 0 {
		// A real widget. Whether a same-named column elsewhere on the page makes
		// the address ambiguous is each operation's call, as it was before the
		// resolver existed: drop/replace/insert refuse it, set goes to the
		// widget. Refusing here would be a new rejection (ADR-0011).
		return backend.AlterTargetMatch{Kind: "widget", Name: name}, nil
	}
	if n := m.columnMatchCount(name); n > 1 {
		return backend.AlterTargetMatch{}, columnAmbiguityError(name, n)
	}
	return backend.AlterTargetMatch{Kind: "column", Name: name}, nil
}

// resolveScrollRegion answers a `container.slot` address when the container is a
// scroll container, with the same refusals insertIntoScrollRegion makes. ok is
// false when the container is not one, so the caller tries a grid column — the
// dotted form serves both, and what the named widget IS decides which.
func (m *Mutator) resolveScrollRegion(container, slot string) (kind string, ok bool, err error) {
	result := m.widgetFinder(m.rawData, container)
	if result == nil {
		return "", false, nil
	}
	if t := bsonnav.DGetString(result.widget, "$Type"); t != "Forms$ScrollContainer" && t != "Pages$ScrollContainer" {
		return "", false, nil
	}
	key, known := scrollRegionKey(slot)
	if !known {
		return "", true, fmt.Errorf("scroll container %q has no region %q (want top, right, bottom, left or center)", container, slot)
	}
	if bsonnav.DGetDoc(result.widget, key) == nil {
		return "", true, fmt.Errorf("scroll container %q has no %s region; "+
			"add one with `create or replace layout` — an empty slot has no stored document to insert into",
			container, strings.ToLower(slot))
	}
	return "region", true, nil
}
