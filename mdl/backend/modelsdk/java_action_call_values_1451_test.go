// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genCa "github.com/mendixlabs/mxcli/modelsdk/gen/codeactions"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1451 (mendixlabs/mxcli#1554): a Java action parameter that is a
// text template (BusinessEvents.PublishBusinessEvent_V2's Subject) or a custom
// document (AgentEditorCommons' Agent) was written as an expression, and
// mxbuild reported CE0115. These pin the BSON the write path produces for the
// two values the builder now chooses, and that the read path gives them back.

// encodedJavaCallValues encodes a Java action call and returns its mappings'
// Value documents, in order.
func encodedJavaCallValues(t *testing.T, action *microflows.JavaActionCallAction) []bson.Raw {
	t.Helper()
	encoded, err := (&codec.Encoder{}).Encode(microflowActionToGen(action))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	vals, err := bson.Raw(encoded).Lookup("ParameterMappings").Array().Values()
	if err != nil {
		t.Fatalf("ParameterMappings: %v", err)
	}
	var out []bson.Raw
	for _, v := range vals {
		if doc, ok := v.DocumentOK(); ok { // element 0 is the list marker
			out = append(out, doc.Lookup("Value").Document())
		}
	}
	return out
}

func templateAndDocumentCall() *microflows.JavaActionCallAction {
	return &microflows.JavaActionCallAction{
		JavaAction: "BusinessEvents.PublishBusinessEvent_V2",
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "BusinessEvents.PublishBusinessEvent_V2.Subject",
				Value: &microflows.StringTemplateParameterValue{TypedTemplate: &microflows.TypedTemplate{
					Text: "Order {1} shipped, {2} items",
					Arguments: []*microflows.TypedTemplateArgument{
						{Expression: "$Event/OrderNumber", Type: "String"},
						{Expression: "$Event/Quantity", Type: "Integer"},
					},
				}},
			},
			{
				Parameter: "AgentEditorCommons.Agent_CallDocument_WithoutHistory.Agent",
				Value:     &microflows.CustomBlobDocumentCodeActionParameterValue{CustomDocument: "MyModule.PaymentReminderAgent"},
			},
		},
	}
}

func TestJavaActionCallToGen_TemplateAndDocumentValues(t *testing.T) {
	values := encodedJavaCallValues(t, templateAndDocumentCall())
	if len(values) != 2 {
		t.Fatalf("values = %d, want 2", len(values))
	}

	tmpl := values[0]
	if got := tmpl.Lookup("$Type").StringValue(); got != "Microflows$StringTemplateParameterValue" {
		t.Fatalf("Subject value $Type = %q, want Microflows$StringTemplateParameterValue (CE0115 otherwise)", got)
	}
	tt := tmpl.Lookup("TypedTemplate").Document()
	if got := tt.Lookup("$Type").StringValue(); got != "Microflows$TypedTemplate" {
		t.Errorf("TypedTemplate $Type = %q", got)
	}
	if got := tt.Lookup("Text").StringValue(); got != "Order {1} shipped, {2} items" {
		t.Errorf("Text = %q", got)
	}
	args, err := tt.Lookup("Arguments").Array().Values()
	if err != nil {
		t.Fatalf("Arguments: %v", err)
	}
	if len(args) != 3 || args[0].Int32() != 2 {
		t.Fatalf("Arguments = %v, want marker 2 then two arguments", args)
	}
	for i, want := range [][2]string{{"$Event/OrderNumber", "String"}, {"$Event/Quantity", "Integer"}} {
		a := args[i+1].Document()
		if got := a.Lookup("$Type").StringValue(); got != "Microflows$PrimitiveTypedTemplateArgument" {
			t.Errorf("argument %d $Type = %q", i+1, got)
		}
		if got := a.Lookup("Expression").StringValue(); got != want[0] {
			t.Errorf("argument %d Expression = %q, want %q", i+1, got, want[0])
		}
		// mxbuild checks the expression against Type: CE0117 on a mismatch.
		if got := a.Lookup("Type").StringValue(); got != want[1] {
			t.Errorf("argument %d Type = %q, want %q", i+1, got, want[1])
		}
	}

	doc := values[1]
	if got := doc.Lookup("$Type").StringValue(); got != "Microflows$CustomBlobDocumentCodeActionParameterValue" {
		t.Fatalf("Agent value $Type = %q, want Microflows$CustomBlobDocumentCodeActionParameterValue", got)
	}
	if got := doc.Lookup("CustomDocument").StringValue(); got != "MyModule.PaymentReminderAgent" {
		t.Errorf("CustomDocument = %q", got)
	}
}

func TestJavaActionCallFromGen_TemplateAndDocumentValuesRoundTrip(t *testing.T) {
	values := encodedJavaCallValues(t, templateAndDocumentCall())

	tmpl, ok := codeActionParameterValueFromRaw(values[0]).(*microflows.StringTemplateParameterValue)
	if !ok || tmpl.TypedTemplate == nil {
		t.Fatalf("Subject read back as %T", codeActionParameterValueFromRaw(values[0]))
	}
	if len(tmpl.TypedTemplate.Arguments) != 2 {
		t.Fatalf("arguments read back = %d, want 2 (describe would drop them)", len(tmpl.TypedTemplate.Arguments))
	}
	if a := tmpl.TypedTemplate.Arguments[1]; a.Expression != "$Event/Quantity" || a.Type != "Integer" {
		t.Errorf("argument 2 = %+v", a)
	}

	doc, ok := codeActionParameterValueFromRaw(values[1]).(*microflows.CustomBlobDocumentCodeActionParameterValue)
	if !ok || doc.CustomDocument != "MyModule.PaymentReminderAgent" {
		t.Errorf("Agent read back as %#v", codeActionParameterValueFromRaw(values[1]))
	}
}

// The document parameter's TYPE: read as a String before, so describe printed
// `Agent: String` and the call builder took it for an expression parameter.
func TestJavaActionParamType_CustomBlobDocumentRoundTrips(t *testing.T) {
	g := genCa.NewCustomBlobDocumentParameterType()
	g.SetCustomDocumentTypeName("agenteditor.agent")
	g.SetCustomDocumentReadableTypeName("Agent")

	got, ok := codeActionParamTypeFromGen(g).(*javaactions.CustomBlobDocumentType)
	if !ok {
		t.Fatalf("read as %T, want *CustomBlobDocumentType", codeActionParamTypeFromGen(g))
	}
	if got.DocumentType != "agenteditor.agent" || got.ReadableName != "Agent" {
		t.Errorf("read %+v", got)
	}
	if s := got.TypeString(); s != "Document(agenteditor.agent)" {
		t.Errorf("TypeString = %q", s)
	}

	back, ok := codeActionParamTypeToGen(got).(*genCa.CustomBlobDocumentParameterType)
	if !ok {
		t.Fatalf("written as %T, want *CustomBlobDocumentParameterType (a rewrite would turn it into a String)",
			codeActionParamTypeToGen(got))
	}
	if back.CustomDocumentTypeName() != "agenteditor.agent" || back.CustomDocumentReadableTypeName() != "Agent" {
		t.Errorf("written %q / %q", back.CustomDocumentTypeName(), back.CustomDocumentReadableTypeName())
	}
}
