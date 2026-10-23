// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `find $l by System.owner = $u` (and filter, and System.changedBy) builds when
// the list's entity stores the member, and fails otherwise (ako/mxcli#1629).
// Measured with mx check on 11.12.5:
//
//	the root stores it (own entity, System.FileDocument specialization,
//	specialization of an owning root)                       0 problems
//	the root does not (plain entity, its specialization)    CE0091 "No member selected."
//	System.Owner                                            CE1613
//
// So the verdict is the root's flag, judged as validateRetrieveSystemMemberPaths
// judges a constraint's `$var/System.owner`. Silence where the list's entity is
// not visible in the text or the chain cannot be walked.

// validateListOperationSystemMembers reports each find or filter by
// System.owner / System.changedBy that mxbuild rejects: a misspelled member, or
// a list whose entity does not store it.
func validateListOperationSystemMembers(ctx *ExecContext, params []ast.MicroflowParam, body []ast.MicroflowStatement, sc *scriptContext) []string {
	var index map[string]*domainmodel.Entity // loaded on first use: most flows have no such member
	loaded := false
	var errs []string
	types := newFlowObjectTypes(params)
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		defer types.observe(s)
		x, ok := s.(*ast.ListOperationStmt)
		if !ok || (x.Operation != ast.ListOpFind && x.Operation != ast.ListOpFilter) ||
			x.ByExpression || !ast.IsMemberEquality(x.Condition) {
			return
		}
		name, ok := listOperationFieldName(x.Condition.(*ast.BinaryExpr).Left)
		if !ok {
			return
		}
		canonical, ok := builtinSystemMemberAssociation(name)
		if !ok {
			return
		}
		entity := types.lists[x.InputVariable]
		if name != canonical {
			errs = append(errs, systemMemberSpellingRefusal(name, canonical, entity))
			return
		}
		if entity == "" {
			return
		}
		if !loaded {
			index, loaded = buildEntityIndex(ctx), true
		}
		member := strings.TrimPrefix(canonical, "System.")
		stores, root, known := scriptSystemMemberRoot(index, sc, entity, member)
		if stores || !known {
			return
		}
		holder := entity
		if root != entity {
			holder = entity + "'s root " + root
		}
		verb := "find"
		if x.Operation == ast.ListOpFilter {
			verb = "filter"
		}
		errs = append(errs, fmt.Sprintf(
			"%s $%s by %s: %s does not store %s — mxbuild rejects the member with CE0091 \"No member "+
				"selected.\". Store it first: %s",
			verb, x.InputVariable, canonical, holder, canonical, systemMemberStoreFix(root, member)))
	})
	return errs
}
