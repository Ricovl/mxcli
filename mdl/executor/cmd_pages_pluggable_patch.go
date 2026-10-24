// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"io"
	"maps"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Patching a stored pluggable widget per property (ako/mxcli#832).
//
// The passthrough keeps a stored pluggable widget whole when the statement
// says the same about it as describe does. Any other widget was rebuilt whole
// from the template, so every property the statement did not mention lost what
// Studio Pro had stored — and "any other" included widgets the statement did
// not change at all, whenever describe printed one imperfectly. On ako/TestApp,
// re-running `describe page WorkflowCommons.TaskInbox` unchanged rebuilt both
// data grids: the Assignee(s) column, stored as Administration.Account.FullName
// through the assignees association, came back as System.User.FullName
// (CE1613), the row click that opens the task became a NoAction, a caption
// lost its second translation. Rebuilding every pluggable widget of TestApp
// this way changed 23 distinct properties nobody had touched.
//
// So a widget the passthrough declines is patched instead: the stored widget
// is kept, and only the properties the statement changes are written. Which
// those are is decided by building the widget twice from the same template —
// once from the statement, once from describe's own text for the stored widget
// — and comparing the two by property key. A property both builds agree on is
// one the statement said nothing new about, and keeps its stored value; a
// property they disagree on is the statement's, and is written from its build.
// The engine's mappings decide which properties an MDL keyword reaches, so the
// comparison needs no list of its own.
//
// Three things keep a stored value from being kept where it would be wrong:
//
//   - **The context.** The same text binds to other attributes under another
//     data view entity. A property that names model elements and is not bound
//     to one of the widget's own data sources keeps its stored value only when
//     the describe build names what the stored one names; one that does not
//     abandons the patch, and the widget is rebuilt as before. A property bound
//     to the widget's own data source (a column's attribute: DataSourceProperty
//     `../datasource`) is kept while that data source is — which is what keeps
//     the specialization describe cannot print.
//   - **Child widgets.** A Widgets property is always written from the
//     statement's build: its children were built, named and registered with
//     the IDs that build gave them, and stored children would not be those.
//     Each pluggable child is patched or passed through on its own.
//   - **The schema.** The stored Type is kept, as the passthrough keeps it; a
//     written property is moved onto it by key. A changed property the stored
//     schema has no slot for, or a written value that points at a type the
//     stored schema lacks, abandons the patch.
//
// The second build runs on describe's text in the statement's context, with
// child widgets stubbed (their values are never kept, and building them twice
// would build their own children twice), its output discarded and the widget
// scope restored afterwards. Any failure leaves the widget rebuilt as before:
// this is a fidelity improvement on a rewrite, not a precondition for one.

// patchStoredPluggable replaces a rebuilt pluggable widget's Type and Object
// with the stored ones, carrying over the properties the statement changes. It
// reports whether it did.
func (pb *pageBuilder) patchStoredPluggable(def *WidgetDefinition, w *ast.WidgetV3, cw *pages.CustomWidget) bool {
	pt := pb.storedPluggables
	if pt == nil || pb.pluggableEngine == nil || def == nil || w == nil || cw == nil || cw.RawType == nil || cw.RawObject == nil {
		return false
	}
	sp, ok := pt.byName[w.Name]
	if !ok || sp.declared == nil || sp.widgetID != def.WidgetID || !strings.EqualFold(w.Type, sp.declared.Type) {
		return false
	}
	shadow, err := pb.buildShadowPluggable(def, sp.declared)
	if err != nil || shadow == nil || shadow.RawType == nil || shadow.RawObject == nil {
		return false
	}
	m := &pluggableMerge{ids: map[string]any{}}
	sOT, aOT, bOT := bsonDocField(sp.typ, "ObjectType"), bsonDocField(cw.RawType, "ObjectType"), bsonDocField(shadow.RawType, "ObjectType")
	m.mapTypes(sOT, aOT)
	obj, ok := m.object(sOT, aOT, bOT, sp.obj, cw.RawObject, shadow.RawObject, nil)
	if !ok {
		return false
	}
	cw.RawType = cloneBsonD(sp.typ)
	cw.RawObject = obj
	// The ID maps describe the template's Type, which is no longer written.
	cw.PropertyTypeIDMap = nil
	cw.ObjectTypeID = ""
	return true
}

// buildShadowPluggable builds the widget describe prints for the stored one,
// in the current context, without side effects on the build in progress.
func (pb *pageBuilder) buildShadowPluggable(def *WidgetDefinition, w *ast.WidgetV3) (*pages.CustomWidget, error) {
	prevShadow, prevScope, prevCtx := pb.shadowBuild, pb.widgetScope, pb.ctx
	pb.shadowBuild = true
	// The statement's build registered the widget's name; this one registers
	// it again, into a scope that is thrown away.
	pb.widgetScope = maps.Clone(prevScope)
	delete(pb.widgetScope, w.Name)
	if pb.ctx != nil {
		quiet := *pb.ctx
		quiet.Output, quiet.Format = io.Discard, ""
		pb.ctx = &quiet
	}
	defer func() { pb.shadowBuild, pb.widgetScope, pb.ctx = prevShadow, prevScope, prevCtx }()
	return pb.pluggableEngine.Build(def, w)
}

// shadowChild is what a child widget of the second build is: its value is
// never kept, so only its place matters.
func shadowChild(w *ast.WidgetV3) pages.Widget {
	return &pages.Container{BaseWidget: pages.BaseWidget{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$DivContainer"},
		Name:        w.Name,
	}}
}

// pluggableMerge carries one widget's merge: ids maps each template type ID
// (property, value and object types) to the stored type's by key path.
type pluggableMerge struct {
	ids map[string]any
}

// mapTypes pairs the built object type's IDs with the stored one's.
func (m *pluggableMerge) mapTypes(stored, built bson.D) {
	if stored == nil || built == nil {
		return
	}
	m.ids[bsonIDKey(bsonValue(built, "$ID"))] = bsonValue(stored, "$ID")
	storedPTs := propertyTypesByKey(stored)
	for _, bpt := range bsonDocs(bsonValue(built, "PropertyTypes")) {
		spt := storedPTs[bsonString(bpt, "PropertyKey")]
		if spt == nil {
			continue
		}
		m.ids[bsonIDKey(bsonValue(bpt, "$ID"))] = bsonValue(spt, "$ID")
		bvt, svt := bsonDocField(bpt, "ValueType"), bsonDocField(spt, "ValueType")
		m.ids[bsonIDKey(bsonValue(bvt, "$ID"))] = bsonValue(svt, "$ID")
		m.mapTypes(bsonDocField(svt, "ObjectType"), bsonDocField(bvt, "ObjectType"))
	}
}

// object merges one widget object: the stored s, the statement's build a and
// the describe build b, each read through its own object type. parentChanged
// is the enclosing object's changed set, for `../` data source links.
func (m *pluggableMerge) object(sOT, aOT, bOT, s, a, b bson.D, parentChanged map[string]bool) (bson.D, bool) {
	aProps, bProps := propertiesByKey(aOT, a), propertiesByKey(bOT, b)
	aTypes := propertyTypesByKey(aOT)
	sKeys := propertyKeysByTypeID(bson.D{{Key: "ObjectType", Value: sOT}})
	stored := map[string]bool{}
	for _, sp := range bsonDocs(bsonValue(s, "Properties")) {
		stored[sKeys[bsonIDKey(bsonValue(sp, "TypePointer"))]] = true
	}
	changed := map[string]bool{}
	for k, ap := range aProps {
		changed[k] = !canonEqual(bsonValue(ap, "Value"), bsonValue(bProps[k], "Value"))
		if changed[k] && !stored[k] {
			return nil, false // the stored schema has no slot for what the statement writes
		}
	}

	out := cloneBsonD(s)
	props, _ := bsonValue(out, "Properties").(bson.A)
	for i, item := range props {
		sp, ok := item.(bson.D)
		if !ok {
			continue
		}
		key := sKeys[bsonIDKey(bsonValue(sp, "TypePointer"))]
		ap := aProps[key]
		if ap == nil {
			continue // a property the template does not have: as stored
		}
		vt := bsonDocField(aTypes[key], "ValueType")
		aVal, bVal := bsonDocField(ap, "Value"), bsonDocField(bProps[key], "Value")
		sVal := bsonDocField(sp, "Value")
		var nv any
		switch {
		case bsonString(vt, "Type") == "Widgets":
			nv = aVal
		case bsonString(vt, "Type") == "Object":
			merged, ok, keep := m.objectList(key, sOT, vt, bOT, sVal, aVal, bVal, changed[key], changed)
			if !ok {
				return nil, false
			}
			if keep {
				continue
			}
			nv = merged
		case !changed[key] && !linkedChanged(vt, changed, parentChanged):
			if bsonString(vt, "DataSourceProperty") == "" && !sameModelRefs(bVal, sVal) {
				return nil, false // bound differently than the statement reads it: the context changed
			}
			continue
		default:
			nv = aVal
		}
		remapped, ok := m.remap(nv)
		if !ok {
			return nil, false
		}
		props[i] = replaceValue(sp, remapped)
	}
	return out, true
}

// objectList merges an object-list property. It keeps the stored list (keep)
// when the statement leaves it alone, merges item by item when both builds and
// the stored widget hold the same number of items, and otherwise takes the
// statement's list.
// aVT is the statement build's value type for key; siblings is the enclosing
// object's changed set, which the items' `../` links read.
func (m *pluggableMerge) objectList(key string, sOT, aVT, bOT, sVal, aVal, bVal bson.D, changed bool, siblings map[string]bool) (any, bool, bool) {
	sObjs, aObjs, bObjs := bsonDocs(bsonValue(sVal, "Objects")), bsonDocs(bsonValue(aVal, "Objects")), bsonDocs(bsonValue(bVal, "Objects"))
	sNested := bsonDocField(bsonDocField(propertyTypesByKey(sOT)[key], "ValueType"), "ObjectType")
	aNested := bsonDocField(aVT, "ObjectType")
	if len(sObjs) != len(aObjs) || len(aObjs) != len(bObjs) || !canonEqual(withoutObjects(aVal), withoutObjects(bVal)) {
		if changed {
			// The statement's items, written whole: only where the stored
			// schema's item type has exactly the template's properties, or the
			// items would not match the stored Type that is kept.
			if !maps.Equal(propertyKeySet(sNested), propertyKeySet(aNested)) {
				return nil, false, false
			}
			return aVal, true, false
		}
		// Unchanged, but the stored list does not line up with describe's:
		// kept only when it names what the describe build names.
		return nil, sameModelRefs(bVal, sVal), true
	}
	bNested := bsonDocField(bsonDocField(propertyTypesByKey(bOT)[key], "ValueType"), "ObjectType")
	merged := bson.A{}
	if arr, ok := bsonValue(sVal, "Objects").(bson.A); ok && len(arr) > 0 {
		if marker, isMarker := arr[0].(int32); isMarker {
			merged = append(merged, marker)
		}
	}
	for i := range sObjs {
		o, ok := m.object(sNested, aNested, bNested, sObjs[i], aObjs[i], bObjs[i], siblings)
		if !ok {
			return nil, false, false
		}
		merged = append(merged, o)
	}
	out := cloneBsonD(sVal)
	for i, e := range out {
		if e.Key == "Objects" {
			out[i].Value = merged
		}
	}
	// The stored value keeps its own type pointer; nothing in it needs a remap.
	return storedValue{out}, true, false
}

// storedValue marks a merged value already expressed against the stored type.
type storedValue struct{ d bson.D }

// remap moves a value built against the template's type onto the stored type.
// A pointer into the template's type the stored one has no counterpart for
// fails the merge. Child widgets keep their own types and are not entered.
func (m *pluggableMerge) remap(v any) (any, bool) {
	if sv, ok := v.(storedValue); ok {
		return sv.d, true
	}
	c := cloneAny(v)
	ok := true
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case bson.D:
			if bsonString(x, "$Type") == "CustomWidgets$CustomWidget" {
				return
			}
			for i, e := range x {
				if e.Key == "TypePointer" {
					id, found := m.ids[bsonIDKey(e.Value)]
					if !found {
						ok = false
						return
					}
					x[i].Value = id
					continue
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(c)
	return c, ok
}

// linkedChanged reports whether the data source a value is bound to
// (DataSourceProperty `x`, or `../x` on the enclosing object) is one the
// statement changes. A link it cannot follow counts as changed.
func linkedChanged(vt bson.D, changed, parentChanged map[string]bool) bool {
	ds := bsonString(vt, "DataSourceProperty")
	switch {
	case ds == "":
		return false
	case strings.HasPrefix(ds, "../"):
		rest := strings.TrimPrefix(ds, "../")
		if parentChanged == nil || strings.Contains(rest, "/") {
			return true
		}
		return parentChanged[rest]
	case strings.Contains(ds, "/"):
		return true
	}
	return changed[ds]
}

func propertyKeySet(ot bson.D) map[string]bool {
	out := map[string]bool{}
	for k := range propertyTypesByKey(ot) {
		out[k] = true
	}
	return out
}

func propertyTypesByKey(ot bson.D) map[string]bson.D {
	out := map[string]bson.D{}
	for _, pt := range bsonDocs(bsonValue(ot, "PropertyTypes")) {
		out[bsonString(pt, "PropertyKey")] = pt
	}
	return out
}

// propertiesByKey indexes an object's properties by their key in ot.
func propertiesByKey(ot, obj bson.D) map[string]bson.D {
	keys := propertyKeysByTypeID(bson.D{{Key: "ObjectType", Value: ot}})
	out := map[string]bson.D{}
	for _, p := range bsonDocs(bsonValue(obj, "Properties")) {
		if k := keys[bsonIDKey(bsonValue(p, "TypePointer"))]; k != "" {
			out[k] = p
		}
	}
	return out
}

func replaceValue(prop bson.D, v any) bson.D {
	out := make(bson.D, len(prop))
	copy(out, prop)
	for i, e := range out {
		if e.Key == "Value" {
			out[i].Value = v
		}
	}
	return out
}

func withoutObjects(v bson.D) bson.D {
	out := bson.D{}
	for _, e := range v {
		if e.Key != "Objects" {
			out = append(out, e)
		}
	}
	return out
}

// canonEqual compares two built values with their element IDs and type
// pointers left out: the two builds mint both afresh.
func canonEqual(a, b any) bool {
	return reflect.DeepEqual(canonValue(a), canonValue(b))
}

func canonValue(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := bson.D{}
		for _, e := range x {
			if e.Key == "$ID" || e.Key == "TypePointer" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: canonValue(e.Value)})
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = canonValue(e)
		}
		return out
	}
	return v
}

// sameModelRefs reports whether two values name the same entities, attributes
// and associations.
func sameModelRefs(a, b bson.D) bool {
	return maps.Equal(modelRefs(a), modelRefs(b))
}

func cloneAny(v any) any {
	switch x := v.(type) {
	case bson.D:
		return cloneBsonD(x)
	case bson.A:
		d := cloneBsonD(bson.D{{Key: "v", Value: x}})
		return d[0].Value
	}
	return v
}
