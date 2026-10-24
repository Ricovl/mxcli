// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1444 (mendixlabs/mxcli#1546): `call rest service … (Body: $Payload)`
// was stored as the custom request template '$Payload' with no parameters, so
// the request body was the eight characters "$Payload". `check`, `exec` and
// `mx check` all passed — mx check only warned that the variable was unused
// (CW0094). The deprecated `body $Payload` clause built the same statement.
//
// A REST call's URL and its custom body are both a Microflows$StringTemplate:
// a text with {n} placeholders and a parameter expression per placeholder.
// Studio Pro has no "expression" body; a body that is just a value is the
// template '{1}' with that value as parameter 1, which is what `log`,
// `show message`, `validation feedback` and `send email` already store for a
// message written as an expression. The URL had the same defect: an
// expression URL (`@M.BaseUrl + '/rates'`) became its own source text.

// restCallFromMDL parses a microflow holding one REST call and builds it.
func restCallFromMDL(t *testing.T, src string) *microflows.RestCallAction {
	t.Helper()
	oc := buildMicroflowFromMDL(t, src)
	for _, obj := range oc.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if rc, ok := act.Action.(*microflows.RestCallAction); ok {
			return rc
		}
	}
	t.Fatalf("no REST call built from:\n%s", src)
	return nil
}

func firstCreateMicroflow(t *testing.T, src string) *ast.CreateMicroflowStmt {
	t.Helper()
	for _, stmt := range parseMDL(t, src).Statements {
		if c, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			return c
		}
	}
	t.Fatal("no CREATE MICROFLOW in the parsed program")
	return nil
}

func restCallMicroflow(call string) string {
	return "create microflow M.Send ($Payload: String) returns String\nbegin\n" +
		"  $Response = " + call + ";\n  return $Response;\nend;\n"
}

func customBody(t *testing.T, rc *microflows.RestCallAction) *microflows.CustomRequestHandling {
	t.Helper()
	body, ok := rc.RequestHandling.(*microflows.CustomRequestHandling)
	if !ok {
		t.Fatalf("RequestHandling is %T, want *microflows.CustomRequestHandling", rc.RequestHandling)
	}
	return body
}

func TestRestCallBody_ExpressionIsTheTemplatesParameter(t *testing.T) {
	for _, c := range []struct {
		name, call   string
		wantTemplate string
		wantParams   []string
	}{
		// The reported statement.
		{"settings list", `call rest service post 'https://example.com/orders' (
    Headers: ('Content-Type': 'application/json'),
    Body: $Payload,
    Timeout: 30,
  ) returns String`, "{1}", []string{"$Payload"}},
		// The deprecated clause built the same statement, and the same bug.
		{"deprecated clause", `call rest service post 'https://example.com/orders'
    body $Payload
    timeout 30
    returns String`, "{1}", []string{"$Payload"}},
		// Any expression, not only a variable.
		{"an expression", `call rest service post 'https://example.com/orders' (
    Body: '{"number": "' + $Payload + '"}',
  ) returns String`, "{1}", []string{`'{"number": "' + $Payload + '"}'`}},
		// Controls: what was right stays as it was.
		{"control: the explicit template", `call rest service post 'https://example.com/orders' (
    Body: template '{1}' with ({1} = $Payload),
  ) returns String`, "{1}", []string{"$Payload"}},
		{"control: a template whose TEXT is $Payload", `call rest service post 'https://example.com/orders' (
    Body: template '$Payload',
  ) returns String`, "$Payload", nil},
		{"control: a string literal is the template text", `call rest service post 'https://example.com/orders' (
    Body: 'hello',
  ) returns String`, "hello", nil},
		{"control: a string literal with parameters", `call rest service post 'https://example.com/orders' (
    Body: '{1}!' with ({1} = $Payload),
  ) returns String`, "{1}!", []string{"$Payload"}},
		{"control: deprecated string clause", `call rest service post 'https://example.com/orders'
    body '{1}!' with ({1} = $Payload)
    returns String`, "{1}!", []string{"$Payload"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := customBody(t, restCallFromMDL(t, restCallMicroflow(c.call)))
			if body.Template != c.wantTemplate || !reflect.DeepEqual(body.TemplateParams, c.wantParams) {
				t.Errorf("custom request template = %q with %q, want %q with %q",
					body.Template, body.TemplateParams, c.wantTemplate, c.wantParams)
			}
		})
	}
}

func TestRestCallURL_ExpressionIsTheTemplatesParameter(t *testing.T) {
	for _, c := range []struct {
		name, call   string
		wantTemplate string
		wantParams   []string
	}{
		{"an expression URL", `call rest service get @M.BaseUrl + '/rates' (
    Timeout: 30,
  ) returns String`, "{1}", []string{"@M.BaseUrl + '/rates'"}},
		{"a variable URL", `call rest service get $Payload returns String`, "{1}", []string{"$Payload"}},
		{"control: a template URL", `call rest service get '{1}/rates' with ({1} = @M.BaseUrl) (
    Timeout: 30,
  ) returns String`, "{1}/rates", []string{"@M.BaseUrl"}},
		{"control: a literal URL", `call rest service get 'https://example.com/rates' returns String`,
			"https://example.com/rates", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			rc := restCallFromMDL(t, restCallMicroflow(c.call))
			hc := rc.HttpConfiguration
			if hc.LocationTemplate != c.wantTemplate || !reflect.DeepEqual(hc.LocationParams, c.wantParams) {
				t.Errorf("location template = %q with %q, want %q with %q",
					hc.LocationTemplate, hc.LocationParams, c.wantTemplate, c.wantParams)
			}
		})
	}
}

// An expression that ALSO states parameters has no meaning Studio Pro can
// store: the expression is not a template, so there is nothing for {1} to
// fill. It used to store the expression's source as the template text. The
// check pass and exec make the one decision (checkRestCallTemplatesStmt).
func TestRestCallTemplate_ExpressionWithParametersIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, call string
		refused    bool
	}{
		{"body", `call rest service post 'https://example.com/orders' (
    Body: $Payload with ({1} = $Payload),
  ) returns String`, true},
		{"deprecated body", `call rest service post 'https://example.com/orders'
    body $Payload with ({1} = $Payload)
    returns String`, true},
		{"url", `call rest service get @M.BaseUrl + '/{1}' with ({1} = $Payload) returns String`, true},
		{"control: template body", `call rest service post 'https://example.com/orders' (
    Body: template '{1}' with ({1} = $Payload),
  ) returns String`, false},
		{"control: template url", `call rest service get '{1}/rates' with ({1} = $Payload) returns String`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := restCallMicroflow(c.call)
			create := firstCreateMicroflow(t, src)

			var rule string
			for _, v := range ValidateMicroflow(create) {
				if v.RuleID == "MDL-REST05" {
					rule = v.Message
				}
			}
			if (rule != "") != c.refused {
				t.Errorf("check: MDL-REST05 reported = %v, want %v (%q)", rule != "", c.refused, rule)
			}

			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
				measurer: &layoutMeasurer{}, varTypes: map[string]string{}}
			fb.buildFlowGraph(create.Body, create.ReturnType)
			var execErr string
			for _, e := range fb.GetErrors() {
				if strings.Contains(e, "with (") {
					execErr = e
				}
			}
			if (execErr != "") != c.refused {
				t.Errorf("exec: refused = %v, want %v (errors: %q)", execErr != "", c.refused, fb.GetErrors())
			}
		})
	}
}

// describe prints the stored form, `Body: template '{1}' with ({1} = $Payload)`;
// executing that must store the same template as the statement it came from.
func TestRestCallBody_ExpressionRoundTripsThroughDescribe(t *testing.T) {
	src := restCallMicroflow(`call rest service post @M.BaseUrl + '/orders' (
    Body: $Payload,
  ) returns String`)
	first := restCallFromMDL(t, src)

	described := newTestExecutor().formatRestCallAction(first)
	for _, want := range []string{
		"call rest service post '{1}' with ({1} = @M.BaseUrl + '/orders')",
		"Body: template '{1}' with ({1} = $Payload),",
	} {
		if !strings.Contains(described, want) {
			t.Fatalf("describe printed:\n%s\nwant it to contain %q", described, want)
		}
	}

	again := restCallFromMDL(t, "create microflow M.Send ($Payload: String) returns String\nbegin\n  "+
		described+"\n  return $Response;\nend;\n")
	if a, b := customBody(t, first), customBody(t, again); a.Template != b.Template ||
		!reflect.DeepEqual(a.TemplateParams, b.TemplateParams) {
		t.Errorf("body after describe → exec = %q with %q, want %q with %q",
			b.Template, b.TemplateParams, a.Template, a.TemplateParams)
	}
	if a, b := first.HttpConfiguration, again.HttpConfiguration; a.LocationTemplate != b.LocationTemplate ||
		!reflect.DeepEqual(a.LocationParams, b.LocationParams) {
		t.Errorf("URL after describe → exec = %q with %q, want %q with %q",
			b.LocationTemplate, b.LocationParams, a.LocationTemplate, a.LocationParams)
	}
}

// The two spellings are one activity, so `create or modify` re-run against the
// flow it wrote sees nothing changed (the ako/mxcli#905 rule for messages).
func TestDeclaredMatches_RestCallExpressionIsItsTemplate(t *testing.T) {
	stored := parseFlowBody(t, `  $R = call rest service post '{1}' with ({1} = @M.Base + '/o') (
    Body: template '{1}' with ({1} = $In),
  ) returns String;`)
	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"as written", `  $R = call rest service post @M.Base + '/o' (Body: $In) returns String;`, true},
		{"as described", `  $R = call rest service post '{1}' with ({1} = @M.Base + '/o') (
    Body: template '{1}' with ({1} = $In),
  ) returns String;`, true},
		{"control: the body's text is $In", `  $R = call rest service post @M.Base + '/o' (Body: template '$In') returns String;`, false},
		{"control: another body", `  $R = call rest service post @M.Base + '/o' (Body: $In + 'x') returns String;`, false},
		{"control: another URL", `  $R = call rest service post @M.Base + '/p' (Body: $In) returns String;`, false},
	} {
		declared := parseFlowBody(t, c.body)
		if got := declaredMatches(declared.Body, stored.Body); got != c.want {
			t.Errorf("%s: declaredMatches = %v, want %v", c.name, got, c.want)
		}
	}
}
