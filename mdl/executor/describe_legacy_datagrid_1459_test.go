// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1459: DESCRIBE printed a legacy Forms$DataGrid as one opaque
// `-- Forms$DataGrid (grid1)` line, so its columns, search fields and control-bar
// buttons had no names in the output and `alter page … drop column14` could not
// be written from it. The grid is still unauthorable (MDL-WIDGET38), so the
// structure is printed as comments — readable and addressable, never executed.

func legacyText(s string) map[string]any {
	return map[string]any{
		"$Type": "Texts$Text",
		"Items": []any{int32(3), map[string]any{
			"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": s,
		}},
	}
}

func legacyAttrRef(qn string) map[string]any {
	return map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": qn, "EntityRef": nil}
}

// legacyDataGridBSON is a Forms$DataGrid in the shape Studio Pro stores it
// (property names from modelsdk/gen/pages: Columns of Forms$GridColumn with
// Caption + AttributeRef, ControlBar.Items of Forms$Grid*Button, and the search
// bar under DataSource.SearchBar.Items).
func legacyDataGridBSON() map[string]any {
	return map[string]any{
		"$Type": "Forms$DataGrid",
		"Name":  "grid1",
		"DataSource": map[string]any{
			"$Type":     "Forms$GridXPathSource",
			"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "Shop.Order"},
			"SearchBar": map[string]any{
				"$Type": "Forms$SearchBar",
				"Type":  "FoldableOpen",
				"Items": []any{int32(2),
					map[string]any{
						"$Type": "Forms$ComparisonSearchField", "Name": "searchField1",
						"Caption": legacyText("Number"), "Type": "Normal", "Operator": "Equal",
						"AttributeRef": legacyAttrRef("Shop.Order.Number"),
					},
					map[string]any{
						"$Type": "Forms$DropDownSearchField", "Name": "searchField2",
						"Caption": legacyText("Status"), "Type": "Hidden",
						"AttributeRef": legacyAttrRef("Shop.Order.Status"),
					},
				},
			},
			"SortBar":         map[string]any{"$Type": "Forms$GridSortBar", "SortItems": []any{int32(2)}},
			"XPathConstraint": "",
		},
		"ControlBar": map[string]any{
			"$Type": "Forms$GridControlBar",
			"Items": []any{int32(2),
				map[string]any{"$Type": "Forms$GridSearchButton", "Name": "searchButton1", "Caption": legacyText("Search")},
				map[string]any{"$Type": "Forms$GridNewButton", "Name": "newButton1", "Caption": legacyText("New")},
			},
		},
		"Columns": []any{int32(2),
			map[string]any{
				"$Type": "Forms$GridColumn", "Name": "column1", "Caption": legacyText("Order number"),
				"AttributeRef": legacyAttrRef("Shop.Order.Number"),
			},
			map[string]any{
				"$Type": "Forms$GridColumn", "Name": "column14", "Caption": legacyText("Internal note"),
				"AttributeRef": legacyAttrRef("Shop.Order.Note"),
			},
		},
	}
}

func TestDescribeLegacyDataGridPrintsItsStructure(t *testing.T) {
	ctx, buf := newMockCtx(t)
	raw := parseRawWidget(ctx, legacyDataGridBSON())
	describeWidgetBody(ctx, func(c *ExecContext) {
		for _, w := range raw {
			outputWidgetMDLV3(c, w, 1)
		}
	})
	body := buf.String()
	t.Log("\n" + body)

	for _, want := range []string{
		"grid1", "NOT re-executable", "Shop.Order",
		"searchButton1", "newButton1",
		"column1", "'Order number'", "Number",
		"column14", "'Internal note'", "Note",
		"searchField1", "Comparison", "searchField2", "DropDown", "Hidden", "Status",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("describe output lacks %q:\n%s", want, body)
		}
	}
	// Every line is a comment: the grid cannot be built, so a describe → exec
	// round trip must not create anything in its place.
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			t.Errorf("legacy data grid line is not a comment: %q", line)
		}
	}
	script := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + body + "};"
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Errorf("describe output does not parse: %v\n%s", errs[0], script)
	}
}
