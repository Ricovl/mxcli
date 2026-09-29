// SPDX-License-Identifier: Apache-2.0

package mfmutator

// # Document properties as a patch (ako/mxcli#818)
//
// `create or modify` of a stored flow whose header changes — a return type, a
// URL, an export level, a parameter — used to rebuild the whole document,
// because the splice edits activities only; under `mdl 1` it was refused. A
// header is not part of the graph, though: every one of its properties is a
// value of the unit itself, or (a parameter) an object no flow connects to. So
// it is patched like the rest: the declared document is encoded exactly as the
// rebuild would encode it, and each header property whose value differs is
// copied onto the stored document. Nothing else is touched: no object, no
// flow, and no property a create statement cannot state.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// headerKeys are the document properties a create statement states: its
// clauses, its return type and its documentation. A property the statement
// has no syntax for (MarkAsUsed, AllowedModuleRoles, StableId, …) is not
// among them, so it is never written by this path, whatever the encoder
// produces for it.
var headerKeys = []string{
	"Documentation",
	"Excluded",
	"MicroflowReturnType",
	"ReturnVariableName",
	"ApplyEntityAccess",
	"ExportLevel",
	"AllowConcurrentExecution",
	"ConcurrenyErrorMessage", // sic: Mendix's storage name
	"ConcurrencyErrorMessage",
	"ConcurrencyErrorMicroflow",
	"Url",
	"UrlSearchParameters",
	"MicroflowActionInfo",
	"WorkflowActionInfo",
}

const parameterType = "Microflows$MicroflowParameter"

// parameterTypeKey is where a parameter stores its type: the metamodel's
// ParameterType is stored as VariableType.
const parameterTypeKey = "VariableType"

// SetHeader writes the header of declared onto the stored document; see
// backend.MicroflowMutator.
func (m *Mutator) SetHeader(declared any) ([]string, error) {
	d, err := m.deps.SerializeDocument(declared)
	if err != nil {
		return nil, fmt.Errorf("encode the declared header: %w", err)
	}
	if got, want := dString(d, "$Type"), dString(m.doc, "$Type"); got != want {
		return nil, fmt.Errorf("the declared document is a %s, the stored one a %s", got, want)
	}
	var changed []string
	for _, key := range headerKeys {
		nv, inNew := lookup(d, key)
		if !inNew {
			// The encoder does not write it for this document (a void
			// nanoflow's return type, a pre-10 URL): nothing is stated.
			continue
		}
		ov, inOld := lookup(m.doc, key)
		if !inOld {
			if isZero(nv) {
				continue
			}
			return nil, fmt.Errorf("the stored document has no %s property, so the value the statement gives it cannot be "+
				"patched in; set it in Studio Pro", key)
		}
		carried, same, err := carryValue(key, nv, ov)
		if err != nil {
			return nil, err
		}
		if same {
			continue
		}
		m.replaceValue(ov, carried)
		dSet(m.doc, key, carried)
		changed = append(changed, strings.Replace(key, "Concurreny", "Concurrency", 1))
	}
	params, err := m.setParameters(d)
	if err != nil {
		return nil, err
	}
	return append(changed, params...), nil
}

// carryValue prepares a declared property value for the stored document: the
// languages of a text the statement could not state are carried from the
// stored one, and every element that corresponds to a stored one keeps the
// stored $ID — the same two carries a rebuild's write gets (canon.Reconcile),
// confined to this property. same reports that the result is canonically the
// stored value, so there is nothing to write.
func carryValue(key string, declared, stored any) (any, bool, error) {
	nb, err := bson.Marshal(bson.D{{Key: key, Value: declared}})
	if err != nil {
		return nil, false, fmt.Errorf("encode %s: %w", key, err)
	}
	ob, err := bson.Marshal(bson.D{{Key: key, Value: stored}})
	if err != nil {
		return nil, false, fmt.Errorf("encode the stored %s: %w", key, err)
	}
	nb = canon.CarryTranslations(nb, ob)
	nb = canon.TransplantIDs(nb, ob)
	if eq, err := canon.Equal(nb, ob); err == nil && eq {
		return stored, true, nil
	}
	var out bson.D
	if err := bson.Unmarshal(nb, &out); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", key, err)
	}
	v, _ := lookup(out, key)
	return v, false, nil
}

// replaceValue records the elements of a stored value that the new value no
// longer holds, so Save can prove nothing still points at one of them.
func (m *Mutator) replaceValue(old, replacement any) {
	keep := map[string]bool{}
	collectIDs(replacement, keep)
	gone := map[string]bool{}
	collectIDs(old, gone)
	for id := range gone {
		if !keep[id] {
			m.removed[id] = true
		}
	}
}

func collectIDs(v any, into map[string]bool) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "$ID" {
				if b, ok := e.Value.(primitive.Binary); ok {
					into[string(b.Data)] = true
				}
				continue
			}
			collectIDs(e.Value, into)
		}
	case bson.A:
		for _, el := range t {
			collectIDs(el, into)
		}
	}
}

// setParameters makes the stored parameters the declared ones, matched by
// name: a parameter the statement adds is inserted after the one it follows, a
// retyped one gets the declared type in place (its $ID, position and
// documentation stay), and a removed one is taken out — unless the flow still
// uses it, which would leave an expression naming a variable that no longer
// exists. Where a kept parameter is drawn is not this function's concern
// (Move is), and neither are the properties MDL cannot state.
//
// Reordering is refused: the order of the parameters is the order a caller
// passes arguments in, and it is the order they are stored in.
func (m *Mutator) setParameters(declared bson.D) ([]string, error) {
	oc := dDoc(m.doc, "ObjectCollection")
	objs := arrayElements(dGet(oc, "Objects"))
	var declParams []bson.D
	for _, el := range arrayElements(dGet(dDoc(declared, "ObjectCollection"), "Objects")) {
		if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType {
			declParams = append(declParams, p)
		}
	}
	storedAt := map[string]int{} // name -> index in objs
	var storedOrder []string
	for i, el := range objs {
		if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType {
			storedAt[dString(p, "Name")] = i
			storedOrder = append(storedOrder, dString(p, "Name"))
		}
	}
	inDecl := map[string]bool{}
	var keptOrder []string
	for _, p := range declParams {
		name := dString(p, "Name")
		inDecl[name] = true
		if _, ok := storedAt[name]; ok {
			keptOrder = append(keptOrder, name)
		}
	}
	var storedKept []string
	for _, name := range storedOrder {
		if inDecl[name] {
			storedKept = append(storedKept, name)
		}
	}
	for i := range keptOrder {
		if keptOrder[i] != storedKept[i] {
			return nil, fmt.Errorf("the parameters are reordered ($%s where $%s is stored); the order is the order "+
				"callers pass arguments in, and a reorder is not patched: reorder them in Studio Pro", keptOrder[i], storedKept[i])
		}
	}

	var changed []string
	var removed []string
	for _, name := range storedOrder {
		if !inDecl[name] {
			removed = append(removed, name)
		}
	}
	if len(removed) > 0 {
		drop := map[int]bool{}
		for _, name := range removed {
			drop[storedAt[name]] = true
		}
		for _, name := range removed {
			if where := m.usesVariable(name, drop); where != "" {
				return nil, fmt.Errorf("parameter $%s is removed, but the flow still uses it (%s); an expression "+
					"naming it would be left behind", name, where)
			}
		}
		var keep []any
		for i, el := range objs {
			if drop[i] {
				m.markRemoved(el)
				continue
			}
			keep = append(keep, el)
		}
		objs = keep
		for _, name := range removed {
			changed = append(changed, "parameter $"+name+" removed")
		}
	}

	index := func(name string) int {
		for i, el := range objs {
			if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType && dString(p, "Name") == name {
				return i
			}
		}
		return -1
	}
	insertAt := 0 // parameters go first, as the rebuild writes them
	for _, dp := range declParams {
		name := dString(dp, "Name")
		if i := index(name); i >= 0 {
			sp, _ := objs[i].(bson.D)
			key := parameterTypeKey
			nv, inNew := lookup(dp, key)
			ov, inOld := lookup(sp, key)
			if !inNew || !inOld {
				return nil, fmt.Errorf("parameter $%s stores no %s", name, key)
			}
			carried, same, err := carryValue(key, nv, ov)
			if err != nil {
				return nil, err
			}
			if !same {
				m.replaceValue(ov, carried)
				dSet(sp, key, carried)
				changed = append(changed, "parameter $"+name+" type")
			}
			insertAt = i + 1
			continue
		}
		objs = append(objs[:insertAt], append([]any{dp}, objs[insertAt:]...)...)
		insertAt++
		changed = append(changed, "parameter $"+name+" added")
	}
	if len(changed) > 0 && !setArray(oc, "Objects", objs) {
		return nil, fmt.Errorf("the object collection has no Objects list")
	}
	return changed, nil
}

// usesVariable returns where the flow still names $name in an expression, or
// "". It looks at every string in the unit's objects and flows outside the
// parameter objects being removed: a variable is referenced by name, in text,
// so a text search finds every use without knowing which properties hold
// expressions.
func (m *Mutator) usesVariable(name string, skip map[int]bool) string {
	re := regexp.MustCompile(`(?i)\$` + regexp.QuoteMeta(name) + `\b`)
	var hits []string
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch t := v.(type) {
		case bson.D:
			for _, e := range t {
				walk(e.Value, e.Key)
			}
		case bson.A:
			for _, el := range t {
				walk(el, key)
			}
		case string:
			if re.MatchString(t) {
				hits = append(hits, fmt.Sprintf("%s %q", key, t))
			}
		}
	}
	oc := dDoc(m.doc, "ObjectCollection")
	for i, el := range arrayElements(dGet(oc, "Objects")) {
		if !skip[i] {
			walk(el, "")
		}
	}
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		walk(el, "")
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return hits[0]
}

func lookup(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// isZero reports whether an encoded value says nothing: an empty string, false,
// or a list with no elements.
func isZero(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return !t
	case bson.A:
		return len(arrayElements(t)) == 0
	}
	return false
}
