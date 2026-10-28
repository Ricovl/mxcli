// SPDX-License-Identifier: Apache-2.0

// The two templates of a `call rest service` activity.
//
// A REST call's URL and its custom request body are each stored as a
// Microflows$StringTemplate: a text with {n} placeholders and one parameter
// expression per placeholder. MDL spells each of them two ways:
//
//	'https://x/{1}' with ({1} = $Id)           the template, as Studio Pro shows it
//	@M.BaseUrl + '/rates'                      an expression: the whole value
//
//	Body: template '{{"n": {1}}' with ({1} = $N)
//	Body: $Payload
//
// Studio Pro has no expression slot here. A value that is not a template is the
// template '{1}' with the value as its one parameter — what Studio Pro stores
// for a custom request template that only holds a variable, and what `log`,
// `show message`, `validation feedback` and `send email` already store for a
// message written as an expression.
//
// ako/mxcli#1444 (mendixlabs/mxcli#1546): the expression's SOURCE TEXT was
// stored as the template, with no parameters, so `Body: $Payload` sent the
// eight characters "$Payload" and `@M.BaseUrl + '/rates'` requested that text
// as a URL. `check`, `exec` and `mx check` all passed; mx check only warned that
// the variable was never used (CW0094).
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// restTemplateText returns the template text e states when it is one string
// literal. A literal that spans lines reaches here as source text kept around
// the literal (shouldPreserveExpressionSource); it is the same literal.
func restTemplateText(e ast.Expression) (string, bool) {
	if src, ok := e.(*ast.SourceExpr); ok {
		e = src.Expression
	}
	if lit, ok := e.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		return fmt.Sprintf("%v", lit.Value), true
	}
	return "", false
}

// restCallTemplate returns a REST call URL or custom body in the form it is
// stored: a string literal is the template text, filled by params; any other
// expression is the template '{1}' with the expression as its one parameter.
// An expression that also states params is returned unchanged —
// checkRestCallTemplatesStmt refuses it.
func restCallTemplate(text ast.Expression, params []ast.TemplateParam) (ast.Expression, []ast.TemplateParam) {
	if text == nil {
		return nil, params
	}
	if s, ok := restTemplateText(text); ok {
		return &ast.LiteralExpr{Kind: ast.LiteralString, Value: s}, params
	}
	if len(params) > 0 {
		return text, params
	}
	return &ast.LiteralExpr{Kind: ast.LiteralString, Value: "{1}"}, []ast.TemplateParam{{Index: 1, Value: text}}
}

// restCallAsTemplates returns s with its URL and custom body as restCallTemplate
// stores them, so the two spellings of one activity compare equal
// (declaredMatches). s itself is not changed.
func restCallAsTemplates(s ast.RestCallStmt) ast.RestCallStmt {
	s.URL, s.URLParams = restCallTemplate(s.URL, s.URLParams)
	if s.Body != nil && s.Body.Type == ast.RestBodyCustom {
		body := *s.Body
		body.Template, body.TemplateParams = restCallTemplate(body.Template, body.TemplateParams)
		s.Body = &body
	}
	return s
}

// checkRestCallTemplatesStmt reports a URL or custom body that is an
// expression and also states template parameters, or nil.
//
// This is the single decision: the executor calls it before building the
// action, and the check pass calls it through the microflow validator, so a
// script cannot pass one and fail the other.
func checkRestCallTemplatesStmt(s *ast.RestCallStmt) error {
	if s == nil {
		return nil
	}
	if s.URL != nil && len(s.URLParams) > 0 {
		if _, ok := restTemplateText(s.URL); !ok {
			return fmt.Errorf("call rest service: the URL is an expression, so `with (…)` has no template " +
				"to fill — the parameters fill the {n} placeholders of a URL written as a string.\n" +
				"  Write the URL as a template, '{1}/path' with ({1} = …), or as an expression " +
				"without `with (…)`")
		}
	}
	if s.Body != nil && s.Body.Type == ast.RestBodyCustom && s.Body.Template != nil && len(s.Body.TemplateParams) > 0 {
		if _, ok := restTemplateText(s.Body.Template); !ok {
			return fmt.Errorf("call rest service: the body is an expression, so `with (…)` has no template " +
				"to fill — the parameters fill the {n} placeholders of a template.\n" +
				"  Write Body: template '…{1}…' with ({1} = …), or Body: <expression> without `with (…)`")
		}
	}
	return nil
}

// checkRestCallTemplates surfaces checkRestCallTemplatesStmt as MDL-REST05.
func (v *microflowValidator) checkRestCallTemplates(stmt *ast.RestCallStmt) {
	if err := checkRestCallTemplatesStmt(stmt); err != nil {
		v.addViolation("MDL-REST05", linter.SeverityError, err.Error(),
			"See `mxcli syntax rest.call` for the template and expression forms")
	}
}
