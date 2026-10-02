// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// MDL-WIDGET39 (CE2421) and MDL-WIDGET40 (CE0582): two page errors only mxbuild
// reported. `check --references` passed a textbox bound to an enumeration and a
// classic drop-down on a React-client project; `mx check` then failed both.

func typeCheckCtx(t *testing.T, optimizedClient string) *ExecContext {
	t.Helper()
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	attr := func(name string, typ domainmodel.AttributeType) *domainmodel.Attribute {
		return &domainmodel.Attribute{Name: name, Type: typ}
	}
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-thing")}, Name: "Thing",
						Attributes: []*domainmodel.Attribute{
							attr("Name", &domainmodel.StringAttributeType{Length: 200}),
							attr("Pw", &domainmodel.HashedStringAttributeType{}),
							attr("Color", &domainmodel.EnumerationAttributeType{EnumerationRef: "Sales.Color"}),
							attr("Flag", &domainmodel.BooleanAttributeType{}),
							attr("WhenAt", &domainmodel.DateTimeAttributeType{}),
							attr("Num", &domainmodel.IntegerAttributeType{}),
							attr("LongNum", &domainmodel.LongAttributeType{}),
							attr("Amount", &domainmodel.DecimalAttributeType{}),
							attr("Seq", &domainmodel.AutoNumberAttributeType{}),
							attr("Blob", &domainmodel.BinaryAttributeType{}),
						}},
					{BaseElement: model.BaseElement{ID: model.ID("e-other")}, Name: "Other",
						Attributes: []*domainmodel.Attribute{
							attr("Shade", &domainmodel.EnumerationAttributeType{EnumerationRef: "Sales.Color"}),
							attr("Txt", &domainmodel.StringAttributeType{}),
						}},
					{BaseElement: model.BaseElement{ID: model.ID("e-sub")}, Name: "SubThing",
						GeneralizationRef: "Sales.Thing",
						Attributes:        []*domainmodel.Attribute{attr("Extra", &domainmodel.StringAttributeType{})}},
				},
				Associations: []*domainmodel.Association{{
					BaseElement: model.BaseElement{ID: model.ID("a-thing-other")}, Name: "Thing_Other",
					ParentID: model.ID("e-thing"), ChildID: model.ID("e-other"),
				}},
			}}, nil
		},
	}
	if optimizedClient != "" {
		b.GetProjectSettingsFunc = func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: optimizedClient}}, nil
		}
	}
	ctx, _ := newMockCtx(t, withBackend(b))
	ctx.widgetRegistry = &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{}, byWidgetID: map[string]*WidgetDefinition{}}
	ctx.widgetRegistryLoaded = true
	return ctx
}

func typeCheckPage(t *testing.T, body string) *ast.CreatePageStmtV3 {
	t.Helper()
	src := `create page Sales.P (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ($Thing: Sales.Thing, $Sub: Sales.SubThing)) {
` + body + `
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

func typeErrs(t *testing.T, ctx *ExecContext, body string, sc *scriptContext) []string {
	t.Helper()
	s := typeCheckPage(t, body)
	if sc == nil {
		sc = newScriptContext()
	}
	return validatePluggableAttributeScopes(ctx, s.Parameters, allPageWidgets(s), sc)
}

// The reported case: a text box on an enumeration attribute.
func TestCheck_TextBoxOnEnumerationIsCE2421(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) { textbox tbColor (label: 'C', attribute: Color) }`, nil)
	if len(errs) != 1 {
		t.Fatalf("want one error, got %q", errs)
	}
	for _, want := range []string{"MDL-WIDGET39", "CE2421", "tbColor", "Sales.Thing.Color", "Enumeration",
		"Hashed string, Integer, Long, String, Decimal, AutoNumber"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}
}

// The measured matrix (mxbuild 11.14.0, every pair built on a fresh app): for
// each built-in input widget, the attribute types it accepts. Every other pair
// in the table was CE2421. Written out here independently of the rule's own
// table so the test is a second statement of the measurement, not a copy.
func TestCheck_InputWidgetAttributeTypeMatrix(t *testing.T) {
	ctx := typeCheckCtx(t, "No")
	attrs := map[string]string{ // attribute -> kind
		"Name": "String", "Pw": "HashedString", "Color": "Enumeration", "Flag": "Boolean",
		"WhenAt": "DateTime", "Num": "Integer", "LongNum": "Long", "Amount": "Decimal",
		"Seq": "AutoNumber", "Blob": "Binary",
	}
	accepts := map[string]map[string]bool{
		"textbox":      {"String": true, "HashedString": true, "Integer": true, "Long": true, "Decimal": true, "AutoNumber": true},
		"textarea":     {"String": true},
		"datepicker":   {"DateTime": true},
		"checkbox":     {"Boolean": true},
		"radiobuttons": {"Boolean": true, "Enumeration": true},
		"dropdown":     {"Enumeration": true},
	}
	for widget, ok := range accepts {
		for attrName, kind := range attrs {
			body := fmt.Sprintf(`dataview dv (datasource: $Thing) { %s w1 (label: 'x', attribute: %s) }`, widget, attrName)
			errs := typeErrs(t, ctx, body, nil)
			flagged := len(errs) > 0
			if kind == "HashedString" {
				// Not measured: MDL cannot author one, so it is never judged.
				if flagged {
					t.Errorf("%s on %s: an unmeasured kind was judged: %q", widget, attrName, errs)
				}
				continue
			}
			if flagged == ok[kind] {
				t.Errorf("%s on %s (%s): flagged=%v, want %v (%q)", widget, attrName, kind, flagged, !ok[kind], errs)
			}
		}
	}
}

// An attribute declared on a generalization is judged by its declared type.
func TestCheck_InheritedEnumerationOnTextBoxIsCE2421(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Sub) {
    textbox tbColor (label: 'C', attribute: Color)
    textbox tbName (label: 'N', attribute: Name)
  }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "tbColor") || !strings.Contains(errs[0], "Sales.Thing.Color") {
		t.Fatalf("want one error on tbColor naming the declaring entity, got %q", errs)
	}
}

// Over an association path the final attribute's type is what counts — mxbuild
// reported `Thing_OtherE/Shade` against `OtherE.Shade`.
func TestCheck_AssociationPathIsJudgedAtItsEnd(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) {
    textbox tbShade (label: 'S', attribute: Thing_Other/Shade)
    textbox tbTxt (label: 'T', attribute: Thing_Other/Txt)
  }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "tbShade") || !strings.Contains(errs[0], "Sales.Other.Shade") {
		t.Fatalf("want one error on tbShade, got %q", errs)
	}
}

// A bare association where an attribute belongs: mxbuild answers CE1613 (the
// page builder qualifies it as an attribute that does not exist), and that
// error stops `mx check` from reporting anything else.
func TestCheck_InputBoundToAnAssociationIsCE1613(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) { textbox tbAssoc (label: 'A', attribute: Thing_Other) }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "CE1613") || !strings.Contains(errs[0], "Sales.Thing_Other") {
		t.Fatalf("want one CE1613 error naming the association, got %q", errs)
	}
}

// The script's own entities are judged by the types it declares — the common
// shape is one script with the entity and the page.
func TestCheck_ScriptDeclaredEntityAttributeTypes(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	prog, perrs := visitor.Build(`create enumeration Sales.Size (S 'S', L 'L');
create persistent entity Sales.Shirt (Label: String(50), Size: Enumeration(Sales.Size));
create association Sales.Shirt_Other from Sales.Shirt to Sales.Other;
create page Sales.P2 (title: 'P', layout: Atlas_Core.Atlas_Default, params: ($Shirt: Sales.Shirt)) {
  dataview dv (datasource: $Shirt) {
    textbox tbSize (label: 'S', attribute: Size)
    textbox tbLabel (label: 'L', attribute: Label)
    textbox tbShade (label: 'X', attribute: Shirt_Other/Shade)
  }
}`)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v", perrs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	s := prog.Statements[3].(*ast.CreatePageStmtV3)
	errs := validatePluggableAttributeScopes(ctx, s.Parameters, allPageWidgets(s), sc)
	if len(errs) != 2 || !strings.Contains(strings.Join(errs, "\n"), "tbSize") || !strings.Contains(strings.Join(errs, "\n"), "tbShade") {
		t.Fatalf("want errors on tbSize and tbShade only, got %q", errs)
	}
}

// What the walk cannot place is not judged: an association data source, a page
// variable, an attribute nobody declares.
func TestCheck_AttributeTypeUnjudgedWhenUnknown(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) {
    dataview dv2 (datasource: association Thing_Other) { textbox tb1 (label: 'x', attribute: Shade) }
    textbox tb2 (label: 'x', attribute: NoSuchAttribute)
  }`, nil)
	if len(errs) != 0 {
		t.Fatalf("want nothing judged, got %q", errs)
	}
}

// CE0582: the classic drop-down under the React client, and only there —
// measured on 11.14.0: UseOptimizedClient=Yes reports it, No and MigrationMode
// do not.
func TestCheck_ClassicDropDownUnderReactIsCE0582(t *testing.T) {
	body := `dataview dv (datasource: $Thing) { dropdown ddColor (label: 'C', attribute: Color) }`
	errs := typeErrs(t, typeCheckCtx(t, "Yes"), body, nil)
	if len(errs) != 1 {
		t.Fatalf("want one error under React, got %q", errs)
	}
	for _, want := range []string{"MDL-WIDGET40", "CE0582", "ddColor", "combobox"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}
	for _, mode := range []string{"No", "MigrationMode", ""} {
		if errs := typeErrs(t, typeCheckCtx(t, mode), body, nil); len(errs) != 0 {
			t.Errorf("UseOptimizedClient=%q: want no error, got %q", mode, errs)
		}
	}
}

// Wired into the reference pass a page goes through.
func TestCheck_AttributeTypeIsPartOfThePageReferencePass(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	sc := newScriptContext()
	sc.modules["Sales"] = true
	err := validateWithContext(ctx, typeCheckPage(t, `dataview dv (datasource: $Thing) { textbox tbColor (label: 'C', attribute: Color) }`), sc)
	if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET39") {
		t.Fatalf("want the page refused for MDL-WIDGET39, got %v", err)
	}
}
