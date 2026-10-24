// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
)

// ako/mxcli#1560 (mendixlabs/mxcli#1628): a button passing a widget's
// SELECTION to a microflow — Studio Pro stores the argument as a
// Forms$PageVariable naming only the widget — described as
//
//	Action: call microflow WorkflowCommons.ACT_WorkflowUserTask_Assignees_Remove
//
// with the argument gone, so `check` reported CE1571 on a page Studio Pro builds,
// and exec would have written the call with no mapping at all. 13 of the 16
// CE1571 reports describe → check gives on TestApp are this shape: a grid's
// control-bar button passing the grid (UserTask_Assign: grid2, grid8 …) and a
// data view footer button passing a combo box's selection
// (User_WorkflowGroup_Select: comboBox1).

// gridSelection is the stored Variable of those 13 mappings, every slot present
// as Studio Pro writes it.
func gridSelection(widget string) map[string]any {
	return map[string]any{
		"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "",
		"SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": widget,
	}
}

func selectionMapping(param string, widget string) map[string]any {
	return map[string]any{
		"$Type":      "Forms$MicroflowParameterMapping",
		"Parameter":  param,
		"Expression": "",
		"Variable":   gridSelection(widget),
	}
}

func TestDescribeFlowArgReadsWidgetSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action map[string]any
		want   string
	}{
		{
			"microflow action in a control bar",
			map[string]any{
				"$Type": "Forms$MicroflowAction",
				"MicroflowSettings": map[string]any{
					"$Type":     "Forms$MicroflowSettings",
					"Microflow": "WorkflowCommons.ACT_WorkflowUserTask_Assignees_Remove",
					"ParameterMappings": []any{int32(2),
						selectionMapping("WorkflowCommons.ACT_WorkflowUserTask_Assignees_Remove.UserList", "grid2")},
				},
			},
			"call microflow WorkflowCommons.ACT_WorkflowUserTask_Assignees_Remove(UserList = $grid2)",
		},
		{
			"nanoflow action",
			map[string]any{
				"$Type":    "Forms$CallNanoflowClientAction",
				"Nanoflow": "M.NF_Remove",
				"ParameterMappings": []any{int32(2), map[string]any{
					"$Type": "Forms$NanoflowParameterMapping", "Parameter": "M.NF_Remove.Items",
					"Expression": "", "Variable": gridSelection("comboBox1"),
				}},
			},
			"call nanoflow M.NF_Remove(Items = $comboBox1)",
		},
		{
			// Control: a page-parameter binding reads exactly as before.
			"page parameter is unchanged",
			map[string]any{
				"$Type": "Forms$MicroflowAction",
				"MicroflowSettings": map[string]any{
					"Microflow": "M.ACT",
					"ParameterMappings": []any{int32(2), map[string]any{
						"Parameter": "M.ACT.Order",
						"Variable": map[string]any{
							"$Type": "Forms$PageVariable", "PageParameter": "Order", "Widget": "",
						},
					}},
				},
			},
			"call microflow M.ACT(Order = $Order)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderClientActionBaseMDL(nil, tc.action); got != tc.want {
				t.Errorf("described\n  %s\nwant\n  %s", got, tc.want)
			}
		})
	}
}

// A data source stores its arguments the same way: TestApp's
// ConflictedWorkflowDefinitionView has a data view in a grid's control bar whose
// microflow source takes the grid's selection.
func TestDescribeDataSourceArgReadsWidgetSelection(t *testing.T) {
	ds := map[string]any{
		"$Type": "Forms$MicroflowSource",
		"MicroflowSettings": map[string]any{
			"Microflow": "WorkflowCommons.DS_ShowActionOptions",
			"ParameterMappings": []any{int32(2),
				selectionMapping("WorkflowCommons.DS_ShowActionOptions.ViewList", "dataGrid2_2")},
		},
	}
	got := parseDataSource(ds)
	if got == nil {
		t.Fatal("datasource not read")
	}
	if expr, want := dataSourceExpr(nil, got), "microflow WorkflowCommons.DS_ShowActionOptions(ViewList = $dataGrid2_2)"; expr != want {
		t.Errorf("rendered %q, want %q", expr, want)
	}
}

// The write half: `$grid2` naming a widget of the document being built binds as
// that widget — the PageVariable's Widget slot — and not as the Expression
// "$grid2" it fell through to, which binds nothing.
func TestClassifyFlowArgValueNamesDocumentWidget(t *testing.T) {
	pb := builderWithParams("WorkflowUserTask", "Shared")
	pb.localVariables["showAll"] = true
	widgets := map[string]bool{"grid2": true, "comboBox1": true, "Shared": true, "showAll": true}
	pb.isDocumentWidget = func(name string) bool { return widgets[name] }

	for _, tc := range []struct {
		name, value, wantVar, wantKind string
	}{
		{"grid selection", "$grid2", "$grid2", "widget"},
		{"combo box selection", "$comboBox1", "$comboBox1", "widget"},
		// A parameter or page variable is what the name means when both exist:
		// that is how Mendix resolves `$name` in an expression.
		{"parameter wins over a widget of the same name", "$Shared", "$Shared", "parameter"},
		{"page variable wins over a widget of the same name", "$showAll", "$showAll", "local"},
		// Controls: nothing else changes.
		{"no such widget", "$grid9", "", ""},
		{"current object", "$currentObject", "", ""},
		{"path through a widget name", "$grid2/M.Assoc", "", ""},
		{"page parameter", "$WorkflowUserTask", "$WorkflowUserTask", "parameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotVar, gotKind := pb.classifyFlowArgValue(tc.value)
			if gotVar != tc.wantVar || gotKind != tc.wantKind {
				t.Errorf("classifyFlowArgValue(%q) = (%q, %q), want (%q, %q)",
					tc.value, gotVar, gotKind, tc.wantVar, tc.wantKind)
			}
		})
	}
}

// With no document in hand (ALTER on a mutator that cannot answer), nothing is
// a widget, and the argument binds exactly as it did before.
func TestClassifyFlowArgValueWithoutDocumentWidgets(t *testing.T) {
	pb := builderWithParams()
	if v, kind := pb.classifyFlowArgValue("$grid2"); v != "" || kind != "" {
		t.Errorf("classifyFlowArgValue($grid2) with no document = (%q, %q), want (\"\", \"\")", v, kind)
	}
}
