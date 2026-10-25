// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A JavaScript action has no text-template type: StringTemplate(Text) and
// StringTemplate(Sql) are Java action parameter types (Studio Pro offers them
// nowhere else, and a nanoflow has no template value to pass). The grammar
// reads the type for both kinds of action and they share one converter, so
// `create javascript action` took it without complaint — main wrote it as a
// String parameter (describe printed `S: String`), and once the converter
// learned StringTemplate for Java actions it wrote a
// CodeActions$StringTemplateParameterType into the JavaScript action instead.
// mxbuild 11.13 accepts either at 0 errors, so nothing told the author.
//
// unsupportedJavaScriptActionTypes is the one decision: exec refuses the
// statement with it before anything is written, and check reports it as
// MDL-JSA01 (validateJavaScriptActionTypes), with no project needed. A
// document type (Document(…)) has no MDL spelling at all, so the parser
// already refuses it.
func unsupportedJavaScriptActionTypes(s *ast.CreateJavaScriptActionStmt) []string {
	refuse := func(where string, dt ast.DataType) string {
		if dt.Kind != ast.TypeStringTemplate {
			return ""
		}
		return fmt.Sprintf("javascript action %s: %s is typed %s, which only a Java action has — "+
			"use String for a JavaScript action, or make it a Java action to take a text template",
			s.Name.String(), where, codeActionTemplateSpelling(dt))
	}
	var out []string
	for _, p := range s.Parameters {
		if msg := refuse("parameter "+p.Name, p.Type); msg != "" {
			out = append(out, msg)
		}
	}
	if msg := refuse("the return type", s.ReturnType); msg != "" {
		out = append(out, msg)
	}
	return out
}

// codeActionTemplateSpelling is how a StringTemplate type is written:
// StringTemplate(Text) or StringTemplate(Sql).
func codeActionTemplateSpelling(dt ast.DataType) string {
	grammar := "Text"
	if strings.EqualFold(dt.TemplateContext, "sql") {
		grammar = "Sql"
	}
	return "StringTemplate(" + grammar + ")"
}

// validateJavaScriptActionTypes is MDL-JSA01: exec's refusal of a type a
// JavaScript action cannot have, reported at check time.
func validateJavaScriptActionTypes(s *ast.CreateJavaScriptActionStmt) []linter.Violation {
	var out []linter.Violation
	for _, msg := range unsupportedJavaScriptActionTypes(s) {
		out = append(out, linter.Violation{
			RuleID:   "MDL-JSA01",
			Severity: linter.SeverityError,
			Location: linter.Location{
				Module:       s.Name.Module,
				DocumentType: "javascript action",
				DocumentName: s.Name.Name,
			},
			Message:    msg,
			Suggestion: "Declare the parameter as String; text templates are a Java action type",
		})
	}
	return out
}
