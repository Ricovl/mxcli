// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Pluggable widget passthrough (ako/mxcli#721 L4).
//
// A pluggable widget is written as two coupled BSON structures, the Type (the
// package's property schema) and the Object (the values), and the engine builds
// both from the widget's TEMPLATE plus whatever the statement maps onto it.
// That is right for a new widget and wrong for a stored one: the template is a
// snapshot of one package version in one configuration, and a stored widget
// carries what Studio Pro wrote — its own schema version, its property order,
// translated captions, a row action MDL has no spelling for, every property MDL
// does not map. Rebuilt from the template, `describe page` → `exec` rewrote the
// data grids, combo boxes, filters and images of 37 documents across PedApp and
// ako/TestApp, while the run reported success and `mx check` stayed green:
// captions lost their translations, a row click lost its user-task action,
// `configurationStorageType` went from localStorage to attribute, and an
// Image's whole Type was replaced by the template's.
//
// The stored widget is therefore kept whenever the statement says the same
// thing about it as describe says about the stored one — the diff-then-patch
// rule `create or modify` already applies to flows (ADR-0012): a widget the
// statement did not change keeps its stored Type and Object byte for byte, and
// only a widget whose statement differs is rebuilt.
//
// "Says the same thing" is decided on the AST: the stored document is described
// in the script's own language and parsed back, and the widget's subtree there
// must equal the statement's. Equal text is not enough on its own, because the
// same subtree means something else under a different context — a data view
// switched to another entity binds the same bare attribute names to other
// attributes. So the rebuilt widget must also name no model element the stored
// one does not: every entity, attribute and association its Object refers to
// must already be referred to by the stored Object. A context change fails
// that, and so does a rebuild that disagrees with the stored widget about what
// it is bound to.

// storedPluggable is one pluggable widget of the stored document.
type storedPluggable struct {
	widgetID string
	typ, obj bson.D
	// declared is the widget as describe prints it, parsed back; nil when
	// describe printed no widget of this name.
	declared *ast.WidgetV3
	// refs are the model elements the stored Object names.
	refs map[string]bool
}

// pluggablePassthrough indexes a stored document's pluggable widgets by name.
type pluggablePassthrough struct {
	byName map[string]*storedPluggable
}

// loadPluggablePassthrough reads the stored unit's pluggable widgets and pairs
// each with the widget describe prints for it. describe writes the stored
// document to ctx.Output; the caller pins it to the unit being rewritten. A
// document with no pluggable widget is not described at all.
//
// Any failure returns nil, which leaves every widget to be rebuilt as before:
// this is a fidelity improvement on a rewrite, not a precondition for one.
func loadPluggablePassthrough(ctx *ExecContext, unitID model.ID, describe func() error) *pluggablePassthrough {
	if ctx == nil || ctx.Backend == nil || unitID == "" {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnitBytes(unitID)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	pt := &pluggablePassthrough{byName: map[string]*storedPluggable{}}
	ambiguous := map[string]bool{}
	collectWidgets(doc, func(w *types.CustomWidgetInstance) {
		typ, _ := w.RawType.(bson.D)
		obj, _ := w.RawObject.(bson.D)
		if w.WidgetName == "" || typ == nil || obj == nil {
			return
		}
		if _, dup := pt.byName[w.WidgetName]; dup {
			ambiguous[w.WidgetName] = true
			return
		}
		pt.byName[w.WidgetName] = &storedPluggable{
			widgetID: w.WidgetID,
			typ:      typ,
			obj:      obj,
			refs:     modelRefs(obj),
		}
	})
	for name := range ambiguous {
		delete(pt.byName, name)
	}
	if len(pt.byName) == 0 {
		return nil
	}

	declared, err := describedWidgets(ctx, describe)
	if err != nil {
		return nil
	}
	for name, sp := range pt.byName {
		sp.declared = declared[name]
	}
	return pt
}

// describedWidgets describes the stored document in the script's own language
// and parses it back, indexing every widget of the create statement by name. It
// follows describedFlowStmt: describing in another language would read one side
// by different rules than the other.
func describedWidgets(ctx *ExecContext, describe func() error) (map[string]*ast.WidgetV3, error) {
	var buf bytes.Buffer
	prevOut, prevIn := ctx.Output, ctx.describeIn
	script := ctx.LanguageVersion
	ctx.Output, ctx.describeIn = &buf, &script
	err := describe()
	src := describedSource(ctx, buf.String())
	ctx.Output, ctx.describeIn = prevOut, prevIn
	if err != nil {
		return nil, err
	}
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, fmt.Errorf("the description does not parse: %v", errs[0])
	}
	out := map[string]*ast.WidgetV3{}
	dup := map[string]bool{}
	var walk func([]*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if w.Name != "" {
				if _, seen := out[w.Name]; seen {
					dup[w.Name] = true
				}
				out[w.Name] = w
			}
			walk(w.Children)
		}
	}
	for _, st := range prog.Statements {
		switch s := st.(type) {
		case *ast.CreatePageStmtV3:
			walk(s.Widgets)
			for _, ph := range s.Placeholders {
				if ph != nil {
					walk(ph.Widgets)
				}
			}
		case *ast.CreateSnippetStmtV3:
			walk(s.Widgets)
		}
	}
	for name := range dup {
		delete(out, name)
	}
	return out, nil
}

// passStoredThrough replaces a rebuilt pluggable widget's Type and Object with
// the stored ones when the statement did not change the widget: same widget
// package, same subtree as describe prints for the stored widget, and nothing
// named that the stored widget does not name. It reports whether it did.
func (pt *pluggablePassthrough) passStoredThrough(def *WidgetDefinition, w *ast.WidgetV3, cw *pages.CustomWidget) bool {
	if pt == nil || def == nil || w == nil || cw == nil {
		return false
	}
	sp, ok := pt.byName[w.Name]
	if !ok || sp.declared == nil || sp.widgetID != def.WidgetID {
		return false
	}
	if !declaredMatches(w, sp.declared) {
		return false
	}
	for ref := range modelRefs(cw.RawObject) {
		if !sp.refs[ref] {
			return false
		}
	}
	cw.RawType = cloneBsonD(sp.typ)
	cw.RawObject = cloneBsonD(sp.obj)
	// The ID maps describe the template's Type, which is no longer written.
	cw.PropertyTypeIDMap = nil
	cw.ObjectTypeID = ""
	return true
}

// modelRefKeys are the keys under which a widget Object names a model element
// whose meaning depends on the entity context the widget is built in.
var modelRefKeys = map[string]bool{
	"Attribute":         true,
	"Entity":            true,
	"Association":       true,
	"DestinationEntity": true,
}

// modelRefs collects the entity, attribute and association names a widget
// Object refers to, nested widgets included.
func modelRefs(node any) map[string]bool {
	out := map[string]bool{}
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case bson.D:
			for _, e := range v {
				if s, ok := e.Value.(string); ok && modelRefKeys[e.Key] && s != "" {
					out[e.Key+"="+s] = true
					continue
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(node)
	return out
}

// cloneBsonD deep-copies a document through its encoding, so the stored index
// is never aliased by a widget the serializer may still touch.
func cloneBsonD(d bson.D) bson.D {
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
