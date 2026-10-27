// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// maxFieldChanges bounds the field list, so one rewritten unit cannot bury the
// rest of a diff.
const maxFieldChanges = 40

// fieldChange is one stored field that differs between two revisions of a unit.
type fieldChange struct {
	kind   ChangeType
	path   string
	detail string // type and size, never the value
}

func (c fieldChange) String() string {
	return fmt.Sprintf("%s (%s)", c.path, c.detail)
}

// rawFieldChanges lists the fields that differ between two decoded revisions of
// a unit, by path and type, without values. It is diff-local's answer when the
// MDL renderings of both sides come out the same while the documents do not:
// the renderer does not show what changed (a unit type with no renderer, or a
// property one does not print), and saying "(no changes)" under "1 modified"
// is the silent diff mendixlabs/mxcli#1602 reported. Values are left out
// because the unit types without a renderer include ones that hold secrets.
func rawFieldChanges(oldDoc, newDoc map[string]any) []fieldChange {
	var out []fieldChange
	diffRawValue("", oldDoc, newDoc, &out)
	return out
}

func diffRawValue(path string, a, b any, out *[]fieldChange) {
	if am, ok := fieldDoc(a); ok {
		if bm, ok := fieldDoc(b); ok {
			diffRawDocs(path, am, bm, out)
			return
		}
	}
	if aa, ok := fieldList(a); ok {
		if ba, ok := fieldList(b); ok {
			diffRawLists(path, aa, ba, out)
			return
		}
	}
	if rawEqual(a, b) {
		return
	}
	*out = append(*out, fieldChange{kind: ChangeModified, path: path, detail: changedDetail(a, b)})
}

func diffRawDocs(path string, a, b map[string]any, out *[]fieldChange) {
	keys := make([]string, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		p := k
		if path != "" {
			p = path + "." + k
		}
		av, inA := a[k]
		bv, inB := b[k]
		switch {
		case !inA:
			*out = append(*out, fieldChange{kind: ChangeAdded, path: p, detail: "added, " + rawKind(bv)})
		case !inB:
			*out = append(*out, fieldChange{kind: ChangeRemoved, path: p, detail: "removed, " + rawKind(av)})
		default:
			diffRawValue(p, av, bv, out)
		}
	}
}

// diffRawLists pairs list elements by Name when every element has a distinct
// one, so inserting a role does not report every later role as changed, and by
// position otherwise.
func diffRawLists(path string, a, b []any, out *[]fieldChange) {
	an, aok := namedElements(a)
	bn, bok := namedElements(b)
	if aok && bok {
		for _, el := range a {
			name := elementName(el)
			p := fmt.Sprintf("%s[%s]", path, strconv.Quote(name))
			if other, ok := bn[name]; ok {
				diffRawValue(p, el, other, out)
			} else {
				*out = append(*out, fieldChange{kind: ChangeRemoved, path: p, detail: "removed"})
			}
		}
		for _, el := range b {
			name := elementName(el)
			if _, ok := an[name]; !ok {
				*out = append(*out, fieldChange{kind: ChangeAdded, path: fmt.Sprintf("%s[%s]", path, strconv.Quote(name)), detail: "added"})
			}
		}
		return
	}
	for i := range max(len(a), len(b)) {
		p := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case i >= len(a):
			*out = append(*out, fieldChange{kind: ChangeAdded, path: p, detail: "added, " + rawKind(b[i])})
		case i >= len(b):
			*out = append(*out, fieldChange{kind: ChangeRemoved, path: p, detail: "removed, " + rawKind(a[i])})
		default:
			diffRawValue(p, a[i], b[i], out)
		}
	}
}

// namedElements indexes list elements by Name, reporting false unless every
// element is a document with a distinct, non-empty one.
func namedElements(list []any) (map[string]any, bool) {
	byName := make(map[string]any, len(list))
	for _, el := range list {
		name := elementName(el)
		if name == "" {
			return nil, false
		}
		if _, dup := byName[name]; dup {
			return nil, false
		}
		byName[name] = el
	}
	return byName, len(list) > 0
}

func elementName(el any) string {
	if m, ok := fieldDoc(el); ok {
		return extractString(m["Name"])
	}
	return ""
}

func fieldDoc(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case bson.M:
		return map[string]any(m), true
	case bson.D:
		out := make(map[string]any, len(m))
		for _, e := range m {
			out[e.Key] = e.Value
		}
		return out, true
	}
	return nil, false
}

// asList returns a list's elements without Mendix's leading type marker.
func fieldList(v any) ([]any, bool) {
	switch v.(type) {
	case bson.A, []any:
		return extractBsonArray(v), true
	}
	return nil, false
}

func rawEqual(a, b any) bool {
	if ab, ok := rawBytes(a); ok {
		if bb, ok := rawBytes(b); ok {
			return bytes.Equal(ab, bb)
		}
	}
	return reflect.DeepEqual(a, b)
}

func rawBytes(v any) ([]byte, bool) {
	switch x := v.(type) {
	case []byte:
		return x, true
	case bson.Binary:
		return x.Data, true
	}
	return nil, false
}

// changedDetail describes a changed scalar by type and size only.
func changedDetail(a, b any) string {
	ka, kb := rawKind(a), rawKind(b)
	if ka != kb {
		return ka + " → " + kb
	}
	if sa, ok := a.(string); ok {
		sb, _ := b.(string)
		return fmt.Sprintf("string, %d → %d chars", len([]rune(sa)), len([]rune(sb)))
	}
	if ba, ok := rawBytes(a); ok {
		bb, _ := rawBytes(b)
		return fmt.Sprintf("binary, %d → %d bytes", len(ba), len(bb))
	}
	return ka
}

func rawKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int32, int64, int, float64:
		return "number"
	}
	if _, ok := rawBytes(v); ok {
		return "binary"
	}
	if _, ok := fieldDoc(v); ok {
		return "object"
	}
	if _, ok := fieldList(v); ok {
		return "list"
	}
	return fmt.Sprintf("%T", v)
}

// fieldChangesMDL renders field changes as MDL comments, so they read as a
// hunk in the unified and side-by-side views.
func fieldChangesMDL(changes []fieldChange) string {
	lines := []string{"-- Changed fields this view does not render (values not shown):"}
	for i, c := range changes {
		if i == maxFieldChanges {
			lines = append(lines, fmt.Sprintf("--   … and %d more", len(changes)-maxFieldChanges))
			break
		}
		lines = append(lines, "--   "+c.String())
	}
	return strings.Join(lines, "\n")
}

// fieldStructuralChanges is the same list for the structural view.
func fieldStructuralChanges(changes []fieldChange) []StructuralChange {
	var out []StructuralChange
	for i, c := range changes {
		if i == maxFieldChanges {
			out = append(out, StructuralChange{ChangeType: ChangeModified, ElementType: "Fields",
				Details: fmt.Sprintf("%d more", len(changes)-maxFieldChanges)})
			break
		}
		out = append(out, StructuralChange{ChangeType: c.kind, ElementType: "Field", ElementName: c.path, Details: c.detail})
	}
	return out
}
