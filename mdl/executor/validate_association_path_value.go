// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// loopOverPathRule reports a loop whose list is written as a path.
const loopOverPathRule = "MDL094"

// checkAssociationPathValues flags an association path written where a flow
// needs a whole value or a list variable (mendixlabs/mxcli#199). Measured on
// mxbuild 11.12.1, one microflow each into a fresh project:
//
//	return $Parent/M.Child_Parent            (reverse, list)        CE0117
//	return $Child/M.Child_Parent             (forward, object)      CE0117
//	return $Child/M.Child_Tag                (reference set, list)  CE0117
//	return $Child/M.Child_Parent/M.Parent    (entity step)          0 errors
//	return $Child/M.Child_Tag/M.Tag          (entity step, list)    0 errors
//	loop $c in $Parent/M.Child_Parent                               CE0099
//
// A return value is the shape MDL049 refuses as a call argument, so it is the
// same rule with the same exemptions. A loop's list is a variable name in the
// model — there is nowhere to store a path, with or without the entity step —
// so any path there is refused (MDL094). `retrieve $Children from
// $Parent/M.Child_Parent;` is the activity both mean.
func (v *microflowValidator) checkAssociationPathValues(body []ast.MicroflowStatement) {
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		switch st := s.(type) {
		case *ast.ReturnStmt:
			if exprIsAssociationObjectPath(st.Value) {
				path := microflowExprSource(st.Value)
				v.addViolation("MDL049", linter.SeverityError,
					fmt.Sprintf("return %s returns a bare association path, which Mendix rejects as a value "+
						"(CE0117 \"Error(s) in expression\" at the end event)", path),
					fmt.Sprintf("Retrieve over the association and return the variable — `retrieve $Result from %s;` "+
						"then `return $Result;` — or, for a reference followed from its owner, name the target entity "+
						"after the association: `%s/<Module.TargetEntity>`.", path, path))
			}
		case *ast.LoopStmt:
			if st.ListPath != "" {
				v.addViolation(loopOverPathRule, linter.SeverityError,
					fmt.Sprintf("loop $%s in %s iterates over a path, but a loop iterates over a list variable — "+
						"the path cannot be stored and the loop would be written with nothing to iterate "+
						"(CE0099 \"The 'Iterate over' property is required.\")", st.LoopVariable, st.ListPath),
					fmt.Sprintf("Retrieve the list first: `retrieve $%sList from %s;` then `loop $%s in $%sList`.",
						st.LoopVariable, st.ListPath, st.LoopVariable, st.LoopVariable))
			}
		}
	})
}
