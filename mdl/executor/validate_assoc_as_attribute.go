// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// An association given to a pluggable widget's attribute-typed property passed
// `check` and was refused by `exec` (mendixlabs/mxcli#1440):
//
//	alter snippet M.S { replace <widget> with {
//	  combobox x (Attribute: $Asset.Asset_Customer, CaptionAttribute: …)
//	} };
//
//	check: Check passed!
//	exec:  … property `attributeEnumeration` binds `Asset_Customer`, which is an
//	       association … not an attribute
//
// exec is right — an association cannot be stored in an attribute-typed
// property (#830, rejectAssociationAsAttribute) — and check never asked. It now
// asks exec's own question, through the same function, with the entity exec
// resolves the name against: the parameter's entity for `$Param.Name`, the
// property's binding scope otherwise. CREATE PAGE / SNIPPET asks it in the
// pluggable attribute-scope walk (checkWidget); an ALTER's INSERT / REPLACE asks
// it here, against the stored document's parameters and insertion point.

// associationAsAttributeError is exec's refusal for one attribute mapping's
// value: val as the engine's Attribute mapping reads it, entity the scope a bare
// name resolves in, paramEntity the entity of a document parameter by name. ""
// for an entity or a parameter that cannot be established leaves the value
// unjudged — exec reports an unknown parameter itself.
func associationAsAttributeError(pb *pageBuilder, w *ast.WidgetV3, key, val, entity string, paramEntity func(string) string) error {
	if pb == nil || val == "" {
		return nil
	}
	where := fmt.Sprintf("widget `%s` property `%s`", w.Name, key)
	if name, rest, named := namedObjectBinding(val); named {
		return pb.rejectAssociationAsAttributeOn(w, name, rest, paramEntity(name), where)
	}
	return pb.rejectAssociationAsAttributeOn(w, "", val, entity, where)
}

// alterAssociationAsAttribute judges the pluggable widgets one INSERT / REPLACE
// places, bound against the insertion point's entity and the stored document's
// parameters. A widget with a data source of its own binds its attributes to
// that source, and a pluggable widget's children sit in slots this does not
// model, so neither is descended into or judged here.
func alterAssociationAsAttribute(ctx *ExecContext, mutator backend.PageMutator, target string, into bool,
	modName string, modID model.ID, widgets []*ast.WidgetV3) []string {
	registry := pageContextWidgetRegistry(ctx)
	if registry == nil {
		return nil
	}
	entity, _ := alterEntityContext(ctx, mutator, target, into, modName, modID)
	_, params := mutator.ParamScope()
	paramEntity := func(name string) string {
		for p, e := range params {
			if strings.EqualFold(p, name) {
				return e
			}
		}
		return ""
	}
	pb := &pageBuilder{ctx: ctx, backend: ctx.Backend, execCache: ctx.Cache, moduleName: modName, moduleID: modID}

	var out []string
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil || w.GetDataSource() != nil {
				continue
			}
			def := lookupWidgetDef(w, registry)
			if def == nil {
				walk(w.Children)
				continue
			}
			engine := &PluggableWidgetEngine{currentDef: def}
			mappings, _, err := engine.selectMappings(def, w)
			if err != nil {
				continue
			}
			for _, m := range mappings {
				if m.Source != "Attribute" {
					continue
				}
				val := namedPropValue(m, w)
				if val == "" && engine.isPrimaryAttributeMapping(m) {
					val = w.GetAttribute()
				}
				if err := associationAsAttributeError(pb, w, m.PropertyKey, val, entity, paramEntity); err != nil {
					out = append(out, err.Error())
				}
			}
		}
	}
	walk(widgets)
	return out
}
