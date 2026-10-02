// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateAssociationEndpoints resolves an association's FROM and TO entities
// the way exec does, so `check --references` refuses what exec would stop on
// (ako/mxcli#555).
//
// Until this, --references resolved only each endpoint's MODULE. Reported:
//
//	create or modify association ServiceCore.WorkOrder_Account
//	from ServiceCore.WorkOrder to System.Account
//
// passed as "All references valid", and exec then failed at that statement with
// `child entity not found: System.Account` — having already written the 18
// statements before it. `exec` is not transactional, so a reference check that
// is weaker than exec turns "run it and see" into a half-applied project.
//
// The lookup is findEntity, the same one createAssociation uses, so the two
// cannot disagree on what exists — including the System module, which is
// resolved through its virtual domain model (#610).
//
// An endpoint the script itself produces is skipped: created (entity, view or
// external entity), renamed or moved to that name, or in a module a bulk
// `create external entities` imports into — whose entity names come from the
// service's contract and are not in the statement.
func validateAssociationEndpoints(ctx *ExecContext, s *ast.CreateAssociationStmt, sc *scriptContext) error {
	for _, end := range []struct {
		role string
		side string
		qn   ast.QualifiedName
	}{{"parent", "FROM", s.Parent}, {"child", "TO", s.Child}} {
		qn := end.qn
		if qn.Name == "" {
			continue
		}
		if qn.Module == "" {
			// exec qualifies a bare endpoint with the association's module.
			qn.Module = s.Name.Module
		}
		if qn.Module == "" || sc.producesEntity(qn) {
			continue
		}
		if _, err := findEntity(ctx, qn.Module, qn.Name); err == nil {
			continue
		}
		msg := fmt.Sprintf("association %s: %s entity %s does not exist — exec would stop at this "+
			"statement (\"%s entity not found\") with every statement before it already applied",
			s.Name.String(), end.side, qn.String(), end.role)
		if hint := entityNameHint(qn); hint != "" {
			msg += ". " + hint
		}
		return mdlerrors.NewNotFoundMsg(end.role+" entity", qn.String(), msg)
	}
	return nil
}

// entityNameHint names the entity meant by a commonly mistaken one.
//
// System.Account is the reported case: it does not exist in Mendix 10 or 11.
// The user entity is System.User; Administration.Account is the specialization
// the Administration module adds.
func entityNameHint(qn ast.QualifiedName) string {
	if qn.Module == "System" && qn.Name == "Account" {
		return "The System user entity is System.User; Administration.Account (from the " +
			"Administration module) specializes it"
	}
	return ""
}

// producesEntity reports whether the script brings an entity of this name into
// existence, so its absence from the project means nothing yet.
func (sc *scriptContext) producesEntity(qn ast.QualifiedName) bool {
	if sc == nil {
		return false
	}
	name := qn.String()
	return sc.entities[name] || sc.indirectEntities[name] || sc.externalEntityModules[qn.Module]
}
