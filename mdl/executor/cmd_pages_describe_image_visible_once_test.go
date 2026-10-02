// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// customWidget is a CustomWidget whose one property (key) holds value.
func customWidget(widgetID, key string, value map[string]any, extra map[string]any) map[string]any {
	w := map[string]any{
		"$Type": "CustomWidgets$CustomWidget", "Name": "w1",
		"Type": map[string]any{
			"WidgetId": widgetID,
			"ObjectType": map[string]any{"PropertyTypes": []any{int32(2),
				map[string]any{"$ID": "pt-1", "PropertyKey": key}}},
		},
		"Object": map[string]any{"Properties": []any{int32(2),
			map[string]any{"TypePointer": "pt-1", "Value": value}}},
	}
	for k, v := range extra {
		w[k] = v
	}
	return w
}

// A pluggable image's attribute-value visibility printed twice (#721 C, TestApp
// WorkflowCommons.Snip_UserTask_NameColumnWithIcon): the image branch appended
// the conditional settings and then appendAppearanceProps, which appends them
// again. Re-executed, the duplicate key is at best redundant.
func TestDescribeImageVisibilityPrintsOnce(t *testing.T) {
	img := customWidget("com.mendix.widget.web.image.Image", "datasource", map[string]any{"PrimitiveValue": "image"},
		map[string]any{"ConditionalVisibilitySettings": map[string]any{
			"$Type":     "Forms$ConditionalVisibilitySettings",
			"Attribute": "System.WorkflowUserTask.CompletionType",
			"Conditions": []any{int32(2),
				map[string]any{"AttributeValue": "Single", "EditableVisible": true},
				map[string]any{"AttributeValue": "Veto", "EditableVisible": false},
			},
			"Expression": "",
		}})
	got := describeParsed(t, img)
	if n := strings.Count(got, "Visible:"); n != 1 {
		t.Errorf("Visible printed %d times, want once; got:\n%s", n, got)
	}
	exprImg := customWidget("com.mendix.widget.web.image.Image", "datasource", map[string]any{"PrimitiveValue": "image"},
		map[string]any{"ConditionalVisibilitySettings": map[string]any{
			"$Type": "Forms$ConditionalVisibilitySettings", "Expression": "$currentObject/Shown",
		}})
	if n := strings.Count(describeParsed(t, exprImg), "Visible:"); n != 1 {
		t.Errorf("expression Visible printed %d times, want once", n)
	}
}
