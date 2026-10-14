// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
)

// A DataGrid 2 column's `Attribute:` was never resolved (mendixlabs/mxcli#1472).
// The scope rule (validatePluggableAttributeScopes) only asks whether a name
// belongs to ANOTHER entity in scope; a name that belongs to none passed check
// and exec, and mxbuild answered — measured on 11.13.0 —
//
//	CE1613 "The selected attribute 'Administration.Account.NoSuchColumnAttr'
//	        no longer exists." at Columns (1/1) of data grid 2 'dg'
//
// A filter in the column's header is NOT judged, though the issue names it: it
// filters the column's own attribute, and the column builder does not write
// the filter's `Attribute:` at all — a valid one and a misspelt one alike are
// dropped (describe prints a bare `textfilter tf1`), and mxbuild reports
// nothing for either. Refusing the misspelt one would refuse a script that
// builds.
//
// The grid's entity is only taken from a `database` source, where it is the
// statement's own text. A flow, association or selection source leaves the
// columns unjudged: could-not-establish stays silent. So does a path or a
// qualified name, a system member (stored as a flag, not an attribute), an
// entity whose chain the index does not hold, and an entity the script alters,
// imports or produces without declaring its members.

// columnAttrJudge answers "is attr provably not an attribute of entity".
type columnAttrJudge func(entity, attr string) bool

// newColumnAttrJudge builds the judge over the stored domain model, overlaid
// with the entities this script declares.
func newColumnAttrJudge(ctx *ExecContext, sc *scriptContext) columnAttrJudge {
	stored := checkAttributeIndex(ctx, nil)
	overlay := checkAttributeIndex(ctx, sc)
	return func(entity, attr string) bool {
		if entity == "" || attr == "" || strings.ContainsAny(attr, "./$ ") || isSystemMemberName(attr) {
			return false
		}
		if sc != nil {
			if sc.alteredEntities[entity] || sc.indirectEntities[entity] ||
				sc.externalEntityModules[qualifiedNameModule(entity)] {
				return false
			}
			if sc.entities[entity] && sc.entityAttrs[entity] == nil {
				return false // a view or external entity: members not recorded
			}
		}
		// A CREATE OR MODIFY keeps stored members it does not restate, so a
		// member either side declares is enough.
		if found, _ := stored.declares(entity, attr); found {
			return false
		}
		found, known := overlay.declares(entity, attr)
		return !found && known
	}
}

// isSystemMemberName reports the system members an entity stores as a flag
// rather than as an attribute, which the attribute index therefore lacks.
func isSystemMemberName(attr string) bool {
	switch strings.ToLower(attr) {
	case "createddate", "changeddate", "owner", "changedby", "id":
		return true
	}
	return false
}

// datagridColumnAttrErrors walks a widget tree and reports the DataGrid 2
// column attributes that are not attributes of the grid's database-source
// entity. columnsEntity is the entity of the grid that
// top-level `column` widgets belong to (ALTER … INSERT INTO a stored grid),
// or "" when the widgets are not columns of a known grid.
func datagridColumnAttrErrors(widgets []*ast.WidgetV3, columnsEntity string, missing columnAttrJudge) []string {
	var errs []string
	report := func(where, attr, entity string) {
		errs = append(errs, fmt.Sprintf(
			"%s: `%s` is not an attribute of %s, the entity of the grid's data source — "+
				"mxbuild reports CE1613 \"The selected attribute '%s.%s' no longer exists.\"",
			where, attr, entity, entity, attr))
	}
	// gridEntity is the entity the widgets in ws are columns of, or "".
	var walk func(ws []*ast.WidgetV3, gridName, gridEntity string)
	walk = func(ws []*ast.WidgetV3, gridName, gridEntity string) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			kind := strings.ToLower(w.Type)
			switch {
			case kind == "datagrid":
				entity := ""
				if ds := w.GetDataSource(); ds != nil && ds.Type == "database" {
					entity = ds.Reference
				}
				walk(w.Children, w.Name, entity)
				continue
			case kind == "column" && gridEntity != "":
				attr := w.GetAttribute()
				if missing(gridEntity, attr) {
					report(fmt.Sprintf("datagrid `%s` column `%s`", gridName, columnLabel(w)), attr, gridEntity)
				}
			}
			// Below a column, or any other widget, the grid's reach ends: the
			// content is judged by the rules for the widgets it holds.
			walk(w.Children, "", "")
		}
	}
	walk(widgets, "", columnsEntity)
	return errs
}

// alterColumnsEntity is the entity a stored grid gives the columns an ALTER
// inserts into it, when that entity is the grid's own non-flow source — the
// entity exec builds the columns against (alterEntityContext). "" otherwise.
func alterColumnsEntity(mutator backend.PageMutator, grid string) string {
	if mf, nf := mutator.EnclosingDataSourceFlow(grid, true); mf != "" || nf != "" {
		return ""
	}
	return mutator.EnclosingEntityForChildren(grid)
}
