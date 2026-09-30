// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#826, the describe half. Two stored bindings describe printed
// nothing for, so describe → exec lost them:
//
//   - a snippet call's Forms$SnippetParameterMapping (TestApp: 27 in
//     WorkflowCommons snippets passing their own parameter, 5 on pages passing a
//     page parameter). Exec then refused the call, the parameter being required;
//   - an input widget's widget-scoped SourceVariable {Widget: dataView1,
//     PageParameter: Account} (PedApp Administration.Account_Edit/Account_New).

func describeParsed(t *testing.T, w map[string]any) string {
	t.Helper()
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, w) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	return strings.TrimSpace(buf.String())
}

func snippetMapping(param string, variable map[string]any) map[string]any {
	return map[string]any{
		"$Type":     "Forms$SnippetParameterMapping",
		"Argument":  "",
		"Parameter": "WorkflowCommons.Snip_Workflow_MoreActions." + param,
		"Variable":  variable,
	}
}

func pageVar(slots map[string]any) map[string]any {
	pv := map[string]any{"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "",
		"SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": ""}
	for k, v := range slots {
		pv[k] = v
	}
	return pv
}

func TestDescribeSnippetCallParameterMappings(t *testing.T) {
	call := func(mappings ...any) map[string]any {
		return map[string]any{
			"$Type": "Forms$SnippetCallWidget", "Name": "snippetCall1",
			"FormCall": map[string]any{
				"$Type":             "Forms$SnippetCall",
				"Form":              "WorkflowCommons.Snip_Workflow_MoreActions",
				"ParameterMappings": append([]any{int32(2)}, mappings...),
			},
		}
	}
	for _, tc := range []struct {
		name string
		w    map[string]any
		want string
	}{
		{"snippet parameter", call(snippetMapping("Workflow", pageVar(map[string]any{"SnippetParameter": "Workflow"}))),
			"Params: (Workflow = $Workflow)"},
		{"page parameter", call(snippetMapping("Workflow", pageVar(map[string]any{"PageParameter": "Wf"}))),
			"Params: (Workflow = $Wf)"},
		{"local variable", call(snippetMapping("Label", pageVar(map[string]any{"LocalVariable": "Caption"}))),
			"Params: (Label = $Caption)"},
		{"two", call(
			snippetMapping("Workflow", pageVar(map[string]any{"PageParameter": "Wf"})),
			snippetMapping("Task", pageVar(map[string]any{"PageParameter": "T"}))),
			"Params: (Workflow = $Wf, Task = $T)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeParsed(t, tc.w); !strings.Contains(got, tc.want) {
				t.Errorf("describe dropped the mapping; want %q in:\n%s", tc.want, got)
			}
		})
	}
	// Control: a call bound by its data context has no mappings and no Params.
	if got := describeParsed(t, call()); strings.Contains(got, "Params") {
		t.Errorf("describe invented Params on an unmapped call: %s", got)
	}
}

func TestDescribeInputWidgetScopedVariable(t *testing.T) {
	input := func(typ string, sv any) map[string]any {
		return map[string]any{
			"$Type": typ, "Name": "w1",
			"AttributeRef":   map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "Administration.Account.FullName"},
			"SourceVariable": sv,
		}
	}
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea", "Forms$CheckBox", "Forms$DatePicker", "Forms$RadioButtonGroup"} {
		t.Run(typ, func(t *testing.T) {
			got := describeParsed(t, input(typ, pageVar(map[string]any{"Widget": "dataView1", "PageParameter": "Account"})))
			if !strings.Contains(got, "Attribute: $dataView1.FullName") {
				t.Errorf("describe dropped the widget-scoped variable; got:\n%s", got)
			}
			// Control: no SourceVariable keeps the context-relative spelling.
			if got := describeParsed(t, input(typ, nil)); strings.Contains(got, "$dataView1") {
				t.Errorf("describe invented a widget scope: %s", got)
			}
		})
	}
}

// A dynamic-text parameter read through a data view prints as `$dataView1.Attr`
// — the Widget slot, not the data view's page parameter beside it, which would
// re-execute as a plain page-parameter binding and drop the Widget.
func TestDescribeTemplateParameterWidgetSlot(t *testing.T) {
	p := map[string]any{
		"AttributeRef":   map[string]any{"Attribute": "MyFirstModule.Car.Brand"},
		"SourceVariable": pageVar(map[string]any{"Widget": "dataView1", "PageParameter": "Car"}),
	}
	name, isLocal := sourceVariableBinding(p)
	if name != "dataView1" || isLocal {
		t.Errorf("sourceVariableBinding = (%q, %v), want (dataView1, false)", name, isLocal)
	}
	// Control: a page parameter binding.
	p["SourceVariable"] = pageVar(map[string]any{"PageParameter": "Car"})
	if name, _ := sourceVariableBinding(p); name != "Car" {
		t.Errorf("sourceVariableBinding = %q, want Car", name)
	}
}
