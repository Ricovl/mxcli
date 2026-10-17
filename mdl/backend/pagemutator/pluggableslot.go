// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A pluggable widget's widgets-typed property — a Data grid 2's control bar
// (filtersPlaceholder), a Gallery's template or filter — is a SLOT, not a
// widget: it stores no Name, and describe prints it as a keyword block
// (`controlbar { … }`). So once its last widget was dropped nothing could
// reach it again: `insert into controlBar1` was "not found" and `insert into
// dataGrid21` "not a simple container" (mendixlabs/mxcli#957).
//
// It is addressed the way a data view's footer is — positionally, by owner and
// slot: `dataGrid21.controlbar`, spelled with the keyword describe prints
// (types.WidgetSlotKeyword) or the property key itself. INSERT INTO appends,
// REPLACE swaps the whole content, DROP empties it; the slot itself stays, as
// the property is mandatory. BEFORE/AFTER a slot mean nothing and are refused.

// pluggableSlot is one widgets-typed property of a pluggable widget's Object.
type pluggableSlot struct {
	owner   *bsonWidgetResult
	keyword string // lower-case MDL keyword, or the property key
	alias   bool   // keyword is the property key, not describe's spelling
	value   bson.D // the property's WidgetValue, holding Widgets
}

// pluggableSlots lists the widget's own widgets-typed properties (not those of
// its object-list items — an Accordion group's content is not the Accordion's).
func pluggableSlots(owner *bsonWidgetResult) []pluggableSlot {
	if bsonnav.DGetString(owner.widget, "$Type") != "CustomWidgets$CustomWidget" {
		return nil
	}
	keys := buildPropKeyMap(owner.widget)
	kinds := buildPropKindMap(owner.widget)
	widgetID := pluggableWidgetID(owner.widget)
	var out []pluggableSlot
	for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(owner.widget, "Object"), "Properties")) {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		tp := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		if kinds[tp] != "Widgets" || keys[tp] == "" {
			continue
		}
		value := bsonnav.DGetDoc(propDoc, "Value")
		if value == nil || !hasKey(value, "Widgets") {
			continue
		}
		keyword := strings.ToLower(types.WidgetSlotKeyword(widgetID, keys[tp]))
		out = append(out, pluggableSlot{owner: owner, keyword: keyword, value: value})
		if !strings.EqualFold(keyword, keys[tp]) {
			// Also answer to the property key, the name the widget's own
			// definition uses for it.
			out = append(out, pluggableSlot{owner: owner, keyword: strings.ToLower(keys[tp]), alias: true, value: value})
		}
	}
	return out
}

// findPluggableSlot resolves `<owner>.<slot>`. ok is false when member names no
// slot of a pluggable owner, so the caller falls through to the other dotted
// forms. A member that also resolves as a Data grid 2 column is refused rather
// than guessed.
func (m *Mutator) findPluggableSlot(owner, member string) (slot pluggableSlot, ok bool, err error) {
	result := m.widgetFinder(m.rawData, owner)
	if result == nil {
		return pluggableSlot{}, false, nil
	}
	for _, s := range pluggableSlots(result) {
		if !strings.EqualFold(s.keyword, member) {
			continue
		}
		if _, colErr := findBsonColumn(m.rawData, owner, member, m.widgetFinder); colErr == nil {
			return s, true, fmt.Errorf("%s.%s is ambiguous: it names both the %s slot of %q and one of its "+
				"columns — address the column by its other name (attribute or caption)", owner, member, s.keyword, owner)
		}
		return s, true, nil
	}
	return pluggableSlot{}, false, nil
}

// widgets is the slot's current content, without the list marker.
func (s pluggableSlot) widgets() []any {
	return bsonnav.DGetArrayElements(bsonnav.DGet(s.value, "Widgets"))
}

// set stores the slot's new content, keeping its stored list marker. The
// Widgets key exists (pluggableSlots requires it), so DSet writes in place.
func (s pluggableSlot) set(widgets []any) {
	out := bson.A{int32(2)}
	if raw := bsonnav.ToBsonA(bsonnav.DGet(s.value, "Widgets")); len(raw) > 0 && isListMarker(raw[0]) {
		out = bson.A{raw[0]}
	}
	bsonnav.DSet(s.value, "Widgets", append(out, widgets...))
}

// insertIntoPluggableSlot handles `insert into <widget>.<slot> { … }`.
func (m *Mutator) insertIntoPluggableSlot(owner, member string, position backend.InsertPosition, widgets []pages.Widget) (bool, error) {
	slot, ok, err := m.findPluggableSlot(owner, member)
	if !ok || err != nil {
		return ok, err
	}
	if !strings.EqualFold(string(position), "into") {
		return true, fmt.Errorf("%s.%s is a slot of the widget, not a widget, so it can only be an INSERT INTO target; "+
			"to place a widget relative to another, name that widget: `insert %s <widgetName> { … }`",
			owner, member, strings.ToLower(string(position)))
	}
	newBson, err := m.serializeWidgets(widgets)
	if err != nil {
		return true, fmt.Errorf("serialize widgets: %w", err)
	}
	slot.set(append(append([]any{}, slot.widgets()...), newBson...))
	return true, nil
}

// replacePluggableSlot handles `replace <widget>.<slot> with { … }`.
func (m *Mutator) replacePluggableSlot(owner, member string, widgets []pages.Widget) (bool, error) {
	slot, ok, err := m.findPluggableSlot(owner, member)
	if !ok || err != nil {
		return ok, err
	}
	newBson, err := m.serializeWidgets(widgets)
	if err != nil {
		return true, fmt.Errorf("serialize widgets: %w", err)
	}
	slot.set(newBson)
	return true, nil
}

// dropPluggableSlot handles `drop <widget>.<slot>`: the slot is emptied.
func (m *Mutator) dropPluggableSlot(owner, member string) (bool, error) {
	slot, ok, err := m.findPluggableSlot(owner, member)
	if !ok || err != nil {
		return ok, err
	}
	slot.set(nil)
	return true, nil
}

// pluggableSlotHint names the slot addresses a pluggable widget has, for the
// refusal of `insert into <widget>` on the widget itself.
func pluggableSlotHint(owner *bsonWidgetResult, name string) string {
	var addrs []string
	for _, s := range pluggableSlots(owner) {
		if !s.alias {
			addrs = append(addrs, "`insert into "+name+"."+s.keyword+" { … }`")
		}
	}
	if len(addrs) == 0 {
		return ""
	}
	sort.Strings(addrs)
	return strings.Join(addrs, ", ")
}
