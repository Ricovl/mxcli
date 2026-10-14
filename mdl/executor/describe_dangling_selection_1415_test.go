// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1415: describe page emitted `DataSource: selection grid1`
// for a data view listening to a legacy Forms$DataGrid that it only emits as a
// "NOT re-executable" comment, so check -p and exec refused the script:
//
//	widget 'dataView1': selection DataSource references 'grid1' but no widget
//	with that name exists on this page
//
// The data view cannot be written without its source, so it is commented out
// with the grid and labelled with why, rather than emitted as a statement
// that names a widget the script does not contain.

func danglingSelectionWidgets(gridFirst bool) []rawWidget {
	grid := rawWidget{Type: "Forms$DataGrid", Name: "grid1"}
	dv := rawWidget{
		Type: "Forms$DataView", Name: "dataView1",
		DataSource: &rawDataSource{Type: "selection", Reference: "grid1"},
		Children: []rawWidget{
			{Type: "Forms$TextBox", Name: "textBox1", Content: "Name"},
			// A list nested in the commented data view is not emitted either,
			// so a second data view selecting from it dangles as well.
			{Type: "Forms$ListView", Name: "listView1", DataSource: &rawDataSource{Type: "database", Reference: "M.Item"}},
		},
	}
	dv2 := rawWidget{
		Type: "Forms$DataView", Name: "dataView2",
		DataSource: &rawDataSource{Type: "selection", Reference: "listView1"},
	}
	if gridFirst {
		return []rawWidget{grid, dv, dv2}
	}
	return []rawWidget{dv2, dv, grid}
}

func TestDescribeCommentsOutASelectionOfAnUnemittedWidget(t *testing.T) {
	for _, gridFirst := range []bool{true, false} {
		name := "grid after the data view"
		if gridFirst {
			name = "grid before the data view"
		}
		t.Run(name, func(t *testing.T) {
			ctx, buf := newMockCtx(t)
			describeWidgetBody(ctx, func(c *ExecContext) {
				for _, w := range danglingSelectionWidgets(gridFirst) {
					outputWidgetMDLV3(c, w, 1)
				}
			})
			body := buf.String()
			t.Log("\n" + body)
			for _, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, "selection grid1") || strings.Contains(trimmed, "selection listView1") {
					if !strings.HasPrefix(trimmed, "--") {
						t.Errorf("a selection of a widget describe did not emit is a statement:\n%s", body)
					}
				}
			}
			if !strings.Contains(body, "NOT re-executable") || !strings.Contains(body, "grid1") {
				t.Errorf("the dropped data view is not labelled:\n%s", body)
			}
			script := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + body + "};"
			if _, errs := visitor.Build(script); len(errs) > 0 {
				t.Errorf("describe output does not parse: %v\n%s", errs[0], script)
			}
		})
	}
}

// Control: a data view selecting from a widget describe does emit is left alone.
func TestDescribeKeepsASelectionOfAnEmittedWidget(t *testing.T) {
	ctx, buf := newMockCtx(t)
	describeWidgetBody(ctx, func(c *ExecContext) {
		outputWidgetMDLV3(c, rawWidget{Type: "Forms$ListView", Name: "lv1",
			DataSource: &rawDataSource{Type: "database", Reference: "M.Item"}}, 1)
		outputWidgetMDLV3(c, rawWidget{Type: "Forms$DataView", Name: "dv1",
			DataSource: &rawDataSource{Type: "selection", Reference: "lv1"}}, 1)
	})
	if body := buf.String(); !strings.Contains(body, "dataview dv1 (DataSource: selection lv1") ||
		strings.Contains(body, "NOT re-executable") {
		t.Errorf("a selection of an emitted list was changed:\n%s", body)
	}
}
