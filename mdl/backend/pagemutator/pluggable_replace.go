// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"bytes"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// REPLACE of a pluggable widget by one of the same widget type keeps what the
// statement does not state (mendixlabs/mxcli#1247) — the passthrough principle
// of ako/mxcli#830, at property granularity.
//
// A pluggable widget is rebuilt from its package TEMPLATE plus what MDL maps
// onto it, so a REPLACE wrote every property MDL has no word for at the
// template's value: measured on TestApp's Studio Pro-authored
// Rules.BusinessRule_NewEdit, adding `sort by Name asc` to comboBox1's data
// source also turned its stored Editable "Never" into "Always" and an
// expression property's PrimitiveValue "" into "false"; the reporter's
// translated placeholder (emptyOptionText) and readOnlyStyle were reset the
// same way. exec reported success, mx check stayed green.
//
// "What the statement states" is measured, not listed: the executor builds the
// stored widget as describe prints it (the BASELINE) next to the replacement.
// A property that comes out the same from both is one the statement did not
// change — the template filled it either way — so the stored value stays. A
// property that differs is the statement's, and its new value is grafted into
// the stored widget, its type pointers re-aimed at the stored Type by property
// key. The stored Type is kept whole, as #830 keeps it.
//
// Anything that cannot be matched up — a different widget package, a stated
// property the stored schema does not declare, a type pointer with no
// counterpart — leaves the plain replacement, which is what REPLACE did before.

// ReplacePluggableKeepingUnstated replaces the pluggable widget widgetRef names
// with replacement, keeping every stored property on which replacement and
// baseline agree. handled is false when the merge does not apply; the caller
// then replaces as usual.
func (m *Mutator) ReplacePluggableKeepingUnstated(widgetRef string, replacement, baseline pages.Widget) (handled bool, err error) {
	if replacement == nil || baseline == nil {
		return false, nil
	}
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil || len(result.colPropKeys) > 0 {
		return false, nil
	}
	if bsonnav.DGetString(result.widget, "$Type") != "CustomWidgets$CustomWidget" {
		return false, nil
	}
	newDoc := m.deps.SerializeWidget(replacement)
	baseDoc := m.deps.SerializeWidget(baseline)
	merged, ok := mergeUnstatedPluggable(result.widget, newDoc, baseDoc)
	if !ok {
		return false, nil
	}
	result.parentArr[result.index] = merged
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return true, nil
}

// mergeUnstatedPluggable returns stored with the replacement's stated changes
// applied, or false when the three cannot be lined up.
func mergeUnstatedPluggable(stored, replacement, baseline bson.D) (bson.D, bool) {
	if stored == nil || replacement == nil || baseline == nil {
		return nil, false
	}
	id := pluggableWidgetID(stored)
	if id == "" || pluggableWidgetID(replacement) != id || pluggableWidgetID(baseline) != id {
		return nil, false
	}
	out := cloneDoc(stored)

	// The widget's own fields (name, label, editability, appearance, …).
	for _, e := range replacement {
		switch e.Key {
		case "$ID", "$Type", "Type", "Object":
			continue
		}
		if sameIgnoringIdentity(e.Value, bsonnav.DGet(baseline, e.Key)) {
			if _, has := docField(out, e.Key); has {
				continue
			}
		}
		if !bsonnav.DSet(out, e.Key, e.Value) {
			out = append(out, bson.E{Key: e.Key, Value: e.Value})
		}
	}

	// The widget's properties, by key.
	newProps := propertiesByKey(replacement)
	baseProps := propertiesByKey(baseline)
	storedProps := propertiesByKey(out)
	if newProps == nil || storedProps == nil {
		return nil, false
	}
	newPaths := typeIDPaths(bsonnav.DGetDoc(replacement, "Type"))
	storedIDs := invertPaths(typeIDPaths(bsonnav.DGetDoc(out, "Type")))
	for key, np := range newProps {
		newValue := bsonnav.DGet(np, "Value")
		if bp, ok := baseProps[key]; ok && sameIgnoringIdentity(newValue, bsonnav.DGet(bp, "Value")) {
			continue // not the statement's: the stored value stays
		}
		sp, ok := storedProps[key]
		if !ok {
			return nil, false
		}
		remapped, ok := remapTypePointers(newValue, newPaths, storedIDs)
		if !ok {
			return nil, false
		}
		bsonnav.DSet(sp, "Value", remapped)
	}
	return out, true
}

func pluggableWidgetID(w bson.D) string {
	return bsonnav.DGetString(bsonnav.DGetDoc(w, "Type"), "WidgetId")
}

func docField(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// propertiesByKey indexes a pluggable widget's Object properties by their
// PropertyKey, read through the widget's own Type. The returned documents are
// the ones inside w, so setting a field on one changes w.
func propertiesByKey(w bson.D) map[string]bson.D {
	keyByID := map[string]string{}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(bsonnav.DGetDoc(w, "Type"), "ObjectType"), "PropertyTypes")) {
		if d, ok := pt.(bson.D); ok {
			keyByID[idKey(bsonnav.DGet(d, "$ID"))] = bsonnav.DGetString(d, "PropertyKey")
		}
	}
	obj := bsonnav.DGetDoc(w, "Object")
	if obj == nil || len(keyByID) == 0 {
		return nil
	}
	out := map[string]bson.D{}
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		d, ok := p.(bson.D)
		if !ok {
			continue
		}
		if key := keyByID[idKey(bsonnav.DGet(d, "TypePointer"))]; key != "" {
			out[key] = d
		}
	}
	return out
}

// typeIDPaths maps every element ID in a widget Type to a path that names it
// independently of IDs: the chain of field names, with list elements named by
// their PropertyKey (or Key, or position).
func typeIDPaths(typ bson.D) map[string]string {
	out := map[string]string{}
	var walk func(d bson.D, path string)
	walk = func(d bson.D, path string) {
		if id := idKey(bsonnav.DGet(d, "$ID")); id != "" {
			out[id] = path
		}
		for _, e := range d {
			switch v := e.Value.(type) {
			case bson.D:
				walk(v, path+"/"+e.Key)
			case bson.A:
				for i, el := range v {
					ed, ok := el.(bson.D)
					if !ok {
						continue
					}
					name := bsonnav.DGetString(ed, "PropertyKey")
					if name == "" {
						name = bsonnav.DGetString(ed, "Key")
					}
					if name == "" {
						name = "#" + strconv.Itoa(i)
					}
					walk(ed, path+"/"+e.Key+"["+name+"]")
				}
			}
		}
	}
	walk(typ, "")
	return out
}

// invertPaths turns ID→path into path→stored ID value.
func invertPaths(byID map[string]string) map[string]string {
	out := make(map[string]string, len(byID))
	for id, p := range byID {
		out[p] = id
	}
	return out
}

// remapTypePointers deep-copies v with every TypePointer re-aimed from the
// replacement's Type to the stored one, by path. false when one has no
// counterpart.
func remapTypePointers(v any, newPaths, storedIDs map[string]string) (any, bool) {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, 0, len(x))
		for _, e := range x {
			if e.Key == "TypePointer" {
				if id := idKey(e.Value); id != "" {
					path, ok := newPaths[id]
					if !ok {
						return nil, false
					}
					stored, ok := storedIDs[path]
					if !ok {
						return nil, false
					}
					out = append(out, bson.E{Key: e.Key, Value: idValue(stored, e.Value)})
					continue
				}
			}
			nv, ok := remapTypePointers(e.Value, newPaths, storedIDs)
			if !ok {
				return nil, false
			}
			out = append(out, bson.E{Key: e.Key, Value: nv})
		}
		return out, true
	case bson.A:
		out := make(bson.A, 0, len(x))
		for _, el := range x {
			nv, ok := remapTypePointers(el, newPaths, storedIDs)
			if !ok {
				return nil, false
			}
			out = append(out, nv)
		}
		return out, true
	}
	return v, true
}

// idKey renders an element ID — binary as stored, or a string — as a map key.
func idKey(v any) string {
	switch x := v.(type) {
	case primitive.Binary:
		return "b:" + string(x.Data)
	case []byte:
		return "b:" + string(x)
	case string:
		return "s:" + x
	}
	return ""
}

// idValue turns a key from idKey back into the stored representation, in the
// shape like (the pointer it replaces).
func idValue(key string, like any) any {
	switch key[:2] {
	case "b:":
		data := []byte(key[2:])
		if b, ok := like.(primitive.Binary); ok {
			return primitive.Binary{Subtype: b.Subtype, Data: data}
		}
		return primitive.Binary{Data: data}
	default:
		return key[2:]
	}
}

// sameIgnoringIdentity compares two values with every $ID and TypePointer
// left out: two builds of the same widget mint different IDs for the same
// content.
func sameIgnoringIdentity(a, b any) bool {
	ea, errA := bson.Marshal(bson.D{{Key: "v", Value: stripIdentity(a)}})
	eb, errB := bson.Marshal(bson.D{{Key: "v", Value: stripIdentity(b)}})
	return errA == nil && errB == nil && bytes.Equal(ea, eb)
}

func stripIdentity(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, 0, len(x))
		for _, e := range x {
			if e.Key == "$ID" || e.Key == "TypePointer" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: stripIdentity(e.Value)})
		}
		return out
	case bson.A:
		out := make(bson.A, 0, len(x))
		for _, el := range x {
			out = append(out, stripIdentity(el))
		}
		return out
	}
	return v
}

// cloneDoc deep-copies a document through its encoding.
func cloneDoc(d bson.D) bson.D {
	raw, err := bson.Marshal(d)
	if err != nil {
		return d
	}
	var out bson.D
	if err := bson.Unmarshal(raw, &out); err != nil {
		return d
	}
	return out
}
