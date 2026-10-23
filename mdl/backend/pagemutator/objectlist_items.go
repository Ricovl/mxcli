// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A pluggable widget's object-list item — an Accordion group, a Pop-up menu
// item — is a CustomWidgets$WidgetObject and stores no Name, so `insert into
// grp1` never resolved: the name a script wrote on `group grp1` was parsed and
// dropped (ako/mxcli#1326, mendixlabs/mxcli#1453). It is addressed the way a
// navigation list item is (#1388), by what it holds, what it shows or where it
// is: `acc1 item(btnIn)`, `acc1 item('One')`, `acc1 item@1`. A bare item address
// means the item's BODY slot (types.BodyItemSlotKey, the slot CREATE routes loose
// children to); `acc1 item@1.headercontent` names another slot, as
// `grid.controlbar` names a widget's. PROPOSAL_alter_object_list_item_address.md.

// objectListItemInfo is what a resolved item carries beyond its position: the
// widget that owns the list, and the item's widget slots in schema order.
type objectListItemInfo struct {
	owner    bson.D
	listKey  string
	slotKeys []string // Widgets-typed item property keys, schema order
}

// itemList is one object-list property of a pluggable widget.
type itemList struct {
	key      string
	typeID   string
	value    bson.D // the property's WidgetValue, holding Objects
	slotKeys []string
}

// objectItemLists returns the widget's object-list properties: its schema's
// `Object`-typed, IsList properties that are stored with an Objects list.
func objectItemLists(widget bson.D) []itemList {
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(widget, "Type"), "ObjectType")
	var lists []itemList
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		vt := bsonnav.DGetDoc(ptDoc, "ValueType")
		if bsonnav.DGetString(vt, "Type") != "Object" || bsonnav.DGet(vt, "IsList") != true {
			continue
		}
		l := itemList{
			key:    bsonnav.DGetString(ptDoc, "PropertyKey"),
			typeID: bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID")),
		}
		for _, ipt := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(vt, "ObjectType"), "PropertyTypes")) {
			iptDoc, ok := ipt.(bson.D)
			if ok && bsonnav.DGetString(bsonnav.DGetDoc(iptDoc, "ValueType"), "Type") == "Widgets" {
				l.slotKeys = append(l.slotKeys, bsonnav.DGetString(iptDoc, "PropertyKey"))
			}
		}
		for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(widget, "Object"), "Properties")) {
			propDoc, ok := prop.(bson.D)
			if ok && bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer")) == l.typeID {
				l.value = bsonnav.DGetDoc(propDoc, "Value")
			}
		}
		if l.value != nil && hasKey(l.value, "Objects") {
			lists = append(lists, l)
		}
	}
	return lists
}

// resolveObjectListItem finds the one item of a pluggable widget a selector
// addresses, with the same matching, @n and messages as a navigation list.
func resolveObjectListItem(owner *bsonWidgetResult, sel backend.NavItemSelector) (*bsonWidgetResult, error) {
	lists := objectItemLists(owner.widget)
	switch {
	case len(lists) == 0:
		return nil, fmt.Errorf("%s: %q (%s) has no list of items — `item(…)` and `item@n` address a navigation "+
			"list's items or a pluggable widget's object-list items (an Accordion's groups)",
			sel, sel.List, widgetTypeName(owner.widget))
	case len(lists) > 1:
		var keys []string
		for _, l := range lists {
			keys = append(keys, l.key)
		}
		return nil, fmt.Errorf("%s: %q has more than one list of items (%s), so `item` does not say which — "+
			"not supported yet", sel, sel.List, strings.Join(keys, ", "))
	}
	l := lists[0]
	keys, kinds := buildColumnPropKeyMap(owner.widget, l.typeID)
	items := bsonnav.DGetArrayElements(bsonnav.DGet(l.value, "Objects"))
	var matches []int
	for i, el := range items {
		itemDoc, ok := el.(bson.D)
		if !ok {
			continue
		}
		switch {
		case sel.Widget != "":
			if !itemHoldsWidget(itemDoc, sel.Widget) {
				continue
			}
		case sel.Text != "":
			if !containsString(objectItemTexts(itemDoc, kinds), sel.Text) {
				continue
			}
		}
		matches = append(matches, i)
	}
	pick, err := pickItem(sel, matches, len(items), func() string { return objectItemList(sel.List, items, kinds) })
	if err != nil {
		return nil, err
	}
	return &bsonWidgetResult{
		widget:       items[pick].(bson.D),
		parentArr:    items,
		parentKey:    "Objects",
		parentDoc:    l.value,
		index:        pick,
		colPropKeys:  keys,
		colPropKinds: kinds,
		item:         &objectListItemInfo{owner: owner.widget, listKey: l.key, slotKeys: l.slotKeys},
	}, nil
}

// pickItem applies a selector's @n to the indexes that match it. listing names
// every item's addresses, for a message that says what would hit.
func pickItem(sel backend.NavItemSelector, matches []int, total int, listing func() string) (int, error) {
	switch {
	case sel.Ordinal > 0 && sel.Ordinal <= len(matches):
		return matches[sel.Ordinal-1], nil
	case sel.Ordinal > 0:
		return -1, fmt.Errorf("%s: %s, so there is no @%d. %s", sel, countMatches(len(matches), sel), sel.Ordinal, listing())
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) == 0:
		return -1, fmt.Errorf("%s: no item matches. %s", sel, listing())
	default:
		return -1, fmt.Errorf("%s is ambiguous: %s — add @n to choose one (@1 … @%d). %s",
			sel, countMatches(len(matches), sel), len(matches), listing())
	}
}

// itemHoldsWidget reports whether any slot of the item holds the named widget,
// at any depth.
func itemHoldsWidget(itemDoc bson.D, name string) bool {
	for _, val := range objectListItemPropValues(itemDoc) {
		if findInWidgetArray(val, "Widgets", name) != nil {
			return true
		}
	}
	return false
}

// objectListItemPropValues returns the Value document of every property of an
// item.
func objectListItemPropValues(itemDoc bson.D) []bson.D {
	var out []bson.D
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(itemDoc, "Properties")) {
		if pd, ok := p.(bson.D); ok {
			if v := bsonnav.DGetDoc(pd, "Value"); v != nil {
				out = append(out, v)
			}
		}
	}
	return out
}

// objectItemTexts returns the texts an item shows: its TextTemplate properties in
// every language, and its String properties — an Accordion group's headerText.
func objectItemTexts(itemDoc bson.D, kinds map[string]string) []string {
	var out []string
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(itemDoc, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		val := bsonnav.DGetDoc(pd, "Value")
		switch kinds[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))] {
		case "TextTemplate":
			tmpl := bsonnav.DGetDoc(bsonnav.DGetDoc(val, "TextTemplate"), "Template")
			for _, it := range bsonnav.DGetArrayElements(bsonnav.DGet(tmpl, "Items")) {
				if d, ok := it.(bson.D); ok {
					if t := bsonnav.DGetString(d, "Text"); t != "" && !containsString(out, t) {
						out = append(out, t)
					}
				}
			}
		case "String":
			if t := bsonnav.DGetString(val, "PrimitiveValue"); t != "" && !containsString(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// objectItemList lists each item's addresses, so a miss shows what would hit.
func objectItemList(list string, items []any, kinds map[string]string) string {
	if len(items) == 0 {
		return fmt.Sprintf("%q has no items", list)
	}
	var parts []string
	for i, el := range items {
		itemDoc, _ := el.(bson.D)
		addrs := []string{fmt.Sprintf("item@%d", i+1)}
		if texts := objectItemTexts(itemDoc, kinds); len(texts) > 0 {
			addrs = append(addrs, "item('"+strings.ReplaceAll(texts[0], "'", "''")+"')")
		}
		parts = append(parts, strings.Join(addrs, " = "))
	}
	return fmt.Sprintf("Items of %s: %s", list, strings.Join(parts, "; "))
}

// itemSlots returns a resolved item's widget slots, keyed as an item slot is
// spelled in MDL: the property key, lower-cased (`headercontent`), the keyword
// CREATE's item builder routes. The first returned slot is not special; the
// body is chosen with types.BodyItemSlotKey.
func itemSlots(result *bsonWidgetResult) []pluggableSlot {
	var out []pluggableSlot
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(result.widget, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		tp := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))
		if result.colPropKinds[tp] != "Widgets" {
			continue
		}
		val := bsonnav.DGetDoc(pd, "Value")
		if val == nil || !hasKey(val, "Widgets") {
			continue
		}
		out = append(out, pluggableSlot{owner: result, keyword: strings.ToLower(result.colPropKeys[tp]), value: val})
	}
	return out
}

// itemBodySlot returns the item's body slot.
func itemBodySlot(result *bsonWidgetResult) (pluggableSlot, bool) {
	body := strings.ToLower(types.BodyItemSlotKey(result.item.slotKeys))
	for _, s := range itemSlots(result) {
		if s.keyword == body {
			return s, true
		}
	}
	return pluggableSlot{}, false
}

// insertIntoObjectListItem handles a bare item target: INTO fills its body;
// BEFORE/AFTER would add a new item, which ALTER does not do yet.
func (m *Mutator) insertIntoObjectListItem(result *bsonWidgetResult, ref string, position backend.InsertPosition, widgets []pages.Widget) error {
	if !strings.EqualFold(string(position), "into") {
		return fmt.Errorf("cannot insert %s %q: adding a new %s item is not supported by ALTER yet — "+
			"to put widgets in this item, write `insert into %s { … }`", strings.ToLower(string(position)),
			ref, result.item.listKey, ref)
	}
	body, ok := itemBodySlot(result)
	if !ok {
		return fmt.Errorf("%q has no widget slot to insert into", ref)
	}
	newBson, err := m.serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}
	body.set(append(append([]any{}, body.widgets()...), newBson...))
	return nil
}

// objectListItemHint names the item addresses of a widget that has an item
// list, for the refusal of `insert into <widget>` on the widget itself.
func objectListItemHint(owner *bsonWidgetResult, name string) string {
	lists := objectItemLists(owner.widget)
	if len(lists) != 1 || len(lists[0].slotKeys) == 0 {
		return ""
	}
	return fmt.Sprintf("its widgets live in its %s: `insert into %s item@1 { … }` fills the first one, "+
		"`%s item('<text>')` addresses one by its text", lists[0].key, name, name)
}

// objectListItemMissHint names every object-list item address on the page, for
// a widget lookup that missed. CREATE accepts a name on an item — `group grp1
// (…)` — and drops it, since the item stores none, so `insert into grp1 { … }`
// is the natural next statement and can never resolve (ako/mxcli#1326). The
// miss said only how DataGrid 2 columns are addressed, sending the author to
// the grid. Listed: each widget whose one object list `item` addresses; not a
// DataGrid 2's columns, which the column hint lists by their derived names.
func objectListItemMissHint(page bson.D) string {
	var addrs []string
	collectObjectListItemAddresses(page, &addrs)
	if len(addrs) == 0 {
		return ""
	}
	return "An object-list item (an Accordion group, a pop-up menu item) stores no name, so a name " +
		"written on one in CREATE is not kept — address it on its widget: " + strings.Join(addrs, "; ")
}

func collectObjectListItemAddresses(node any, out *[]string) {
	switch v := node.(type) {
	case bson.D:
		*out = append(*out, objectListItemAddresses(v)...)
		for _, e := range v {
			collectObjectListItemAddresses(e.Value, out)
		}
	case bson.A:
		for _, e := range v {
			collectObjectListItemAddresses(e, out)
		}
	}
}

// objectListItemAddresses spells each item of a pluggable widget's one object
// list as the author types it, `acc1 item@1 = acc1 item('One')`; nil for any
// other node.
func objectListItemAddresses(w bson.D) []string {
	name := bsonnav.DGetString(w, "Name")
	if name == "" || !strings.Contains(widgetTypeName(w), "CustomWidget") {
		return nil
	}
	lists := objectItemLists(w)
	if len(lists) != 1 || lists[0].key == "columns" {
		return nil
	}
	_, kinds := buildColumnPropKeyMap(w, lists[0].typeID)
	var out []string
	for i, el := range bsonnav.DGetArrayElements(bsonnav.DGet(lists[0].value, "Objects")) {
		itemDoc, _ := el.(bson.D)
		addr := fmt.Sprintf("%s item@%d", name, i+1)
		if texts := objectItemTexts(itemDoc, kinds); len(texts) > 0 {
			addr += fmt.Sprintf(" = %s item('%s')", name, strings.ReplaceAll(texts[0], "'", "''"))
		}
		out = append(out, addr)
	}
	return out
}

// ObjectListItemValues reports a stored item's values for visibility rules.
// ok is false when itemRef does not resolve to an object-list item.
func (m *Mutator) ObjectListItemValues(itemRef string) (backend.ObjectListItemValues, bool) {
	result := m.widgetFinder(m.rawData, itemRef)
	if result == nil || result.item == nil {
		return backend.ObjectListItemValues{}, false
	}
	ownerKeys := buildPropKeyMap(result.item.owner)
	return backend.ObjectListItemValues{
		WidgetID: pluggableWidgetID(result.item.owner),
		ListKey:  result.item.listKey,
		BodySlot: types.BodyItemSlotKey(result.item.slotKeys),
		Item:     storedScalarValues(result.widget, result.colPropKeys),
		Widget:   storedScalarValues(bsonnav.DGetDoc(result.item.owner, "Object"), ownerKeys),
	}, true
}

// storedScalarValues maps each property of a WidgetObject (keys: TypePointer
// id -> property key) to its stored PrimitiveValue, or its Expression when it
// has no primitive one.
func storedScalarValues(obj bson.D, keys map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		key := keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))]
		val := bsonnav.DGetDoc(pd, "Value")
		if key == "" || val == nil {
			continue
		}
		v := bsonnav.DGetString(val, "PrimitiveValue")
		if v == "" {
			v = bsonnav.DGetString(val, "Expression")
		}
		if v != "" {
			out[strings.ToLower(key)] = v
		}
	}
	return out
}

// newTemplate returns an empty client template from the engine, or nil when
// the engine has none to give.
func (m *Mutator) newTemplate() bson.D {
	if m.deps == nil {
		return nil
	}
	return m.deps.SerializeClientTemplate()
}

// ensureItemTemplate gives an item's text-template property a template when
// it is stored null, so `set (headerText: 'Two') on acc1 item@2` can write a
// group CREATE made in custom mode. Whether the text may be set at all — it
// may be hidden — is the reset pass's to say afterwards, with the refusal it
// gives a hidden property the statement named.
func (m *Mutator) ensureItemTemplate(result *bsonWidgetResult, prop string) {
	key := resolveColumnPropertyKey(prop, result.colPropKeys)
	if key == "" || keyKind(result.colPropKeys, result.colPropKinds, key) != "TextTemplate" ||
		templateFilled(result.widget, result.colPropKeys, key) {
		return
	}
	if tmpl := m.newTemplate(); tmpl != nil {
		if val := itemPropValue(result.widget, result.colPropKeys, key); val != nil {
			bsonnav.DSet(val, "TextTemplate", tmpl)
		}
	}
}
