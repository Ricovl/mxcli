// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// ALTER … SET on a pluggable widget's attribute property (ako/mxcli#1622).
//
// `set (eventDataAttribute: …) on chart1` was refused for every spelling: the
// mutator holds only BSON, and which entity a bare name belongs to is the
// widget's binding rule, not a fact of the document. So the mutator describes
// the property (backend.PluggablePropertyScoper) and this resolves the binding
// by the rule CREATE writes by — resolveBindingScope, misboundAttributeError,
// `$Param.Attr` — before handing it a backend.AttributeBinding. check
// --references dry-runs the same path against a copy of the document, so it
// refuses exactly what exec refuses.

// resolvePluggableSetValue returns the value SetWidgetProperty should receive
// for prop of widgetRef: an AttributeBinding for an Attribute-kind pluggable
// property, value unchanged for anything else. A `$Param.Attr` value on a
// property that takes no attribute is refused: the visitor lets that one
// expression shape through only for the attribute case.
func resolvePluggableSetValue(ctx *ExecContext, mutator backend.PageMutator, widgetRef, prop string, value any, moduleName string, moduleID model.ID) (any, error) {
	expr, isExpr := value.(*ast.WidgetExpressionV3)
	var scope backend.PluggablePropertyScope
	found := false
	if scoper, ok := mutator.(backend.PluggablePropertyScoper); ok && widgetRef != "" {
		scope, found = scoper.PluggablePropertyScope(widgetRef, prop)
	}
	if !found || scope.Kind != "Attribute" {
		if isExpr {
			return nil, mdlerrors.NewValidationf(
				"property %s takes a plain value, not an expression: %s — `$Param.Attr` binds an attribute "+
					"property of a pluggable widget, and %s is not one", prop, expr.Text, prop)
		}
		return value, nil
	}

	pb := newAlterPageBuilder(ctx, moduleName, moduleID, "", mutator)
	where := fmt.Sprintf("widget `%s` property `%s`", widgetRef, scope.Key)

	if isExpr {
		// `$Param.Attr`: the attribute of a page or snippet parameter, whatever
		// encloses the widget — the AttributeRef plus a SourceVariable naming
		// the parameter, as CREATE writes it.
		name, rest, _ := namedObjectBinding(expr.Text)
		wv, entity, ok := pb.parameterVariable(name)
		if !ok {
			return nil, notAParameterError(where, expr.Text, name, pb.documentKind())
		}
		if err := pb.rejectAssociationAsAttribute(rest, entity, where); err != nil {
			return nil, err
		}
		return backend.AttributeBinding{Path: pb.resolveAttributePathForEntity(rest, entity), SourceVariable: &wv}, nil
	}

	attr, ok := value.(string)
	if !ok || attr == "" {
		return nil, mdlerrors.NewValidationf("%s takes an attribute name, `Module.Entity.Attr` or `$Param.Attr`", where)
	}
	if strings.ContainsAny(attr, "/$") {
		return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
			"%s: alter page set cannot bind the path `%s` — use `replace <widget> with …`, which rebuilds the "+
				"widget through the CREATE PAGE path", where, attr))
	}

	// The property's scope, by the rule CREATE uses: its linked datasource's
	// entity, or the enclosing data container's for an unlinked property.
	outer := sourceScopeEntity(ctx, scope.Enclosing, moduleName, moduleID)
	dsEntities := map[string]string{}
	if scope.DataSource != "" {
		if entity := sourceScopeEntity(ctx, scope.Linked, moduleName, moduleID); entity != "" {
			dsEntities[scope.DataSource] = entity
		}
	}
	bs := resolveBindingScope(scope.DataSource, true, outer, dsEntities, "")
	if err := misboundAttributeError(where, attr, bs, outer, dsEntities, pb.attributeIndex()); err != nil {
		return nil, err
	}
	if strings.Contains(attr, ".") {
		return backend.AttributeBinding{Path: attr}, nil
	}
	if bs.Entity == "" {
		return nil, mdlerrors.NewValidationf(
			"%s: cannot tell which entity `%s` belongs to — the property's data source names none that "+
				"can be read here. Qualify it (`Module.Entity.%s`) or bind a parameter's (`$Param.%s`)",
			where, attr, attr, attr)
	}
	if err := pb.rejectAssociationAsAttribute(attr, bs.Entity, where); err != nil {
		return nil, err
	}
	return backend.AttributeBinding{Path: pb.resolveAttributePathForEntity(attr, bs.Entity)}, nil
}

// sourceScopeEntity is the entity a data source supplies: the one it names, or
// its microflow's or nanoflow's return entity.
func sourceScopeEntity(ctx *ExecContext, s backend.SourceScope, moduleName string, moduleID model.ID) string {
	if s.Entity != "" {
		return s.Entity
	}
	return resolveDataSourceFlowEntity(ctx, moduleName, moduleID, s.Microflow, s.Nanoflow)
}
