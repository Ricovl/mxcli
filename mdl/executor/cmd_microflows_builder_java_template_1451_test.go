// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1451 (mendixlabs/mxcli#1554): `call java action` wrote a
// text-template parameter (BusinessEvents.PublishBusinessEvent_V2's Subject,
// a CodeActions$StringTemplateParameterType) and a document parameter
// (AgentEditorCommons' Agent, a CodeActions$CustomBlobDocumentParameterType)
// as Microflows$BasicCodeActionParameterValue expressions. check and exec
// passed; mx check reported CE0115 "The arguments that are passed to Java
// action … do not match the expected parameters and need to be refreshed".

// templateCallBackend is a project with one Java action taking a text
// template, an object and an agent document, and the entity the microflow's
// $Event holds.
func templateCallBackend() *mock.MockBackend {
	mod := mkModule("MyModule")
	ent := mkEntity(mod.ID, "OrderShipped")
	ent.Attributes = []*domainmodel.Attribute{
		{Name: "OrderNumber", Type: &domainmodel.StringAttributeType{}},
		{Name: "Quantity", Type: &domainmodel.IntegerAttributeType{}},
		{Name: "Price", Type: &domainmodel.DecimalAttributeType{}},
		{Name: "Shipped", Type: &domainmodel.DateTimeAttributeType{}},
		{Name: "Status", Type: &domainmodel.EnumerationAttributeType{EnumerationRef: "MyModule.Status"}},
	}
	dm := mkDomainModel(mod.ID, ent)
	return &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "MyModule" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				Name: "Publish",
				Parameters: []*javaactions.JavaActionParameter{
					{Name: "Subject", ParameterType: &javaactions.StringTemplateParameterType{Grammar: "Text"}},
					{Name: "BusinessEvent", ParameterType: &javaactions.EntityType{Entity: "MyModule.OrderShipped"}},
					{Name: "Agent", ParameterType: &javaactions.CustomBlobDocumentType{DocumentType: "agenteditor.agent", ReadableName: "Agent"}},
				},
			}, nil
		},
	}
}

func templateCallBuilder() *flowBuilder {
	return &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		backend:      templateCallBackend(),
		varTypes:     map[string]string{"Event": "MyModule.OrderShipped"},
		declaredVars: map[string]string{"Count": "Integer"},
	}
}

// buildJavaCall builds the first `call java action` of a microflow body.
func buildJavaCall(t *testing.T, fb *flowBuilder, call string) *microflows.JavaActionCallAction {
	t.Helper()
	body := parsedFlowBody(t, "create microflow MyModule.MF ($Event: MyModule.OrderShipped)\nbegin\n  "+call+"\nend;")
	for _, stmt := range body {
		s, ok := stmt.(*ast.CallJavaActionStmt)
		if !ok {
			continue
		}
		id := fb.addCallJavaActionAction(s)
		for _, obj := range fb.objects {
			if obj.GetID() == id {
				return obj.(*microflows.ActionActivity).Action.(*microflows.JavaActionCallAction)
			}
		}
	}
	t.Fatalf("no call java action in %q", call)
	return nil
}

func mappingValue(t *testing.T, a *microflows.JavaActionCallAction, param string) microflows.CodeActionParameterValue {
	t.Helper()
	for _, m := range a.ParameterMappings {
		if strings.HasSuffix(m.Parameter, "."+param) {
			return m.Value
		}
	}
	t.Fatalf("no mapping for %s", param)
	return nil
}

func typedTemplate(t *testing.T, v microflows.CodeActionParameterValue) *microflows.TypedTemplate {
	t.Helper()
	st, ok := v.(*microflows.StringTemplateParameterValue)
	if !ok || st.TypedTemplate == nil {
		t.Fatalf("value = %#v, want a StringTemplateParameterValue — a BasicCodeActionParameterValue is CE0115", v)
	}
	return st.TypedTemplate
}

func TestCallJavaAction_TemplateParameterWithArguments(t *testing.T) {
	fb := templateCallBuilder()
	a := buildJavaCall(t, fb, `call java action MyModule.Publish(
    Subject = 'Order {1} shipped: {2} x {3} on {4}' with ({1} = $Event/OrderNumber, {2} = $Event/Quantity, {3} = $Event/Price, {4} = $Event/Shipped),
    BusinessEvent = $Event);`)
	if len(fb.errors) > 0 {
		t.Fatalf("builder errors: %v", fb.errors)
	}
	tt := typedTemplate(t, mappingValue(t, a, "Subject"))
	if tt.Text != "Order {1} shipped: {2} x {3} on {4}" {
		t.Errorf("Text = %q", tt.Text)
	}
	want := [][2]string{
		{"$Event/OrderNumber", "String"}, {"$Event/Quantity", "Integer"},
		{"$Event/Price", "Decimal"}, {"$Event/Shipped", "DateTime"},
	}
	if len(tt.Arguments) != len(want) {
		t.Fatalf("arguments = %d, want %d", len(tt.Arguments), len(want))
	}
	for i, w := range want {
		if got := tt.Arguments[i]; got.Expression != w[0] || got.Type != w[1] {
			// The Type is checked by mxbuild against the expression: CE0117.
			t.Errorf("{%d} = %q (%s), want %q (%s)", i+1, got.Expression, got.Type, w[0], w[1])
		}
	}
	// The object parameter next to it keeps its expression.
	if b, ok := mappingValue(t, a, "BusinessEvent").(*microflows.BasicCodeActionParameterValue); !ok || b.Argument != "$Event" {
		t.Errorf("BusinessEvent = %#v", mappingValue(t, a, "BusinessEvent"))
	}
}

// The issue's own repro: a plain string is the template's text.
func TestCallJavaAction_TemplateParameterPlainString(t *testing.T) {
	fb := templateCallBuilder()
	a := buildJavaCall(t, fb, `call java action MyModule.Publish(Subject = 'Order shipped', BusinessEvent = $Event);`)
	tt := typedTemplate(t, mappingValue(t, a, "Subject"))
	if tt.Text != "Order shipped" || len(tt.Arguments) != 0 {
		t.Errorf("template = %q with %d arguments, want 'Order shipped' with none", tt.Text, len(tt.Arguments))
	}
}

// An expression is the template '{1}' with the expression as its argument,
// as a REST call body (ako/mxcli#1444) — not its source text.
func TestCallJavaAction_TemplateParameterExpression(t *testing.T) {
	fb := templateCallBuilder()
	a := buildJavaCall(t, fb, `call java action MyModule.Publish(Subject = 'Order ' + $Event/OrderNumber, BusinessEvent = $Event);`)
	tt := typedTemplate(t, mappingValue(t, a, "Subject"))
	if tt.Text != "{1}" || len(tt.Arguments) != 1 {
		t.Fatalf("template = %q with %d arguments, want '{1}' with one", tt.Text, len(tt.Arguments))
	}
	if got := tt.Arguments[0]; got.Expression != "'Order ' + $Event/OrderNumber" || got.Type != "String" {
		t.Errorf("{1} = %+v", got)
	}

	a = buildJavaCall(t, templateCallBuilder(), `call java action MyModule.Publish(Subject = $Count, BusinessEvent = $Event);`)
	if got := typedTemplate(t, mappingValue(t, a, "Subject")).Arguments[0].Type; got != "Integer" {
		t.Errorf("{1} = $Count has Type %q, want Integer (a declared Integer variable)", got)
	}
	// A date token is a DateTime, not the String exprcheck calls every token
	// (measured: mxbuild reports CE0117 for [%CurrentDateTime%] typed String).
	a = buildJavaCall(t, templateCallBuilder(), `call java action MyModule.Publish(Subject = 'At {1}' with ({1} = [%CurrentDateTime%]), BusinessEvent = $Event);`)
	if got := typedTemplate(t, mappingValue(t, a, "Subject")).Arguments[0].Type; got != "DateTime" {
		t.Errorf("{1} = [%%CurrentDateTime%%] has Type %q, want DateTime", got)
	}
}

func TestCallJavaAction_DocumentParameter(t *testing.T) {
	for _, arg := range []string{"MyModule.PaymentReminderAgent", "'MyModule.PaymentReminderAgent'"} {
		fb := templateCallBuilder()
		a := buildJavaCall(t, fb, `call java action MyModule.Publish(Subject = 'x', Agent = `+arg+`);`)
		if len(fb.errors) > 0 {
			t.Fatalf("Agent = %s: builder errors: %v", arg, fb.errors)
		}
		v, ok := mappingValue(t, a, "Agent").(*microflows.CustomBlobDocumentCodeActionParameterValue)
		if !ok {
			t.Fatalf("Agent = %s: value = %#v, want a CustomBlobDocumentCodeActionParameterValue — an expression is CE0115",
				arg, mappingValue(t, a, "Agent"))
		}
		if v.CustomDocument != "MyModule.PaymentReminderAgent" {
			t.Errorf("Agent = %s: CustomDocument = %q", arg, v.CustomDocument)
		}
	}
}

func TestCallJavaAction_TemplateAndDocumentRefusals(t *testing.T) {
	cases := []struct {
		name, call, want string
	}{
		{"expression with parameters",
			`call java action MyModule.Publish(Subject = $Event/OrderNumber with ({1} = $Event/Quantity), BusinessEvent = $Event);`,
			"is an expression, so `with (…)` has no template"},
		{"with on a parameter that is no template",
			`call java action MyModule.Publish(Subject = 'x', BusinessEvent = 'a {1}' with ({1} = $Event/OrderNumber));`,
			"BusinessEvent is not a text template"},
		{"enumeration argument",
			`call java action MyModule.Publish(Subject = 'Now {1}' with ({1} = $Event/Status), BusinessEvent = $Event);`,
			"is an enumeration"},
		{"gap in the placeholders",
			`call java action MyModule.Publish(Subject = 'a {2}' with ({2} = $Event/OrderNumber), BusinessEvent = $Event);`,
			"has no {1} argument"},
		{"document given as an expression",
			`call java action MyModule.Publish(Subject = 'x', Agent = $Event/OrderNumber);`,
			"Agent takes a document"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := templateCallBuilder()
			buildJavaCall(t, fb, c.call)
			if !strings.Contains(strings.Join(fb.errors, "\n"), c.want) {
				t.Errorf("errors = %v, want one containing %q", fb.errors, c.want)
			}
		})
	}
}

// describe prints each value in a form that re-executes to the same document.
func TestCallJavaAction_TemplateAndDocumentDescribeRoundTrip(t *testing.T) {
	calls := []string{
		`call java action MyModule.Publish(Subject = 'Order {1} shipped: {2}' with ({1} = $Event/OrderNumber, {2} = $Event/Quantity), BusinessEvent = $Event, Agent = 'MyModule.PaymentReminderAgent');`,
		`call java action MyModule.Publish(Subject = 'Order shipped', BusinessEvent = $Event);`,
		`call java action MyModule.Publish(Subject = $Event/OrderNumber, BusinessEvent = $Event);`,
		// '{1}' holding a string literal is not the literal: it stays a template.
		`call java action MyModule.Publish(Subject = '{1}' with ({1} = 'x'), BusinessEvent = $Event);`,
	}
	for _, call := range calls {
		ctx, _ := newMockCtx(t)
		first := buildJavaCall(t, templateCallBuilder(), call)
		described := formatAction(ctx, first, nil, nil)
		if described != call {
			t.Errorf("describe printed\n  %s\nfor\n  %s", described, call)
		}
		second := buildJavaCall(t, templateCallBuilder(), described)
		for i, m := range first.ParameterMappings {
			a, b := describeValueKey(m.Value), describeValueKey(second.ParameterMappings[i].Value)
			if a != b {
				t.Errorf("%s: re-executed as %s, built %s", m.Parameter, b, a)
			}
		}
	}
}

// describeValueKey is a parameter value without its IDs, for comparing two
// builds.
func describeValueKey(v microflows.CodeActionParameterValue) string {
	switch x := v.(type) {
	case *microflows.StringTemplateParameterValue:
		var b strings.Builder
		b.WriteString("template " + x.TypedTemplate.Text)
		for _, a := range x.TypedTemplate.Arguments {
			b.WriteString(" | " + a.Expression + " : " + a.Type)
		}
		return b.String()
	case *microflows.CustomBlobDocumentCodeActionParameterValue:
		return "document " + x.CustomDocument
	case *microflows.BasicCodeActionParameterValue:
		return "basic " + x.Argument
	}
	return "other"
}

// check --references gives the builder's verdict, and resolves the document:
// a missing one is CE1613 at build time.
func TestValidate_JavaActionTemplateAndDocumentArguments(t *testing.T) {
	mb := templateCallBackend()
	mod, _ := mb.GetModuleByNameFunc("MyModule")
	mb.IsConnectedFunc = func() bool { return true }
	mb.ListModulesFunc = func() ([]*model.Module, error) { return []*model.Module{mod}, nil }
	mb.ListJavaActionsFunc = func() ([]*types.JavaAction, error) {
		return []*types.JavaAction{{Name: "Publish", ContainerID: mod.ID}}, nil
	}
	mb.ListAgentEditorAgentsFunc = func() ([]*agenteditor.Agent, error) {
		return []*agenteditor.Agent{{ContainerID: mod.ID, Name: "PaymentReminderAgent"}}, nil
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	errs := func(call string) string {
		body := parsedFlowBody(t, "create microflow MyModule.MF ($Event: MyModule.OrderShipped)\nbegin\n  "+call+"\nend;")
		return strings.Join(validateFlowBodyReferences(ctx, body, newScriptContext()), "\n")
	}
	if got := errs(`call java action MyModule.Publish(Subject = 'Order {1}' with ({1} = $Event/OrderNumber), BusinessEvent = $Event, Agent = MyModule.PaymentReminderAgent);`); got != "" {
		t.Errorf("valid call: unexpected reference errors: %s", got)
	}
	if got := errs(`call java action MyModule.Publish(Subject = 'x', Agent = MyModule.NoSuchAgent);`); !strings.Contains(got, "MyModule.NoSuchAgent") {
		t.Errorf("missing agent: want a reference error naming it, got %q", got)
	}
	if got := errs(`call java action MyModule.Publish(Subject = $Event/OrderNumber with ({1} = 1));`); !strings.Contains(got, "is an expression") {
		t.Errorf("expression with parameters: got %q", got)
	}
	if got := errs(`call java action MyModule.Publish(BusinessEvent = 'a {1}' with ({1} = 1));`); !strings.Contains(got, "not a text template") {
		t.Errorf("with on an object parameter: got %q", got)
	}
}
