// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/mendixlabs/mxcli/mdl/ast"

// flowActionGates are the gates of the microflow and nanoflow activities whose
// storage $Types Mendix added after 9.0 and the converters write with no older
// shape (ako/mxcli#1811), each at its types' introduction in mendixmodelsdk
// 4.117.0. The write refuses each type below it (canon.UndeclaredTypeError);
// these name the statement first, in `create microflow|nanoflow` and in what
// `alter microflow|nanoflow` splices in, wherever it sits (bodyContains).
// Newest first, so a body holding several is refused naming the one whose
// version covers the rest. A statement a nanoflow cannot hold is refused there
// by the nanoflow validation; matching it here as well costs nothing.
var flowActionGates = []typeGate{
	{
		versionGate: versionGate{"microflows", "transform_json", "transform … with (a data transformer)",
			"this version has no Microflows$TransformJsonAction: map the JSON with an import mapping, or upgrade the project"},
		types: []string{"Microflows$TransformJsonAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.TransformJsonStmt); return ok },
	},
	{
		// The body variable is written for an operation whose body is an export
		// mapping, or one the builder cannot find; for a JSON, template or file
		// body the body lives on the operation and the clause is dropped. Gated
		// on the clause as written, which is what an older project refuses.
		versionGate: versionGate{"microflows", "send_rest_request_body", "send rest request … body $Variable",
			"this version has no Microflows$BodyVariable: leave the body to the operation, or upgrade the project"},
		types:      []string{"Microflows$BodyVariable"},
		properties: []string{"Microflows$RestOperationCallAction.BodyVariable"},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.SendRestRequestStmt)
			return ok && s.BodyVariable != ""
		},
	},
	{
		versionGate: versionGate{"microflows", "send_rest_request_output", "$Result = send rest request …",
			"this version has no Microflows$OutputVariable: call the operation without storing its result, or upgrade the project"},
		types:      []string{"Microflows$OutputVariable"},
		properties: []string{"Microflows$RestOperationCallAction.OutputVariable"},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.SendRestRequestStmt)
			return ok && s.OutputVariable != ""
		},
	},
	{
		// A parameter is a path parameter mapping below 11.0: an operation
		// there has no query parameters (Rest$QueryParameter is 11.0), and
		// the query parameter mapping, also 11.0, is written only for one.
		versionGate: versionGate{"microflows", "send_rest_request_parameters", "send rest request … with ( … )",
			"this version has no Microflows$RestOperationParameterMapping: upgrade the project"},
		types:      []string{"Microflows$RestOperationParameterMapping"},
		properties: []string{"Microflows$RestOperationCallAction.ParameterMappings"},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.SendRestRequestStmt)
			return ok && len(s.Parameters) > 0
		},
	},
	{
		// mx 9.24.30 loads the action (CE7053 "No REST operation specified",
		// not a load failure), but the consumed REST service it calls,
		// Rest$ConsumedRestService, is 10.1 too: a 9.24 author cannot set one up.
		versionGate: versionGate{"microflows", "send_rest_request", "send rest request",
			"this version has no consumed REST service (Rest$ConsumedRestService) or operation call " +
				"(Microflows$RestOperationCallAction): use `call rest`, or upgrade the project"},
		types: []string{"Microflows$RestOperationCallAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.SendRestRequestStmt); return ok },
	},
	{
		versionGate: versionGate{"microflows", "call_external_action_arguments", "call external action … ( arguments )",
			"this version has no Microflows$ExternalActionParameterMapping: upgrade the project"},
		types:      []string{"Microflows$ExternalActionParameterMapping"},
		properties: []string{"Microflows$CallExternalAction.ParameterMappings"},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.CallExternalActionStmt)
			return ok && len(s.Arguments) > 0
		},
	},
	{
		versionGate: versionGate{"microflows", "call_external_action", "call external action",
			"this version has no Microflows$CallExternalAction: upgrade the project"},
		types: []string{"Microflows$CallExternalAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.CallExternalActionStmt); return ok },
	},
	{
		versionGate: versionGate{"microflows", "list_range", "range (a list operation)",
			"this version has no Microflows$ListRange: retrieve the range from the database with limit and offset, or upgrade the project"},
		types: []string{"Microflows$ListRange"},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.ListOperationStmt)
			return ok && s.Operation == ast.ListOpRange
		},
	},
	{
		versionGate: versionGate{"microflows", "list_find_filter_by_expression", "find / filter by expression (a list operation)",
			"this version has no Microflows$FindByExpression or FilterByExpression: find or filter by a member " +
				"(`find($List, Member = value)`), or upgrade the project"},
		types: []string{"Microflows$FindByExpression", "Microflows$FilterByExpression"},
		match: writesListExpressionOperation,
	},
}

// writesListExpressionOperation reports a find or filter the builder writes as
// Microflows$FindByExpression / FilterByExpression: one written with `where`,
// or one whose condition is not `Member = value` (listAttributeOperation).
// A `find` the builder rewrites into a String find() call writes no list
// operation (stringOverloadedListOp); flowGates excludes those it can see.
func writesListExpressionOperation(st ast.MicroflowStatement) bool {
	s, ok := st.(*ast.ListOperationStmt)
	if !ok || (s.Operation != ast.ListOpFind && s.Operation != ast.ListOpFilter) {
		return false
	}
	return s.ByExpression || !ast.IsMemberEquality(s.Condition)
}

// flowGates are the gates of the activities in a microflow or nanoflow body
// that flowActionGates names. params are the flow's parameters when the
// statement declares them (create); an alter's fragment has none.
func flowGates(params []ast.MicroflowParam, body []ast.MicroflowStatement) []versionGate {
	stringVars := stringVariables(params, body)
	var gates []versionGate
	for _, g := range flowActionGates {
		match := g.match
		if g.name == "list_find_filter_by_expression" {
			match = func(st ast.MicroflowStatement) bool {
				s, ok := st.(*ast.ListOperationStmt)
				if ok && stringOverloadedListOp(s.Operation) && stringVars[s.InputVariable] {
					return false // a String find(): a change variable, no list operation
				}
				return writesListExpressionOperation(st)
			}
		}
		if bodyContains(body, match) {
			gates = append(gates, g.versionGate)
		}
	}
	return gates
}

// stringVariables are the names the builder knows as String when it reaches a
// list operation: a String parameter, or a variable declared String anywhere in
// the body — the declaredVars a `find` on a String is rewritten from.
func stringVariables(params []ast.MicroflowParam, body []ast.MicroflowStatement) map[string]bool {
	out := map[string]bool{}
	for _, p := range params {
		if p.Type.Kind == ast.TypeString {
			out[p.Name] = true
		}
	}
	bodyContains(body, func(st ast.MicroflowStatement) bool {
		if d, ok := st.(*ast.DeclareStmt); ok && d.Type.Kind == ast.TypeString {
			out[d.Variable] = true
		}
		return false
	})
	return out
}
