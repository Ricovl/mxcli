// SPDX-License-Identifier: Apache-2.0

// Package bsoncompat holds the decode shims that keep mxcli's BSON reads
// byte-for-byte equivalent to what the v1 driver produced.
//
// It deliberately depends on nothing inside mxcli — only the BSON driver — so
// that every layer can import it. modelsdk/codec cannot host it: codec imports
// modelsdk/mpr, which imports modelsdk/canon and modelsdk/meta, so those two
// packages' own tests could not then import codec.
package bsoncompat

import (
	"bytes"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// UnmarshalDocM decodes a BSON document into a bson.M whose nested documents
// are bson.M as well, at every level, including inside arrays.
//
// Why this exists: the plain bson.Unmarshal of the v2 driver decodes a nested
// document into a bson.D even when the destination is a bson.M. The v1 driver
// produced a primitive.M (== bson.M) at every level. Code that reads a nested
// document back with a `doc["Key"].(bson.M)` type assertion therefore stops
// matching after the v1 -> v2 move, and does so SILENTLY, because a failed
// type assertion is not an error — the value is simply dropped, and the next
// write emits a document with a property missing. Studio Pro compares property
// names on merge, so such a document is unmergeable (the
// canon.CompletePropertySets class, mendixlabs/mxcli#1373).
//
// Measured on the same input bytes:
//
//	destination   v1 nested      v2 plain       v2 + this
//	bson.M        primitive.M    bson.D   (!)   bson.M
//	bson.D        primitive.D    bson.D         bson.M   (!)
//
// So use this ONLY where the destination is a bson.M. Where the destination is
// a bson.D, plain bson.Unmarshal is already v1-equivalent and this helper
// would itself introduce the bug, in the other direction.
func UnmarshalDocM(data []byte, out any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(data)))
	dec.DefaultDocumentM()
	return dec.Decode(out)
}

// DMap is the v1 driver's bson.D.Map(), which the v2 driver does not provide:
// the document's elements as a map, last value winning on a duplicate key.
//
// Order is lost, so this is for reading a value out by key, never for writing
// a document back — Studio Pro's property order is part of what mxcli matches.
// UnmarshalDocMap is UnmarshalDocM for a map[string]any destination, whose
// nested documents it decodes as map[string]any too.
//
// bson.M is a NAMED type (type M map[string]any), so a value of type bson.M
// does not satisfy a `.(map[string]any)` assertion and vice versa. A
// destination declared map[string]any therefore needs this, not
// UnmarshalDocM — v1 nested a plain map[string]any there, and both the plain
// v2 decode (bson.D) and UnmarshalDocM (bson.M) fail that assertion.
// mdl/executor's TestResolveWebServiceName_ReadsMapDecodedDocuments pins this
// exact case; it cost a debugging round once already.
func UnmarshalDocMap(data []byte, out any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(data)))
	dec.DefaultDocumentMap()
	return dec.Decode(out)
}

func DMap(d bson.D) map[string]any {
	m := make(map[string]any, len(d))
	for _, e := range d {
		m[e.Key] = e.Value
	}
	return m
}
