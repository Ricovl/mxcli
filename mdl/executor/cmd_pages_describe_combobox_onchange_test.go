// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// A combo box with an `OnChange:` described back without its `Attribute:` (and,
// in association mode, without `CaptionAttribute:`): OnChange routed it into the
// generic pluggable branch meant for Slider/StarRating, which knows nothing of a
// combo box's binding. The stored widget was correct; describe → exec lost it.
func TestDescribeComboboxWithOnChangeKeepsItsBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    rawWidget
		want []string
	}{
		{
			name: "enumeration mode",
			w: rawWidget{
				Type: "CustomWidgets$CustomWidget", RenderMode: "combobox", Name: "cbColor",
				WidgetID: "com.mendix.widget.web.combobox.Combobox",
				Caption:  "Color", Content: "Color", OnChange: "microflow M.OnColor",
			},
			want: []string{"combobox cbColor", "Attribute: Color", "OnChange: microflow M.OnColor"},
		},
		{
			name: "association mode",
			w: rawWidget{
				Type: "CustomWidgets$CustomWidget", RenderMode: "combobox", Name: "cbEmp",
				WidgetID: "com.mendix.widget.web.combobox.Combobox",
				Content:  "M.Task_Employee", CaptionAttribute: "Name",
				DataSource: &rawDataSource{Type: "database", Reference: "M.Employee"},
				OnChange:   "microflow M.OnEmp",
			},
			want: []string{"combobox cbEmp", "Attribute: M.Task_Employee", "CaptionAttribute: Name", "OnChange: microflow M.OnEmp"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			outputWidgetMDLV3(&ExecContext{Output: &buf}, tc.w, 0)
			out := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("description lacks %q:\n%s", want, out)
				}
			}
		})
	}
}
