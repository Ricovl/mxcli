// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// navListItemType is a navigation list's entry. It lives in the list's Items,
// not in a widget list, and Studio Pro stores no Name on it.
const navListItemType = "Forms$NavigationListItem"

// isNavigationList reports whether a stored widget is a navigation list, whose
// entries — and the widgets inside them — sit under Items rather than Widgets.
func isNavigationList(widget bson.D) bool {
	switch widgetTypeName(widget) {
	case "Forms$NavigationList", "Pages$NavigationList":
		return true
	}
	return false
}

// InsertNavigationListItems adds items to an existing navigation list
// (mendixlabs/mxcli#1388).
//
// An item lives in the list's Items and is not a widget — the same reason tab
// pages and list view templates have their own paths. INSERT INTO <list>
// appends. An item has no Name (Studio Pro writes none, and mxcli stopped
// writing one in #1373), so BEFORE/AFTER has something to name only for an item
// an older mxcli stored with a Name; a widget inside an item is that item's
// content, not its sibling, and is refused with the INTO form instead of
// guessing that "after t1" meant "after the item holding t1".
//
// The items are serialized by wrapping them in a throwaway navigation list and
// taking its Items back out, so they get exactly the shape CREATE writes.
func (m *Mutator) InsertNavigationListItems(targetRef string, position backend.InsertPosition, items []*pages.NavigationListItem) error {
	result := m.widgetFinder(m.rawData, targetRef)
	if result == nil {
		return m.widgetNotFoundError(targetRef)
	}
	into := strings.EqualFold(string(position), "into")
	isItem := widgetTypeName(result.widget) == navListItemType
	switch {
	case into && !isNavigationList(result.widget):
		return fmt.Errorf("cannot insert a navigation list item into %q (%s): an item can only be added to a navigation list — "+
			"use `insert into <navigationlist> { item … }`", targetRef, widgetTypeName(result.widget))
	case !into && !isItem:
		hint := "use `insert into <navigationlist> { item … }` to append it"
		if list := m.enclosingNavigationList(targetRef); list != "" {
			hint = fmt.Sprintf("use `insert into %s { item … }` to append it", list)
		}
		return fmt.Errorf("cannot insert a navigation list item %s %q (%s): an item's siblings are items, and items have no name to "+
			"place one next to — %s, or `describe` and `replace <navigationlist>` to reorder",
			strings.ToLower(string(position)), targetRef, widgetTypeName(result.widget), hint)
	}

	newDocs, err := m.serializeNavigationListItems(items)
	if err != nil {
		return err
	}

	if !into {
		insertIdx := result.index
		if strings.EqualFold(string(position), "after") {
			insertIdx++
		}
		out := make([]any, 0, len(result.parentArr)+len(newDocs))
		out = append(out, result.parentArr[:insertIdx]...)
		out = append(out, newDocs...)
		out = append(out, result.parentArr[insertIdx:]...)
		bsonnav.DSetArray(result.parentDoc, result.parentKey, out)
		return nil
	}

	list := result.widget
	stored := bsonnav.ToBsonA(bsonnav.DGet(list, "Items"))
	var out bson.A
	if len(stored) == 0 {
		out = append(out, int32(2)) // Items' list marker, as the codec registers it
	} else {
		out = append(out, stored...)
	}
	out = append(out, newDocs...)
	if !bsonnav.DSet(list, "Items", out) {
		list = append(list, bson.E{Key: "Items", Value: out})
	}
	if result.parentArr == nil || result.index < 0 || result.index >= len(result.parentArr) {
		return fmt.Errorf("cannot add items to %q: its parent slot could not be located", targetRef)
	}
	result.parentArr[result.index] = list
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return nil
}

// enclosingNavigationList names the navigation list a widget sits in, or "".
func (m *Mutator) enclosingNavigationList(widgetRef string) string {
	var found string
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case bson.D:
			if found != "" {
				return
			}
			if isNavigationList(v) {
				for _, it := range bsonnav.DGetArrayElements(bsonnav.DGet(v, "Items")) {
					if itDoc, ok := it.(bson.D); ok && findInWidgetArray(itDoc, "Widgets", widgetRef) != nil {
						found = bsonnav.DGetString(v, "Name")
						return
					}
				}
			}
			for _, e := range v {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(m.rawData)
	return found
}

// serializeNavigationListItems returns the stored form of each item, without a
// list marker, by serializing them inside a wrapper navigation list.
func (m *Mutator) serializeNavigationListItems(items []*pages.NavigationListItem) ([]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no navigation list items to insert")
	}
	wrapper := &pages.NavigationList{Items: items}
	wrapper.TypeName = "Forms$NavigationList"
	doc := m.deps.SerializeWidget(wrapper)
	var out []any
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(doc, "Items")) {
		if d, ok := el.(bson.D); ok {
			out = append(out, d)
		}
	}
	if len(out) != len(items) {
		return nil, fmt.Errorf("serialize navigation list items: got %d stored items for %d inserted", len(out), len(items))
	}
	return out, nil
}
