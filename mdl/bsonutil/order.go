// SPDX-License-Identifier: Apache-2.0

package bsonutil

import (
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// OrderStorageValue recursively normalises a decoded BSON value into an ordered
// form suitable for re-marshalling, guaranteeing that every document places
// "$ID" first and "$Type" second (when present), with the remaining keys in
// deterministic sorted order.
//
// Mendix 11.12+ rejects any storage object whose first BSON property is not
// "$ID" (System.InvalidOperationException: "Expected '$ID' as the first property
// of a storage object, but got '...'"). Values decoded into Go maps
// (bson.M / map[string]any) lose key order, so marshalling them back produces a
// random first key. This helper restores the "$ID"-first invariant on write and,
// by sorting the remaining keys, also makes the serialized output deterministic
// (avoiding flaky BSON diffs). Field ordering beyond "$ID" first is tolerated by
// the Mendix loader, so the sort is safe.
//
// Documents already represented as an ordered bson.D are re-ordered the same way
// so the invariant holds regardless of the input shape. Arrays (bson.A / []any)
// are recursed element-wise with their order preserved (e.g. the leading
// versioned-array marker stays first). Scalars are returned unchanged.
func OrderStorageValue(v any) any {
	switch t := v.(type) {
	case bson.D:
		m := make(map[string]any, len(t))
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return orderMap(m)
	case bson.M:
		return orderMap(t)
	case map[string]any:
		return orderMap(t)
	case bson.A:
		out := make(bson.A, len(t))
		for i, e := range t {
			out[i] = OrderStorageValue(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = OrderStorageValue(e)
		}
		return out
	default:
		return v
	}
}

// HoistStorageID is the minimal counterpart to OrderStorageValue: it moves "$ID"
// first and "$Type" second in every document, but PRESERVES the original relative
// order of all other keys (for ordered bson.D input) instead of sorting them.
//
// Prefer this over OrderStorageValue on trees that contain pluggable-widget /
// datagrid page objects: those carry template-derived structures whose field
// order Mendix is sensitive to, and a blind sort corrupts them (mx aborts with
// "Expected '$ID' as the first property … but got 'LabelTemplate'"). This hoist
// changes only what Mendix 11.12 actually requires — "$ID" first — and leaves
// everything else byte-for-byte where it was. Go maps (which have no inherent
// order) still fall back to sorted output for stability.
func HoistStorageID(v any) any {
	switch t := v.(type) {
	case bson.D:
		return hoistPairs(t)
	case bson.M:
		return hoistPairs(dFromMap(t))
	case map[string]any:
		return hoistPairs(dFromMap(t))
	case bson.A:
		out := make(bson.A, len(t))
		for i, e := range t {
			out[i] = HoistStorageID(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = HoistStorageID(e)
		}
		return out
	default:
		return v
	}
}

// dFromMap converts a Go map to a bson.D with keys sorted (a bare map carries no
// order, so a stable sort is the only sensible input ordering); hoistPairs then
// lifts "$ID"/"$Type" to the front.
func dFromMap(m map[string]any) bson.D {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	d := make(bson.D, len(keys))
	for i, k := range keys {
		d[i] = bson.E{Key: k, Value: m[k]}
	}
	return d
}

// hoistPairs emits "$ID" first, "$Type" second, then every other element in its
// original order, recursing into values via HoistStorageID.
func hoistPairs(d bson.D) bson.D {
	out := make(bson.D, 0, len(d))
	for _, e := range d {
		if e.Key == "$ID" {
			out = append(out, bson.E{Key: e.Key, Value: HoistStorageID(e.Value)})
		}
	}
	for _, e := range d {
		if e.Key == "$Type" {
			out = append(out, bson.E{Key: e.Key, Value: HoistStorageID(e.Value)})
		}
	}
	for _, e := range d {
		if e.Key == "$ID" || e.Key == "$Type" {
			continue
		}
		out = append(out, bson.E{Key: e.Key, Value: HoistStorageID(e.Value)})
	}
	return out
}

// orderMap converts a map into an ordered bson.D with "$ID" first, "$Type"
// second, and all other keys sorted. Values are recursed through
// OrderStorageValue so nested storage objects gain the same ordering.
func orderMap(m map[string]any) bson.D {
	rest := make([]string, 0, len(m))
	for k := range m {
		if k == "$ID" || k == "$Type" {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)

	d := make(bson.D, 0, len(m))
	if v, ok := m["$ID"]; ok {
		d = append(d, bson.E{Key: "$ID", Value: OrderStorageValue(v)})
	}
	if v, ok := m["$Type"]; ok {
		d = append(d, bson.E{Key: "$Type", Value: OrderStorageValue(v)})
	}
	for _, k := range rest {
		d = append(d, bson.E{Key: k, Value: OrderStorageValue(m[k])})
	}
	return d
}
