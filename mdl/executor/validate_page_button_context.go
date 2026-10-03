// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for button action context on pages and
// snippets. A DataGrid/Gallery control bar is not row-scoped, so $currentObject
// has no value there; passing it to a button action builds to CE1571 "No
// argument has been selected for parameter …". This heuristic catches it before
// MxBuild does. See docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidatePageButtonContext warns (MDL-BUTTON01) when a button inside a control
// bar passes $currentObject to its action. A control bar sits above the grid and
// is not bound to a row, so $currentObject is unbound there — MxBuild reports
// CE1571. Row-scoped buttons (inside a grid column / list item) are fine, and so
// is a control bar of a grid nested inside a data view or list view item, where
// $currentObject is the enclosing object (ako/mxcli#953).
func ValidatePageButtonContext(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		if label, widgets, ok := documentWidgets(stmt); ok {
			out = append(out, checkButtonContextTree(widgets, "", false, label)...)
		}
	}
	return out
}

// checkButtonContextTree walks the widget tree, carrying the name of the data
// widget whose control bar it is inside ("" when it is not inside one), and
// whether an enclosing data container already supplies an object context.
//
// The name is carried rather than just a flag because it IS the remedy: a data
// widget's selection is addressed by the widget's own name, so `$dgMaterials` is
// only spellable from here. Advice that stops at "move it into a column" sends
// an author looking for syntax that does not need to exist — which is how
// mendixlabs/mxcli#1082 was filed.
//
// inContext is the enclosing-container half (ako/mxcli#953): a grid nested in a
// data view, a list view item or a grid column sits in that container's object
// context, and its control bar's $currentObject is that object — mxbuild builds
// it clean. The grid's OWN data source never scopes its control bar, so the
// control bar inherits the context from above the grid, not from the grid.
func checkButtonContextTree(widgets []*ast.WidgetV3, controlBarOf string, inContext bool, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if controlBarOf != "" && !inContext {
			if a := w.GetAction(); a != nil {
				out = append(out, checkControlBarAction(a, w.Name, controlBarOf, locationPrefix)...)
			}
		}
		childContext := inContext || isObjectContextContainer(w)
		for _, c := range w.Children {
			childOf, ctx := controlBarOf, childContext
			if c != nil && strings.EqualFold(c.Type, "controlbar") {
				childOf, ctx = w.Name, inContext
			}
			out = append(out, checkButtonContextTree([]*ast.WidgetV3{c}, childOf, ctx, locationPrefix)...)
		}
	}
	return out
}

// isObjectContextContainer reports whether w gives its (non-control-bar)
// children a current object: a data view's object, a list view or gallery
// item, a grid row (column content).
func isObjectContextContainer(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "dataview", "listview", "gallery", "templategrid", "datagrid", "datagrid2":
		return true
	}
	return false
}

// checkControlBarAction flags any $currentObject argument on an action (and its
// chained THEN action) that sits inside a control bar.
func checkControlBarAction(a *ast.ActionV3, widgetName, controlBarOf, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for a != nil {
		for _, arg := range a.Args {
			if s, ok := arg.Value.(string); ok && strings.EqualFold(s, "$currentObject") {
				out = append(out, linter.Violation{
					RuleID:   "MDL-BUTTON01",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"%s: control-bar button `%s` passes $currentObject to its %s action, but a control bar is not row-scoped — $currentObject is unbound there (CE1571)",
						locationPrefix, widgetName, a.Type),
					Suggestion: controlBarSuggestion(controlBarOf),
				})
			}
		}
		a = a.ThenAction
	}
	return out
}

// controlBarSuggestion names the remedy that actually applies to a control bar.
//
// The selection comes first because it is the one that keeps the button where
// the author put it: a data widget with `Selection:` set exposes the selected
// object as `$<widgetName>`, which an action takes as an ordinary argument.
func controlBarSuggestion(controlBarOf string) string {
	if controlBarOf == "" {
		return "Move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter instead of $currentObject."
	}
	return fmt.Sprintf(
		"Pass the selection of `%s` instead — `Action: microflow M.F(Param = $%s)`, with `Selection:` set on the widget. "+
			"Or move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter.",
		controlBarOf, controlBarOf)
}
