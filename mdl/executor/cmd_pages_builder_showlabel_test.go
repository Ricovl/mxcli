// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `textbox (Label: 'Name', ShowLabel: false)` still rendered the label: no
// builder read ShowLabel, and check accepts it, so it was dropped in silence.
// Mendix stores "Show label: No" as no LabelTemplate at all, which the writer
// emits for an input with no label.
func TestInputBuildersHonourShowLabel(t *testing.T) {
	build := map[string]func(*pageBuilder, *ast.WidgetV3) (string, error){
		"textbox": func(pb *pageBuilder, w *ast.WidgetV3) (string, error) {
			x, err := pb.buildTextBoxV3(w)
			return labelOf(x, err, func() string { return x.Label })
		},
		"textarea": func(pb *pageBuilder, w *ast.WidgetV3) (string, error) {
			x, err := pb.buildTextAreaV3(w)
			return labelOf(x, err, func() string { return x.Label })
		},
		"datepicker": func(pb *pageBuilder, w *ast.WidgetV3) (string, error) {
			x, err := pb.buildDatePickerV3(w)
			return labelOf(x, err, func() string { return x.Label })
		},
		"checkbox": func(pb *pageBuilder, w *ast.WidgetV3) (string, error) {
			x, err := pb.buildCheckBoxV3(w)
			return labelOf(x, err, func() string { return x.Label })
		},
		"radiobuttons": func(pb *pageBuilder, w *ast.WidgetV3) (string, error) {
			x, err := pb.buildRadioButtonsV3(w)
			return labelOf(x, err, func() string { return x.Label })
		},
	}
	for kind, fn := range build {
		for _, tc := range []struct {
			name      string
			showLabel any
			want      string
		}{
			{"ShowLabel: false", false, ""},
			{"ShowLabel: No", "No", ""},
			{"ShowLabel: true", true, "Name"},
			{"no ShowLabel (control)", nil, "Name"},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				w := &ast.WidgetV3{Name: "w1", Type: kind, Properties: map[string]any{"Label": "Name"}}
				if tc.showLabel != nil {
					w.Properties["ShowLabel"] = tc.showLabel
				}
				got, err := fn(newPropsBuilder(), w)
				if err != nil {
					t.Fatal(err)
				}
				if got != tc.want {
					t.Errorf("label %q, want %q", got, tc.want)
				}
			})
		}
	}
}

func labelOf(x any, err error, label func() string) (string, error) {
	if err != nil {
		return "", err
	}
	return label(), nil
}
