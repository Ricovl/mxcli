// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// MDL-WIDGET31 counted only the built-in inputs (editableWidgetTypes), so a
// combo box bound to an attribute inside a default (Editable false) list view —
// rendered read-only by exactly the same list view context — checked clean.
func TestMDLWIDGET31_PluggableInputInListView(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "combobox bound to an attribute in a default list view",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    combobox cb (label: 'Status', attribute: Status)
  }
}`,
			want: 1,
		},
		{
			name: "combobox inside a nested data view",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    dataview dv (datasource: $currentObject) {
      combobox cb (label: 'Status', attribute: Status)
    }
  }
}`,
			want: 1,
		},
		{
			name: "control: list view editable true",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing, editable: true) {
    combobox cb (label: 'Status', attribute: Status)
  }
}`,
			want: 0,
		},
		{
			name: "control: combobox editable Never",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    combobox cb (label: 'Status', attribute: Status, editable: Never)
  }
}`,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := widgetViolations(t, tc.src, "MDL-WIDGET31")
			if len(got) != tc.want {
				t.Fatalf("MDL-WIDGET31: got %d violation(s), want %d: %#v", len(got), tc.want, got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Message, "`cb`") {
				t.Errorf("message should name the combo box: %s", got[0].Message)
			}
		})
	}
}

// A pluggable widget that binds an attribute only to SHOW it — a progress bar —
// has no Editability system property, so the list view's context changes
// nothing for it. The installed package says which is which.
func TestWidgetDeclaresEditability_FromThePackage(t *testing.T) {
	const project = "../../testdata/pedapp/PedApp.mpr"
	for id, want := range map[string]bool{
		"com.mendix.widget.custom.slider.Slider":           true,
		"com.mendix.widget.web.combobox.Combobox":          true,
		"com.mendix.widget.custom.progressbar.ProgressBar": false,
	} {
		has, known := widgetDeclaresEditability(project, id)
		if !known {
			t.Fatalf("%s: package not read from %s", id, project)
		}
		if has != want {
			t.Errorf("%s: declares Editability = %v, want %v", id, has, want)
		}
	}
}

// No registry: the rule keeps its built-in behaviour and does not guess.
func TestPluggableEditableInput_NilRegistry(t *testing.T) {
	c := &ast.WidgetV3{Type: "combobox", Name: "cb", Properties: map[string]any{"Attribute": "Status"}}
	if pluggableEditableInput(c, nil) {
		t.Error("a nil registry reported a pluggable input")
	}
}
