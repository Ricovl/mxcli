// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// The describe half of `$Param.Attr` (cmd_pages_parameter_binding.go): a
// binding whose SourceVariable names a page or snippet parameter — no Widget —
// prints in the form exec reads back into the same SourceVariable. Before, the
// attribute printed bare, and re-executing it either failed (no enclosing
// object at the snippet root) or wrote it with a null SourceVariable.

func TestDescribeInputParameterBinding(t *testing.T) {
	input := func(sv any) map[string]any {
		return map[string]any{
			"$Type": "Forms$DatePicker", "Name": "datePicker1",
			"AttributeRef":   map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "WorkflowCommons.WorkflowUserTaskView.DueDate"},
			"SourceVariable": sv,
		}
	}
	got := describeParsed(t, input(pageVar(map[string]any{"SnippetParameter": "WorkflowUserTaskView"})))
	if !strings.Contains(got, "Attribute: $WorkflowUserTaskView.DueDate") {
		t.Errorf("describe dropped the parameter source; got:\n%s", got)
	}
	got = describeParsed(t, input(pageVar(map[string]any{"PageParameter": "Task"})))
	if !strings.Contains(got, "Attribute: $Task.DueDate") {
		t.Errorf("describe dropped the page-parameter source; got:\n%s", got)
	}
	// Control: no SourceVariable keeps the context-relative spelling.
	if got := describeParsed(t, input(nil)); !strings.Contains(got, "Attribute: DueDate") {
		t.Errorf("context-relative binding: got:\n%s", got)
	}
}

func TestDescribeComboBoxParameterBinding(t *testing.T) {
	enum := func(sv any) map[string]any {
		return customWidget("com.mendix.widget.web.combobox.Combobox", "attributeEnumeration", map[string]any{
			"AttributeRef":   map[string]any{"Attribute": "WorkflowCommons.DashboardContext.TimeFrame"},
			"SourceVariable": sv,
		}, nil)
	}
	got := describeParsed(t, enum(pageVar(map[string]any{"SnippetParameter": "DashboardContext"})))
	if !strings.Contains(got, "Attribute: $DashboardContext.TimeFrame") {
		t.Errorf("describe dropped the parameter source; got:\n%s", got)
	}
	// Control: read from the enclosing object.
	if got := describeParsed(t, enum(nil)); strings.Contains(got, "$DashboardContext") {
		t.Errorf("describe invented a parameter source:\n%s", got)
	}
}

// The image's attribute-value visibility reads the parameter.
func TestDescribeImageVisibleWhenThroughParameter(t *testing.T) {
	img := func(sv any) map[string]any {
		return customWidget("com.mendix.widget.web.image.Image", "datasource", map[string]any{"PrimitiveValue": "image"},
			map[string]any{"ConditionalVisibilitySettings": map[string]any{
				"$Type":     "Forms$ConditionalVisibilitySettings",
				"Attribute": "System.WorkflowUserTask.CompletionType",
				"Conditions": []any{int32(2),
					map[string]any{"AttributeValue": "Single", "EditableVisible": true},
					map[string]any{"AttributeValue": "Veto", "EditableVisible": false},
				},
				"Expression":     "",
				"SourceVariable": sv,
			}})
	}
	got := describeParsed(t, img(pageVar(map[string]any{"SnippetParameter": "WorkflowUserTask"})))
	const want = `Visible: $WorkflowUserTask.CompletionType in ("Single")`
	if n := strings.Count(got, want); n != 1 {
		t.Errorf("%q printed %d times, want once; got:\n%s", want, n, got)
	}
	// Control: read from the enclosing object — bare.
	got = describeParsed(t, img(nil))
	if !strings.Contains(got, `Visible: CompletionType in ("Single")`) || strings.Contains(got, "$WorkflowUserTask") {
		t.Errorf("context-relative visibility; got:\n%s", got)
	}
}
