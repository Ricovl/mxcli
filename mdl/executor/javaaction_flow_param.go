// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A Java action parameter of type Microflow stores a document reference — the
// microflow's qualified name — and Studio Pro only lets you pick one. There is
// no dynamic form. Given a variable, mxcli wrote the TEXT `$Name` as that
// reference; check and exec both accepted it, and the project no longer loaded
// (mendixlabs/mxcli#1210, Mendix 11.14.0):
//
//	StorageLoadException: … Microflow parameter value … has an invalid value ''
//	for property Microflow. The text '$Name' is not a valid MicroflowIdentifier.
//
// That is a silent wrong write — an unloadable project, recoverable only from
// version control — so it is refused under every language version (ADR-0011).
// microflowParamArgRefusal is the one decision: the flow builder refuses the
// call with it, and reference validation (check -p, and exec's pre-flight)
// reports it from the same function, so the two cannot disagree.

// microflowIdentifierRe is the shape a stored microflow reference has.
var microflowIdentifierRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*$`)

// microflowParamArgRefusal returns why v cannot be written into the
// Microflow-typed parameter param of action, or "" when it can. `empty` is not
// passed here: it is the unbound marker, handled before the value is built.
func microflowParamArgRefusal(action, param string, v ast.Expression) string {
	switch e := v.(type) {
	case *ast.QualifiedNameExpr:
		return ""
	case *ast.LiteralExpr:
		if s, ok := e.Value.(string); ok && e.Kind == ast.LiteralString && microflowIdentifierRe.MatchString(s) {
			return ""
		}
	}
	return fmt.Sprintf("call java action %s: parameter %s is of type Microflow, which takes a microflow "+
		"name ('Module.Microflow'), not %s — Mendix has no dynamic microflow reference here, and the text "+
		"would be stored as the reference, leaving a project that no longer loads", action, param,
		expressionToString(v))
}

// microflowTypedParams names the parameters a CREATE JAVA ACTION declares as
// `Microflow` — the spelling astDataTypeToJavaActionParamType turns into a
// javaactions.MicroflowType.
func microflowTypedParams(params []ast.JavaActionParam) map[string]bool {
	out := map[string]bool{}
	for _, p := range params {
		if bareDataTypeName(p.Type) == "Microflow" {
			out[p.Name] = true
		}
	}
	return out
}

// microflowParamArgErrors applies microflowParamArgRefusal to the arguments of
// one call, given the action's Microflow-typed parameters.
func microflowParamArgErrors(ref codeActionCallRef, flowParams map[string]bool) []string {
	var out []string
	for _, a := range ref.args {
		if !flowParams[a.Name] || isEmptyJavaActionArgument(a.Value) {
			continue
		}
		if msg := microflowParamArgRefusal(ref.name, a.Name, a.Value); msg != "" {
			out = append(out, msg)
		}
	}
	return out
}
