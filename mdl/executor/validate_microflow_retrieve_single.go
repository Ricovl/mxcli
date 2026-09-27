// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// retrieveSingleRule is the rule ID for "an object-range retrieve used as a list".
const retrieveSingleRule = "MDL-RETRIEVE01"

// checkRetrieveLimitOneAsList flags a variable that a retrieve bound to a single
// OBJECT and a later statement uses as a LIST.
//
// The object range is Mendix's "First object" range, written `retrieve … first`
// in every language version, and `retrieve … limit 1` in a script without the
// `mdl 1;` header, where `limit 1` keeps its alpha meaning (ako/mxcli#734). The
// visitor resolves both to RetrieveStmt.First, which is what the writer stores
// too, so the rule keys on it and never on limit text: a check that disagrees
// with the writer it describes is worse than no check.
//
// Before #734 this rule was the only sign of the object: describe re-emitted
// `limit 1` for it, so an object retrieve and a list retrieve were identical text,
// and the first thing the author saw was CE0097 from mxbuild — inside a
// .test.mdl, not even that (mendixlabs/mxcli#1103). describe now prints `first`
// and an mdl 0 `limit 1` warns MDL-V1-LIMIT1, so the spelling says it; the rule
// stays for the mistake that is left, treating an object as a list, which
// check --references does not otherwise catch before the build.
func (v *microflowValidator) checkRetrieveLimitOneAsList(body []ast.MicroflowStatement) {
	// single holds the variables currently bound to one object by an
	// object-range retrieve. Maintained in statement order so a rebinding
	// clears it: a name reused for a real list further down is not this rule's
	// business.
	single := map[string]bool{}

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if name, op := listUseOf(s); name != "" && single[name] {
			// Measured on 11.13: a loop reports CE0100, a list activity CE0097.
			rejection := fmt.Sprintf("CE0097 \"The selected '%s' variable must be of type List\"", name)
			if op == "a loop" {
				rejection = fmt.Sprintf("CE0100 \"'%s' is of type …, but should be of type List\"", name)
			}
			v.addViolation(retrieveSingleRule, linter.SeverityError,
				fmt.Sprintf("$%s was retrieved as a single object (`first`, which is also what `limit 1` "+
					"means in a script without the `mdl 1;` header), not a list, so %s cannot take it — "+
					"mxbuild rejects this with %s.", name, op, rejection),
				fmt.Sprintf("Retrieve a list instead (no range, or under `mdl 1;` `limit 1` for a list of "+
					"one) and keep %s, or keep the object range and use $%s as the object it already is.",
					op, name))
		}

		// Rebinding first, so a statement that both consumes and produces the
		// name is judged on what it consumed.
		for _, p := range statementProducedVars(s) {
			delete(single, p.name)
		}
		if r, ok := s.(*ast.RetrieveStmt); ok && r.First && r.StartVariable == "" && r.Variable != "" {
			single[r.Variable] = true
		}
	})
}

// listUseOf reports the list variable a statement consumes, and a phrase naming
// what consumes it. ("", "") when the statement takes no list.
func listUseOf(s ast.MicroflowStatement) (string, string) {
	switch st := s.(type) {
	case *ast.ListOperationStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
		if st.SecondVariable != "" {
			return st.SecondVariable, st.Operation.String() + "()"
		}
	case *ast.AggregateListStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
	case *ast.LoopStmt:
		if st.ListVariable != "" {
			return st.ListVariable, "a loop"
		}
	case *ast.AddToListStmt:
		if st.List != "" {
			return st.List, "ADD … TO"
		}
	case *ast.RemoveFromListStmt:
		if st.List != "" {
			return st.List, "REMOVE … FROM"
		}
	}
	return "", ""
}
