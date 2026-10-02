// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A widget placed directly in a snippet (or page), outside every data
// container, binds an attribute of a PARAMETER: Studio Pro stores the
// AttributeRef beside a Forms$PageVariable naming the parameter in its slot and
// no Widget. Measured on TestApp (Mendix 11.x, WorkflowCommons):
//
//	Snip_TaskDashboard_Header  combobox dropDown2 attributeEnumeration:
//	    AttributeRef {Attribute: WorkflowCommons.DashboardContext.TimeFrame}
//	    SourceVariable {SnippetParameter: DashboardContext, Widget: ""}
//	Snip_UserTask_NameColumnWithIcon  image image2 ConditionalVisibilitySettings:
//	    Attribute: System.WorkflowUserTask.CompletionType, Conditions […]
//	    SourceVariable {SnippetParameter: WorkflowUserTask, Widget: ""}
//
// and on the built-in inputs of Snip_WorkflowUserTaskView_Details (a date
// picker, text areas, a radio button group) the same way. MDL spells it
// `$Param.Attr`, the form a template parameter already used for a parameter.

// rootPB is a builder at the top of a snippet (or page) declaring $Task: M.Job,
// with no data container around the widget.
func rootPB(isSnippet bool) *pageBuilder {
	pb := scopedPB(isSnippet)
	pb.argCtx = atDocumentRoot()
	return pb
}

func TestVisibleWhen_ThroughParameter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isSnippet bool
		want      pages.WidgetVariable
	}{
		{"snippet parameter", true, pages.WidgetVariable{Variable: "Task", Kind: "snippet"}},
		{"page parameter", false, pages.WidgetVariable{Variable: "Task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cvs, err := buildVisibleWhen(t, rootPB(tc.isSnippet), "$Task.Status", "Running")
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if cvs == nil {
				t.Fatal("no ConditionalVisibilitySettings — the condition was dropped (widget always visible)")
			}
			if cvs.Attribute != "M.Job.Status" {
				t.Errorf("Attribute = %q, want M.Job.Status", cvs.Attribute)
			}
			if cvs.SourceVariable == nil || *cvs.SourceVariable != tc.want {
				t.Errorf("SourceVariable = %+v, want %+v", cvs.SourceVariable, tc.want)
			}
		})
	}
	// Controls: a bare name at the root has no object, and a `$name` that is
	// no parameter names nothing — both still refused.
	for _, attr := range []string{"Status", "$Nope.Status"} {
		if _, err := buildVisibleWhen(t, rootPB(true), attr, "Running"); err == nil {
			t.Errorf("Visible: %s in (Running) at the snippet root: built; want a refusal", attr)
		}
	}
	// Control: inside a data container a bare name keeps no SourceVariable.
	cvs, err := buildVisibleWhen(t, visibleWhenPB("M.Job"), "Status", "Running")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cvs.SourceVariable != nil {
		t.Errorf("context-relative Visible wrote SourceVariable %+v, want none", *cvs.SourceVariable)
	}
}

func TestInputAttributeThroughParameter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isSnippet bool
		want      pages.WidgetVariable
	}{
		{"snippet parameter", true, pages.WidgetVariable{Variable: "Task", Kind: "snippet"}},
		{"page parameter", false, pages.WidgetVariable{Variable: "Task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := rootPB(tc.isSnippet).buildWidgetV3(&ast.WidgetV3{
				Name: "tb", Type: "textbox", Properties: map[string]any{"Attribute": "$Task.Title"},
			})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			tb, ok := w.(*pages.TextBox)
			if !ok {
				t.Fatalf("built %T, want *pages.TextBox", w)
			}
			if tb.AttributePath != "M.Job.Title" {
				t.Errorf("AttributePath = %q, want M.Job.Title", tb.AttributePath)
			}
			if tb.SourceVariable == nil || *tb.SourceVariable != tc.want {
				t.Errorf("SourceVariable = %+v, want %+v", tb.SourceVariable, tc.want)
			}
		})
	}
	// Control: a name that is neither a parameter nor an enclosing data view.
	_, err := rootPB(true).buildWidgetV3(&ast.WidgetV3{
		Name: "tb", Type: "textbox", Properties: map[string]any{"Attribute": "$Nope.Title"},
	})
	if err == nil {
		t.Error("Attribute: $Nope.Title: built; want a refusal")
	}
}

// paramScopeEngine is scopeEngine at the top of a snippet declaring
// $Ctx: Sales.Invoice — no data container around the widget.
func paramScopeEngine(t *testing.T) *pageBuilder {
	t.Helper()
	pb := scopeEngine(t).pageBuilder
	pb.entityContext = ""
	pb.isSnippet = true
	pb.argCtx = atDocumentRoot()
	pb.paramScope = map[string]model.ID{"Ctx": "e-invoice"}
	pb.paramEntityNames["Ctx"] = "Sales.Invoice"
	return pb
}

// widgetValueFor returns the WidgetValue whose AttributeRef names attr.
func widgetValueFor(v any, attr string) bson.D {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if e.Key == "AttributeRef" {
				if ref, ok := e.Value.(bson.D); ok {
					for _, f := range ref {
						if f.Key == "Attribute" && f.Value == attr {
							return x
						}
					}
				}
			}
		}
		for _, e := range x {
			if d := widgetValueFor(e.Value, attr); d != nil {
				return d
			}
		}
	case bson.A:
		for _, e := range x {
			if d := widgetValueFor(e, attr); d != nil {
				return d
			}
		}
	}
	return nil
}

func dGet(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func TestComboBoxAttributeThroughParameter(t *testing.T) {
	pb := paramScopeEngine(t)
	w, err := pb.buildPluggable(comboDef(t), &ast.WidgetV3{Type: "combobox", Name: "cmb",
		Properties: map[string]any{"Attribute": "$Ctx.Reference"}})
	if err != nil {
		t.Fatalf("buildPluggable: %v", err)
	}
	cw := w.(*pages.CustomWidget)
	val := widgetValueFor(cw.RawObject, "Sales.Invoice.Reference")
	if val == nil {
		t.Fatal("no WidgetValue binds Sales.Invoice.Reference — the binding was dropped")
	}
	sv, ok := dGet(val, "SourceVariable").(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want a Forms$PageVariable naming the snippet parameter", dGet(val, "SourceVariable"))
	}
	for k, want := range map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "Ctx",
		"PageParameter": "", "LocalVariable": "", "Widget": ""} {
		if got := dGet(sv, k); got != want {
			t.Errorf("SourceVariable.%s = %#v, want %#v", k, got, want)
		}
	}

	// Controls: a bare name at the root is refused, as before, and so is a
	// `$name` that is no parameter.
	for _, attr := range []string{"Reference", "$Nope.Reference"} {
		_, err := paramScopeEngine(t).buildPluggable(comboDef(t), &ast.WidgetV3{Type: "combobox", Name: "cmb",
			Properties: map[string]any{"Attribute": attr}})
		if err == nil {
			t.Errorf("combobox (Attribute: %s) at the snippet root: built; want a refusal", attr)
		}
	}
}

// Check accepts what exec accepts and refuses what it refuses: `$Param.Attr`
// naming a declared entity parameter is clean at the snippet root; a name that
// is no parameter is MDL-WIDGET34; a combo box cannot read an enclosing data
// view that way (only the inputs can).
func TestCheckParameterBindings(t *testing.T) {
	widget34 := func(body string) []string {
		t.Helper()
		src := "create snippet M.S (Params: ($Ctx: M.A)) {\n" + body + "\n};"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parsing %q: %v", src, errs)
		}
		var out []string
		for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], LoadWidgetRegistry("")) {
			if v.RuleID == "MDL-WIDGET34" {
				out = append(out, v.Message)
			}
		}
		return out
	}
	for _, body := range []string{
		"combobox c1 (Attribute: $Ctx.Status)",
		"textbox t1 (Attribute: $Ctx.Name)",
		"image i1 (Visible: $Ctx.Status in (Running))",
		"dataview dv1 (DataSource: $Ctx) { textbox t1 (Attribute: $dv1.Name) }",
	} {
		if got := widget34(body); len(got) != 0 {
			t.Errorf("%s: %v, want clean", body, got)
		}
	}
	for _, body := range []string{
		"combobox c1 (Attribute: $Nope.Status)",
		"textbox t1 (Attribute: $Nope.Name)",
		"image i1 (Visible: $Nope.Status in (Running))",
		"dataview dv1 (DataSource: $Ctx) { combobox c1 (Attribute: $dv1.Status) }",
	} {
		if got := widget34(body); len(got) == 0 {
			t.Errorf("%s: clean, want MDL-WIDGET34", body)
		}
	}
	// Control: a bare attribute at the root is still refused (unchanged).
	if got := widget34("textbox t1 (Attribute: Name)"); len(got) == 0 {
		t.Error("bare attribute at the snippet root: clean, want MDL-WIDGET34")
	}
}

// The image keeps its attribute-value visibility through the parameter too.
func TestPluggableVisibleWhenThroughParameter(t *testing.T) {
	pb := paramScopeEngine(t)
	_, err := pb.buildPluggable(comboDef(t), &ast.WidgetV3{Type: "combobox", Name: "cmb",
		Properties: map[string]any{
			"Attribute":   "$Ctx.Reference",
			"VisibleWhen": &ast.VisibleWhenV3{Attribute: "$Ctx.Label", Values: []string{"true"}},
		}})
	if err == nil {
		// Label is a String in this domain model: refused with the reason.
		t.Fatalf("built a value condition on a String attribute")
	}
	if !strings.Contains(err.Error(), "not a Boolean or enumeration") {
		t.Errorf("err = %v, want the attribute-type refusal (the parameter resolved)", err)
	}
}
