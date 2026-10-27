// SPDX-License-Identifier: Apache-2.0

// The message templates of `log`, `send email`, `show message` and
// `validation feedback`.
//
// Each stores its text as a template: a text with {n} placeholders and one
// parameter expression per placeholder (a Texts$Text or a StringTemplate).
// MDL spells the text two ways:
//
//	'value is {1}' with ({1} = $V)     the template, as Studio Pro shows it
//	$Msg                               an expression: the whole text
//
// An expression is stored as the template '{1}' with the expression as its one
// parameter. An expression followed by `with (…)` has no meaning Mendix can
// store: the expression is not a template, so there is no {n} for the
// parameters to fill, and a template's text cannot be an expression.
//
// ako/mxcli#1683: `log` and `send email` stored the expression's SOURCE TEXT as
// the template — `log info node 'P' $Msg with ({1} = 'x')` logged "$Msg",
// `Subject: $Subj with ({1} = '42')` sent the subject "$Subj" — and `show
// message` / `validation feedback` stored '{1}' with the expression and then
// the stated parameters, so 'x' became a {2} nothing reads and the message
// showed its own unfilled "{1}". check, exec and mx check passed all four.
// The same shape as `call rest service` (MDL-REST05, ako/mxcli#1444) and a Java
// action's text-template argument (MDL-JA02, ako/mxcli#1451).
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// messageTemplateRule is the rule ID for a message template written as an
// expression followed by `with (…)`.
const messageTemplateRule = "MDL-TEMPLATE01"

// messageTemplateRefusals returns why each template of stmt cannot be stored —
// one written as an expression that also states parameters — or nil. Only
// `log`, `send email`, `show message` and `validation feedback` have such a
// template; any other statement is not this function's business.
//
// This is the single decision: the flow builder calls it before building the
// action, and the check pass calls it through checkMessageTemplates, so a
// script cannot pass one and fail the other.
func messageTemplateRefusals(stmt ast.MicroflowStatement) []string {
	var out []string
	refuse := func(statement, what, write string, text ast.Expression, hasParams bool) {
		if text == nil || !hasParams {
			return
		}
		if _, ok := restTemplateText(text); ok {
			return
		}
		out = append(out, fmt.Sprintf("%s: the %s is an expression, so `with (…)` has no template to fill — "+
			"the parameters fill the {n} placeholders of a %s written as a string.\n"+
			"  Write %s, or the expression without `with (…)`", statement, what, what, write))
	}
	switch s := stmt.(type) {
	case *ast.LogStmt:
		refuse("log", "message", "the message as a template, 'value is {1}' with ({1} = …)",
			s.Message, len(s.Template) > 0)
	case *ast.ShowMessageStmt:
		refuse("show message", "message", "the message as a template, 'value is {1}' with ({1} = …)",
			s.Message, len(s.TemplateArgs) > 0)
	case *ast.ValidationFeedbackStmt:
		refuse("validation feedback", "message", "the message as a template, 'value is {1}' with ({1} = …)",
			s.Message, len(s.TemplateArgs) > 0)
	case *ast.SendEmailStmt:
		refuse("send email", "subject", "Subject: 'Order {1}' with ({1} = …)",
			s.Subject.Text, len(s.Subject.Params) > 0)
		if b := s.BodyText; b != nil {
			refuse("send email", "body", "Body: template '…{1}…' with ({1} = …)",
				b.Text, len(b.Params) > 0)
		}
		if b := s.BodyHTML; b != nil {
			refuse("send email", "HTML body", "HtmlBody: template '…{1}…' with ({1} = …)",
				b.Text, len(b.Params) > 0)
		}
	}
	return out
}

// checkMessageTemplates surfaces messageTemplateRefusals as MDL-TEMPLATE01 for
// every statement of body, nested and error-handler bodies included.
func (v *microflowValidator) checkMessageTemplates(body []ast.MicroflowStatement) {
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		for _, msg := range messageTemplateRefusals(s) {
			v.addViolation(messageTemplateRule, linter.SeverityError, msg,
				"Write the text as a string with {n} placeholders, or the expression alone without `with (…)`")
		}
	})
}

// messageTemplateViolations runs MDL-TEMPLATE01 alone over a flow that does not
// go through ValidateMicroflow — a rule (see setTargetViolations).
func messageTemplateViolations(docType, name string, body []ast.MicroflowStatement) []linter.Violation {
	v := &microflowValidator{mfName: name, docType: docType}
	v.checkMessageTemplates(body)
	return v.violations
}

// refuseMessageTemplates records messageTemplateRefusals as build errors, so
// exec refuses what check reports as MDL-TEMPLATE01.
func (fb *flowBuilder) refuseMessageTemplates(stmt ast.MicroflowStatement) {
	for _, msg := range messageTemplateRefusals(stmt) {
		fb.addError("%s", msg)
	}
}
