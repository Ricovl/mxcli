// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"sort"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CarryOutputMappings copies onto a rebuilt document the output mappings of a
// flow call that the statement had no way to say.
//
// # Why anything is lost
//
// Mendix 11.5 lets a call-microflow or call-nanoflow action map what the flow
// returns back onto the page (Forms$MicroflowSettings.OutputMappings,
// Forms$CallNanoflowClientAction.OutputMappings). MDL has no syntax for them,
// so a rebuild writes the empty list Studio Pro stores on every other call, and
// describe → exec dropped every mapping: "Replaced page", the mapping gone,
// mx check silent (ako/mxcli#977). This is the carry Reconcile already does for
// translations, for the same reason: the rebuild is authoritative about what
// the statement said and silent about what it could not say.
//
// # How lists are paired
//
// By the named element that owns the call and the path from it — "the button
// btnGo, /Action/MicroflowSettings" — and only where that address occurs once
// in each document and is reached without passing through a list. A list is
// carried only when
//
//   - the rebuilt list is empty: a statement that wrote mappings is the
//     authority for them;
//   - the call still names the same flow: mappings read the flow's result, so
//     they mean nothing for another one, and carrying them would bind a page to
//     outputs the new flow does not have;
//   - none of the carried elements' ids is already in the rebuilt document.
//     Rebuilt ids are freshly minted, so this does not happen in practice, but a
//     duplicate id makes the unit unopenable, which is not a risk worth taking
//     for a carry.
//
// The elements are carried verbatim, ids included, so the canonical form sees
// the same elements it saw before and an unchanged button is an unchanged
// document. Anything that cannot be read returns the contents untouched.
func CarryOutputMappings(contents, stored []byte) []byte {
	if len(contents) == 0 || len(stored) == 0 {
		return contents
	}
	var storedDoc, newDoc bson.D
	if bson.Unmarshal(stored, &storedDoc) != nil {
		return contents
	}
	storedLists := outputMappingHolders(storedDoc)
	if len(storedLists) == 0 {
		return contents
	}
	if bson.Unmarshal(contents, &newDoc) != nil {
		return contents
	}
	newLists := outputMappingHolders(newDoc)
	used := elementIDs(newDoc)

	addrs := make([]string, 0, len(storedLists))
	for a := range storedLists {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)

	changed := false
	for _, addr := range addrs {
		s, n := storedLists[addr], newLists[addr]
		if n == nil || flowOf(s) == "" || flowOf(s) != flowOf(n) {
			continue
		}
		have, ok := docLookup(n, "OutputMappings").(bson.A)
		if !ok || len(listElements(have)) > 0 {
			continue
		}
		carried := listElements(docLookup(s, "OutputMappings").(bson.A))
		if len(carried) == 0 || anyIDUsed(carried, used) {
			continue
		}
		for _, e := range carried {
			markIDs(e, used)
			have = append(have, e)
		}
		for i := range n {
			if n[i].Key == "OutputMappings" {
				n[i].Value = have
			}
		}
		changed = true
	}
	if !changed {
		return contents
	}
	out, err := bson.Marshal(newDoc)
	if err != nil {
		return contents
	}
	return out
}

// outputMappingHolders addresses every flow call holding an OutputMappings list
// by its owning named element and the path from it, keeping only the addresses
// that are exact: owned by an element with a $Type and a Name, reached from it
// without passing through a list, and occurring once in the document. In the
// stored document only calls with a non-empty list matter, but the rebuilt one
// needs every call, so both are returned and the caller filters.
func outputMappingHolders(doc bson.D) map[string]bson.D {
	out := map[string]bson.D{}
	count := map[string]int{}
	var walk func(v any, element, rel string, indexed bool)
	walk = func(v any, element, rel string, indexed bool) {
		switch n := v.(type) {
		case bson.D:
			ty, _ := docLookup(n, "$Type").(string)
			if ty == "Forms$MicroflowSettings" || ty == "Forms$CallNanoflowClientAction" {
				if _, ok := docLookup(n, "OutputMappings").(bson.A); ok && element != "" && !indexed {
					key := element + "\x00" + rel
					count[key]++
					out[key] = n
				}
			}
			if name, _ := docLookup(n, "Name").(string); name != "" && ty != "" {
				element, rel, indexed = ty+"\x00"+name, "", false
			}
			for _, e := range n {
				walk(e.Value, element, rel+"/"+e.Key, indexed)
			}
		case bson.A:
			for i, e := range n {
				walk(e, element, rel+"/"+strconv.Itoa(i), true)
			}
		}
	}
	walk(doc, "", "", false)
	for key, c := range count {
		if c > 1 {
			delete(out, key)
		}
	}
	return out
}

// flowOf is the flow a call names: a MicroflowSettings' Microflow or a
// nanoflow call's Nanoflow, both by qualified name.
func flowOf(d bson.D) string {
	if s, _ := docLookup(d, "Microflow").(string); s != "" {
		return s
	}
	s, _ := docLookup(d, "Nanoflow").(string)
	return s
}

// listElements returns a stored list's elements, without its leading marker.
func listElements(a bson.A) []bson.D {
	var out []bson.D
	for _, e := range a {
		if d, ok := e.(bson.D); ok {
			out = append(out, d)
		}
	}
	return out
}

func anyIDUsed(elems []bson.D, used map[string]bool) bool {
	for _, e := range elems {
		for id := range elementIDs(e) {
			if used[id] {
				return true
			}
		}
	}
	return false
}

func markIDs(e bson.D, used map[string]bool) {
	for id := range elementIDs(e) {
		used[id] = true
	}
}
