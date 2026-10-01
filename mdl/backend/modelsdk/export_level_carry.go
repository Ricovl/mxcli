// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// A document's ExportLevel says whether it is part of its module's public API
// (API) or not (Hidden). The rewrite converters build a fresh document and write
// the level as a constant — "Hidden", or the semantic model's value where the
// executor itself filled in "Hidden" — and the rewrite replaces the unit
// wholesale. So `describe` -> `exec`, CREATE OR MODIFY, or an ALTER that
// rebuilds the document turned an API document Hidden (ako/mxcli#816, the
// per-document half of #801). Nothing reports it: a Hidden document is valid and
// the build is clean; the loss shows only when the module is exported as a
// package and its public surface is gone.
//
// Most document kinds have no MDL spelling for their export level, so a rewrite
// cannot have asked for a change: the stored value is kept unconditionally
// (keepStoredExportLevel). A kind that does have a spelling — a workflow's
// `export level`, a scheduled event's, queue's or regular expression's
// ExportLevel property — keeps the stored value only when the statement did not
// author one (keepStoredExportLevelUnlessSet).
//
// Every rewrite of an existing document that writes ExportLevel goes through one
// of the two; TestUpdatePaths_KeepStoredExportLevel enumerates them.

// keepStoredExportLevel returns contents — the freshly encoded rewrite of unit
// unitID — with its top-level ExportLevel set to the value stored on the unit.
//
// Every other byte of contents is kept as encoded: the document is rebuilt
// element by element, and each element other than ExportLevel is copied
// verbatim. A rewrite that does not write ExportLevel at all is left alone —
// the key may be one the project's metamodel does not declare — and so is one
// whose stored unit has no ExportLevel, or cannot be read: this is a fidelity
// carry on a rewrite, not a precondition for one.
func (b *Backend) keepStoredExportLevel(unitID string, contents []byte) ([]byte, error) {
	if b.reader == nil || unitID == "" || len(contents) == 0 {
		return contents, nil
	}
	stored, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil || len(stored) == 0 {
		return contents, nil
	}
	lvl, ok := bsoncore.Document(stored).Lookup("ExportLevel").StringValueOK()
	if !ok || lvl == "" {
		return contents, nil
	}
	return withExportLevel(contents, lvl)
}

// keepStoredExportLevelUnlessSet is keepStoredExportLevel for a document kind
// whose statement can spell its export level: authored is the level the
// statement set, and a non-empty one is written as the statement said.
func (b *Backend) keepStoredExportLevelUnlessSet(unitID, authored string, contents []byte) ([]byte, error) {
	if authored != "" {
		return contents, nil
	}
	return b.keepStoredExportLevel(unitID, contents)
}

// withExportLevel returns doc with the value of its top-level ExportLevel
// element replaced by lvl, or doc itself when it has no such element or already
// holds lvl.
func withExportLevel(doc []byte, lvl string) ([]byte, error) {
	elems, err := bsoncore.Document(doc).Elements()
	if err != nil {
		return nil, fmt.Errorf("carry stored ExportLevel: %w", err)
	}
	found := false
	for _, el := range elems {
		if el.Key() != "ExportLevel" {
			continue
		}
		if cur, ok := el.Value().StringValueOK(); ok && cur == lvl {
			return doc, nil
		}
		found = true
	}
	if !found {
		return doc, nil
	}
	idx, out := bsoncore.AppendDocumentStart(nil)
	for _, el := range elems {
		if el.Key() == "ExportLevel" {
			out = bsoncore.AppendStringElement(out, "ExportLevel", lvl)
			continue
		}
		out = append(out, el...)
	}
	out, err = bsoncore.AppendDocumentEnd(out, idx)
	if err != nil {
		return nil, fmt.Errorf("carry stored ExportLevel: %w", err)
	}
	return out, nil
}

// keepStoredTopLevel returns contents with each top-level key in keys set to
// the value the stored unit holds, for a document kind whose statement cannot
// state those properties and whose writer therefore emits constants for them.
// A key the stored unit lacks is left as written; a key only the stored unit
// has is inserted before the first written key that sorts after it, the order
// Studio Pro writes.
func (b *Backend) keepStoredTopLevel(unitID string, contents []byte, keys []string) ([]byte, error) {
	if b.reader == nil || unitID == "" || len(contents) == 0 {
		return contents, nil
	}
	stored, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil || len(stored) == 0 {
		return contents, nil
	}
	return withStoredTopLevel(contents, stored, keys)
}

// withStoredTopLevel is keepStoredTopLevel on bytes.
func withStoredTopLevel(doc, stored []byte, keys []string) ([]byte, error) {
	carry := map[string]bsoncore.Value{}
	for _, k := range keys {
		if v, err := bsoncore.Document(stored).LookupErr(k); err == nil {
			carry[k] = v
		}
	}
	if len(carry) == 0 {
		return doc, nil
	}
	elems, err := bsoncore.Document(doc).Elements()
	if err != nil {
		return nil, fmt.Errorf("carry stored properties: %w", err)
	}
	present := map[string]bool{}
	for _, el := range elems {
		present[el.Key()] = true
	}
	var missing []string
	for k := range carry {
		if !present[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	idx, out := bsoncore.AppendDocumentStart(nil)
	for _, el := range elems {
		key := el.Key()
		for len(missing) > 0 && missing[0] < key && !strings.HasPrefix(key, "$") {
			out = bsoncore.AppendValueElement(out, missing[0], carry[missing[0]])
			missing = missing[1:]
		}
		if v, ok := carry[key]; ok {
			out = bsoncore.AppendValueElement(out, key, v)
			continue
		}
		out = append(out, el...)
	}
	for _, k := range missing {
		out = bsoncore.AppendValueElement(out, k, carry[k])
	}
	out, err = bsoncore.AppendDocumentEnd(out, idx)
	if err != nil {
		return nil, fmt.Errorf("carry stored properties: %w", err)
	}
	return out, nil
}
