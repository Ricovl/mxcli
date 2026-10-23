// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A retrieve constraint reading System.owner or System.changedBy off a
// VARIABLE — `[id = $o/System.owner]` — passed `check -p` and exec, and mxbuild
// then reported CE0161 "Error(s) in XPath constraint." whenever $o's entity
// does not store the member (ako/mxcli#1306, the follow-up on
// mendixlabs/mxcli#1358). It is the rewrite MDL-RETRIEVE02 suggests, so the
// shape is easy to reach.
//
// #641's rule judges `System.owner` on the retrieved entity itself; nothing
// judged it behind a variable. Measured with mx check on 11.12.5:
//
//	$o: entity storing owner and changedBy   [id = $o/System.owner]      clean
//	                                         [id = $o/System.changedBy]  clean
//	$o: System.FileDocument specialization   [id = $o/System.owner]      clean
//	$o: specialization of an owning root     [id = $o/System.owner]      clean
//	$o: entity storing neither               [id = $o/System.owner]      CE0161
//	                                         [id = $o/System.changedBy]  CE0161
//	$o: specialization of that entity        [id = $o/System.owner]      CE0161
//	$p = create <that entity> (…)            [id = $p/System.owner]      CE0161
//
// So the verdict is the root's flag, as everywhere else system members are
// judged (scriptSystemMemberRoot). Silence wherever the variable's entity is
// not visible in the text (flowObjectTypes) or the chain cannot be walked.

// systemMemberPathRe matches `$var/System.owner` and `$var/System.changedBy`.
// The case is significant: `System.Owner` is a different name (CE1613).
var systemMemberPathRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)/System\.(owner|changedBy)\b`)

// validateRetrieveSystemMemberPaths reports each database retrieve whose
// constraint reads System.owner / System.changedBy off a variable whose entity
// does not store it.
func validateRetrieveSystemMemberPaths(ctx *ExecContext, params []ast.MicroflowParam, body []ast.MicroflowStatement, sc *scriptContext) []string {
	var index map[string]*domainmodel.Entity // loaded on first use: most flows have no such path
	loaded := false
	var errs []string
	types := newFlowObjectTypes(params)
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		defer types.observe(s)
		r, ok := s.(*ast.RetrieveStmt)
		if !ok || r.StartVariable != "" || r.Source.Module == "" || r.Where == nil {
			return
		}
		entityQN := r.Source.String()
		constraint := blankXPathLiterals(retrieveXPathConstraint(r.Where, entityQN))
		for _, m := range systemMemberPathRe.FindAllStringSubmatch(constraint, -1) {
			variable, member := m[1], m[2]
			varEntity, ok := types.objects[variable]
			if !ok {
				continue
			}
			if !loaded {
				index, loaded = buildEntityIndex(ctx), true
			}
			stores, root, known := scriptSystemMemberRoot(index, sc, varEntity, member)
			if stores || !known {
				continue
			}
			holder := varEntity
			if root != varEntity {
				holder = varEntity + "'s root " + root
			}
			errs = append(errs, fmt.Sprintf(
				"retrieve from %s: the constraint reads $%s/System.%s, but %s does not store System.%s — mxbuild "+
					"rejects the constraint (CE0161 \"Error(s) in XPath constraint\"). Store it first: %s",
				entityQN, variable, member, holder, member, systemMemberStoreFix(root, member)))
		}
	})
	return errs
}
