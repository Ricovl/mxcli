// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// firstTemplateParameter returns the one Forms$ClientTemplateParameter of a
// client template built from a single parameter.
func firstTemplateParameter(t *testing.T, p *pages.ClientTemplateParameter) bson.D {
	t.Helper()
	tmpl := BuildClientTemplateWithTextAndParams("{1}", []*pages.ClientTemplateParameter{p})
	params, ok := findField(t, tmpl, "Parameters").(bson.A)
	if !ok || len(params) != 2 {
		t.Fatalf("Parameters = %#v, want the marker and one parameter", findField(t, tmpl, "Parameters"))
	}
	param, ok := params[1].(bson.D)
	if !ok {
		t.Fatalf("Parameters[1] is %T, not a document", params[1])
	}
	return param
}

// mendixlabs/mxcli#1494: a pluggable widget's text-template parameter bound
// across an association (`legendParams: ({1} = Repro.Parent_Child/Label)`)
// resolved to the right attribute and the right steps, and the serializer threw
// the steps away: AttributeRef.EntityRef was written null, so mxbuild read
// Repro.Child.Label against the widget's context entity — CE1365 "Attribute
// 'Repro.Child.Label' is not an attribute of entity 'Repro.Parent'." The same
// serializer writes Data grid 2 column templates and object-list item templates.
func TestBuildClientTemplateWithTextAndParams_AssociationSteps(t *testing.T) {
	param := firstTemplateParameter(t, &pages.ClientTemplateParameter{
		AttributeRef: "Repro.Child.Label",
		AttributeRefSteps: []pages.AttributeRefStep{
			{Association: "Repro.Parent_Child", DestinationEntity: "Repro.Child"},
		},
	})
	attrRef, ok := findField(t, param, "AttributeRef").(bson.D)
	if !ok {
		t.Fatalf("AttributeRef is %T, not a document", findField(t, param, "AttributeRef"))
	}
	if a := findField(t, attrRef, "Attribute"); a != "Repro.Child.Label" {
		t.Errorf("Attribute = %v, want Repro.Child.Label", a)
	}
	entityRef, ok := findField(t, attrRef, "EntityRef").(bson.D)
	if !ok {
		t.Fatalf("EntityRef is %v, want a DomainModels$IndirectEntityRef — the association hop was dropped "+
			"(CE1365: Repro.Child.Label read against the context entity)", findField(t, attrRef, "EntityRef"))
	}
	if tp := findField(t, entityRef, "$Type"); tp != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef.$Type = %v, want DomainModels$IndirectEntityRef", tp)
	}
	steps, ok := findField(t, entityRef, "Steps").(bson.A)
	if !ok || len(steps) != 2 {
		t.Fatalf("Steps = %#v, want the marker and one step", findField(t, entityRef, "Steps"))
	}
	step, ok := steps[1].(bson.D)
	if !ok {
		t.Fatalf("Steps[1] is %T, not a document", steps[1])
	}
	if a := findField(t, step, "Association"); a != "Repro.Parent_Child" {
		t.Errorf("step Association = %v, want Repro.Parent_Child", a)
	}
	if d := findField(t, step, "DestinationEntity"); d != "Repro.Child" {
		t.Errorf("step DestinationEntity = %v, want Repro.Child", d)
	}
}

// The control: an attribute of the context entity itself has no steps, and its
// EntityRef stays null — what Studio Pro stores, and what this wrote before.
func TestBuildClientTemplateWithTextAndParams_OwnAttributeHasNoEntityRef(t *testing.T) {
	param := firstTemplateParameter(t, &pages.ClientTemplateParameter{AttributeRef: "Repro.Parent.Name"})
	attrRef, ok := findField(t, param, "AttributeRef").(bson.D)
	if !ok {
		t.Fatalf("AttributeRef is %T, not a document", findField(t, param, "AttributeRef"))
	}
	if e := findField(t, attrRef, "EntityRef"); e != nil {
		t.Errorf("EntityRef = %#v, want null for an own-entity attribute", e)
	}
}

// mendixlabs/mxcli#1495: an expression parameter (`{1} = if 1 = 1 then 'A'
// else 'B'`) is the parameter's Expression, with no AttributeRef. v0.24.0 put
// the expression text in AttributeRef.Attribute, and mxbuild answered
//
//	CE1613 "The selected attribute 'Repro.Parent.if1=1then'A'else'B'' no longer exists."
//
// #823 fixed where the executor puts it; this pins that the pluggable
// serializer keeps it there.
func TestBuildClientTemplateWithTextAndParams_ExpressionParameter(t *testing.T) {
	param := firstTemplateParameter(t, &pages.ClientTemplateParameter{Expression: "if 1 = 1 then 'A' else 'B'"})
	if a := findField(t, param, "AttributeRef"); a != nil {
		t.Errorf("AttributeRef = %#v, want null for an expression parameter", a)
	}
	if e := findField(t, param, "Expression"); e != "if 1 = 1 then 'A' else 'B'" {
		t.Errorf("Expression = %v, want the expression as written", e)
	}
}
