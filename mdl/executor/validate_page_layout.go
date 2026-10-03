// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for the layout-grid wrapping of edit/new
// forms. Mirrors the MPR010 lint rule (mdl/linter/rules/dataview_layout_grid.go)
// but works on the MDL AST so `mxcli check` warns while authoring, before the
// page is written. A parameter-bound DataView's label/input widths are expressed
// in Bootstrap grid columns and only render correctly inside a layoutgrid.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidatePageLayoutGrid warns (MPR010) when a form DataView — one containing
// input widgets — is not nested inside a layout grid, since its label/input widths
// are laid out in grid columns (independent of the data source). Any layoutgrid
// ancestor satisfies the rule (grid → column → container → dataview is fine); a
// display-only / container DataView (no inputs) is not flagged.
//
// nativeLayout, which may be nil, tells a native layout; a page on one is
// skipped. The advice is web-only (ako/mxcli#962). Label and input widths in
// Bootstrap grid columns are the web client's; a native page is rendered by
// React Native. Measured on mxbuild 11.13.0 (PedApp copy): a form DataView
// placed directly on an Atlas_Core.NativePhone_Default page builds clean, and
// FOLLOWING the advice there — wrapping it in a layoutgrid — is CE6858 "Please
// update Atlas UI to version 2.4 or higher to use Layout Grid on Native pages".
func ValidatePageLayoutGrid(prog *ast.Program, nativeLayout func(layout string) bool) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		label, widgets, ok := documentWidgets(stmt)
		if !ok {
			continue
		}
		found := checkLayoutGridTree(widgets, false, label)
		// Asked only when there is something to report, so a script whose
		// pages are fine never opens the project for this.
		if page, isPage := stmt.(*ast.CreatePageStmtV3); isPage && len(found) > 0 &&
			nativeLayout != nil && page.Layout != "" && nativeLayout(page.Layout) {
			continue
		}
		out = append(out, found...)
	}
	return out
}

// projectNativeLayouts answers "is this layout native?" from the project that
// project() opens — nil when there is none. A layout the project does not hold
// (one the script creates) reads as web, which keeps a check as loud as it was.
func projectNativeLayouts(project func() backend.FullBackend) func(string) bool {
	var ctx *ExecContext
	return func(layout string) bool {
		if ctx == nil {
			b := project()
			if b == nil {
				return false
			}
			ctx = &ExecContext{Backend: b}
		}
		return layoutIsNative(ctx, layout)
	}
}

func checkLayoutGridTree(widgets []*ast.WidgetV3, underGrid bool, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if !underGrid && isFormDataViewAST(w) {
			out = append(out, linter.Violation{
				RuleID:   "MPR010",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf(
					"%s: DataView `%s` contains input fields but is not inside a layout grid — label and input widths only render correctly inside a layoutgrid",
					locationPrefix, w.Name),
				Suggestion: fmt.Sprintf("Wrap dataview `%s` in `layoutgrid { row { column (desktopwidth: autofill) { … } } }`", w.Name),
			})
		}
		childUnder := underGrid || strings.EqualFold(w.Type, "layoutgrid")
		out = append(out, checkLayoutGridTree(w.Children, childUnder, locationPrefix)...)
	}
	return out
}

// isFormDataViewAST reports whether w is a DataView containing at least one input
// widget — a form, which needs the layout-grid context for its label/input widths
// (independent of the data source).
func isFormDataViewAST(w *ast.WidgetV3) bool {
	if !strings.EqualFold(w.Type, "dataview") {
		return false
	}
	return astSubtreeHasInput(w.Children)
}

// astSubtreeHasInput reports whether any widget in the subtrees is an input
// widget, stopping at a nested DataView boundary.
func astSubtreeHasInput(widgets []*ast.WidgetV3) bool {
	for _, c := range widgets {
		if c == nil {
			continue
		}
		if isInputWidgetAST(c) {
			return true
		}
		if strings.EqualFold(c.Type, "dataview") {
			continue
		}
		if astSubtreeHasInput(c.Children) {
			return true
		}
	}
	return false
}

// isInputWidgetAST reports whether w is a form input widget (native or the
// pluggable combobox).
func isInputWidgetAST(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "textbox", "textarea", "datepicker", "dropdown", "checkbox",
		"radiobuttons", "combobox", "referenceselector", "inputreferencesetselector":
		return true
	}
	return false
}
