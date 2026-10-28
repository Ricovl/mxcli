// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateLayoutGridColumnVisible warns about `Visible:` on a column inside a
// layout-grid row. Forms$LayoutGridColumn has no ConditionalVisibilitySettings
// — the metamodel has no such property, and none of the 732 Studio
// Pro-authored columns in TestApp + PedApp carries the key — so
// buildLayoutGridColumnV3 has nowhere to put it and it is dropped. `Visible` is
// in the cross-widget vocabulary, so MDL-WIDGET07 never saw it
// (mendixlabs/mxcli#1461). The row has visibility; a top-level `column` is
// wrapped in a container, which carries it, so neither is reported.
//
// A warning, as MDL-WIDGET07 is, so no language version is needed: the write
// is unchanged, only no longer silent.
func validateLayoutGridColumnVisible(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "row") {
		return nil
	}
	var out []linter.Violation
	n := 0
	for _, col := range w.Children {
		if col == nil || !strings.EqualFold(col.Type, "column") {
			continue
		}
		n++ // Mendix stores no name on a column, so it is named by position.
		// Visible: in any form — expression (Visible / VisibleIf) or
		// `Attr in (…)` (VisibleWhen).
		has := false
		for _, key := range sortedPropertyKeys(col) {
			switch strings.ToLower(key) {
			case "visible", "visibleif", "visiblewhen":
				has = true
			}
		}
		if !has {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET07",
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("%s: layout-grid column %d of its row: `Visible` cannot be stored on a layout-grid column "+
				"(Forms$LayoutGridColumn has no visibility setting) and will be silently dropped on write",
				locationPrefix, n),
			Suggestion: "put Visible: on the row, or wrap the column's content in a `container` with Visible:",
		})
	}
	return out
}
