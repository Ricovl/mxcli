// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A Java or JavaScript action's return type is a CodeActions$Type: Boolean,
// Integer/Long, Decimal, String, DateTime, an enumeration, an entity, a list,
// a type parameter, or Void. The grammar reads any data type after `returns`,
// and the return-type converter turned every kind it did not handle into
// Boolean — so `returns StringTemplate(Text)` passed check and exec and built a
// Boolean action. A text template is a parameter type only
// (CodeActions$StringTemplateParameterType is not a CodeActions$Type), and a
// unit patched to return one does not load in mxbuild 11.13: "Type
// Mendix.Modeler.CodeActions.StringTemplateParameterType does not contain a
// constructor with a parameter of type Mendix.Modeler.JavaActions.JavaAction".

// codeActionReturnTypeRefusal is the error the return-type converter gives for
// a kind it does not handle.
func codeActionReturnTypeRefusal(dt ast.DataType) error {
	switch dt.Kind {
	case ast.TypeStringTemplate:
		return fmt.Errorf("the return type is %s, which is a parameter type only — a Java action cannot "+
			"return a text template; return String", codeActionTemplateSpelling(dt))
	case ast.TypeEntityTypeParam:
		return fmt.Errorf("the return type is entity <%s>, which is a parameter type only (it selects an "+
			"entity) — return the type parameter %s, or a list of it", dt.TypeParamName, dt.TypeParamName)
	}
	return fmt.Errorf("the return type is %s, which a Java or JavaScript action cannot return — return "+
		"Boolean, Integer, Long, Decimal, String, DateTime, an enumeration, an entity, a list, a type "+
		"parameter or Void", dt.Kind.String())
}

// javaActionReturnTypeError is exec's verdict on a return type, for check: a
// type parameter, or a list of one, is resolved before the converter, as in
// execCreateJavaAction and execCreateJavaScriptAction; anything else is
// whatever the converter says. One decision, so the two cannot disagree.
func javaActionReturnTypeError(dt ast.DataType, typeParams []string) error {
	declared := make(map[string]bool, len(typeParams))
	for _, tp := range typeParams {
		declared[tp] = true
	}
	if isTypeParamRef(dt, declared) {
		return nil
	}
	if dt.Kind == ast.TypeListOf && dt.EntityRef != nil && dt.EntityRef.Module == "" && declared[dt.EntityRef.Name] {
		return nil
	}
	_, err := astDataTypeToJavaActionReturnType(dt)
	return err
}

// codeActionReturnTypeViolation is MDL-JA03 for one action, or nil.
func codeActionReturnTypeViolation(kind string, name ast.QualifiedName, dt ast.DataType, typeParams []string) *linter.Violation {
	err := javaActionReturnTypeError(dt, typeParams)
	if err == nil {
		return nil
	}
	return &linter.Violation{
		RuleID:   "MDL-JA03",
		Severity: linter.SeverityError,
		Location: linter.Location{
			Module:       name.Module,
			DocumentType: kind,
			DocumentName: name.Name,
		},
		Message:    fmt.Sprintf("%s '%s': %v", kind, name.String(), err),
		Suggestion: "See `mxcli syntax java-action.create` for the types an action returns",
	}
}

// validateJavaActionReturnType is MDL-JA03 for a Java action: exec's refusal of
// a return type the converter does not handle, reported at check time.
func validateJavaActionReturnType(s *ast.CreateJavaActionStmt) []linter.Violation {
	if v := codeActionReturnTypeViolation("java action", s.Name, s.ReturnType, s.TypeParameters); v != nil {
		return []linter.Violation{*v}
	}
	return nil
}
