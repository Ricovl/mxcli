// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1683: a `log` message or a `send email` subject written as an
// expression followed by `with (…)` was stored as the expression's SOURCE
// TEXT: `log info node 'P' $Msg with ({1} = 'x')` logged the five characters
// "$Msg", `Subject: $Subj with ({1} = '42')` sent the subject "$Subj". `show
// message` and `validation feedback` made the same statement into the template
// '{1}' with the expression as parameter 1 and the stated parameters after it,
// so 'x' became a {2} no placeholder reads and the message showed its own
// unfilled "{1}". check, exec and mx check all passed every one of them.
//
// A template is a text with {n} placeholders; an expression has none for
// `with (…)` to fill, and Mendix cannot store an expression as a template's
// text. The shape ako/mxcli#1444 refused for `call rest service` (MDL-REST05)
// and ako/mxcli#1451 for a Java action's text-template argument (MDL-JA02).

func messageTemplateMicroflow(stmt string) string {
	return "create microflow M.Probe ($Msg: String, $P: System.User)\nbegin\n  " + stmt + ";\nend;\n"
}

func sendEmailWith(settings string) string {
	return "send email (\n    From: 'a@example.com', To: 'b@example.com',\n    " + settings +
		",\n    Host: 'smtp.example.com', Port: 25,\n  )"
}

func TestMessageTemplate_ExpressionWithParametersIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, stmt string
		refused    bool
	}{
		// The two reported statements.
		{"log", `log info node 'Probe' $Msg with ({1} = 'x')`, true},
		{"email subject", sendEmailWith(`Subject: $Msg with ({1} = '42')`), true},
		// The same shape on the other templates of those statements and of
		// the two that store a message the same way.
		{"email plain body", sendEmailWith(`Subject: 'S', Body: $Msg with ({1} = '42')`), true},
		{"email html body", sendEmailWith(`Subject: 'S', HtmlBody: $Msg + '!' with ({1} = '42')`), true},
		{"show message", `show message $Msg with ({1} = 'x')`, true},
		{"show message, deprecated objects", `show message $Msg objects ['x']`, true},
		{"validation feedback", `validation feedback $P/Name message $Msg with ({1} = 'x')`, true},
		{"log, any expression", `log info $Msg + ' {1}' with ({1} = 'x')`, true},
		// Controls: a template written as a string, and an expression without
		// parameters — stored as '{1}' with it as parameter 1 — stay as they were.
		{"control: log template", `log info node 'Probe' 'value is {1}' with ({1} = $Msg)`, false},
		{"control: log expression", `log info node 'Probe' $Msg`, false},
		{"control: log with stack trace", `log error $Msg with stacktrace`, false},
		{"control: email subject template", sendEmailWith(`Subject: 'Order {1}' with ({1} = $Msg)`), false},
		{"control: email subject expression", sendEmailWith(`Subject: $Msg`), false},
		{"control: email body template", sendEmailWith(`Subject: 'S', Body: template 'Hi {1}' with ({1} = $Msg)`), false},
		{"control: show message template", `show message 'Hi {1}' with ({1} = $Msg)`, false},
		{"control: show message expression", `show message $Msg`, false},
		{"control: validation feedback template", `validation feedback $P/Name message 'Bad {1}' with ({1} = $Msg)`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			create := firstCreateMicroflow(t, messageTemplateMicroflow(c.stmt))

			var rule string
			for _, v := range ValidateMicroflow(create) {
				if v.RuleID == messageTemplateRule {
					rule = v.Message
				}
			}
			if (rule != "") != c.refused {
				t.Errorf("check: %s reported = %v, want %v (%q)", messageTemplateRule, rule != "", c.refused, rule)
			}

			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
				measurer: &layoutMeasurer{}, varTypes: map[string]string{"P": "System.User"},
				declaredVars: map[string]string{"Msg": "String"}}
			fb.buildFlowGraph(create.Body, create.ReturnType)
			var execErr string
			for _, e := range fb.GetErrors() {
				if strings.Contains(e, "has no template to fill") {
					execErr = e
				}
			}
			if (execErr != "") != c.refused {
				t.Errorf("exec: refused = %v, want %v (errors: %q)", execErr != "", c.refused, fb.GetErrors())
			}
			if c.refused && rule != execErr {
				t.Errorf("check and exec disagree:\n check: %q\n exec:  %q", rule, execErr)
			}
		})
	}
}

// The refusal holds in every flow `check` validates that can hold one of these
// statements, not only a microflow: a nanoflow logs and shows messages, a rule
// logs, and an `alter … insert` adds the statement to a stored flow.
func TestMessageTemplate_RefusedInEveryFlowKind(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"microflow", messageTemplateMicroflow(`log info $Msg with ({1} = 'x')`)},
		{"nanoflow", "create nanoflow M.Probe ($Msg: String)\nbegin\n  show message $Msg with ({1} = 'x');\nend;\n"},
		{"rule", "create rule M.Probe ($Msg: String) returns Boolean\nbegin\n  log info $Msg with ({1} = 'x');\n  return true;\nend;\n"},
		{"alter fragment", "alter microflow M.Probe {\n  insert after $Result begin\n    log info $Msg with ({1} = 'x');\n  end;\n};\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var found bool
			for _, v := range ValidateProgram(parseMDL(t, c.src), "") {
				if v.RuleID == messageTemplateRule {
					found = true
				}
			}
			if !found {
				t.Errorf("%s not reported for a %s", messageTemplateRule, c.name)
			}
		})
	}
}

// messageTemplateRefusals is the one decision; the statements it does not know
// are not its business.
func TestMessageTemplateRefusals_OtherStatementsPass(t *testing.T) {
	if got := messageTemplateRefusals(&ast.ReturnStmt{}); len(got) != 0 {
		t.Errorf("return: %q, want none", got)
	}
	if got := messageTemplateRefusals(nil); len(got) != 0 {
		t.Errorf("nil: %q, want none", got)
	}
}

// A body's string literal that spans lines reaches the builder kept with its
// source (shouldPreserveExpressionSource). With parameters it is still the
// template, and is stored as its value: the source — quotes and all — was
// stored as the template text, the same fallback the refusal closes for an
// expression.
func TestEmailBody_MultiLineLiteralWithParametersIsTheTemplateText(t *testing.T) {
	src := messageTemplateMicroflow(sendEmailWith("Subject: 'S', Body: 'line one\nline two {1}' with ({1} = $Msg)"))
	a := sendEmailActionOf(t, buildMicroflowFromMDL(t, src))
	if want := "line one\nline two {1}"; a.BodyPlainText.Text != want {
		t.Errorf("body template = %q, want %q", a.BodyPlainText.Text, want)
	}
	if want := []string{"$Msg"}; len(a.BodyPlainText.Parameters) != 1 || a.BodyPlainText.Parameters[0] != want[0] {
		t.Errorf("body parameters = %q, want %q", a.BodyPlainText.Parameters, want)
	}
}
