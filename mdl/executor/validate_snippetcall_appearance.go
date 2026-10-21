// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// A snippet call renders no element of its own (mendixlabs/mxcli#1511).
//
// The snippet's widgets are inlined into the parent, so the Class, Style,
// DynamicClasses and design properties a Forms$SnippetCallWidget carries have
// nothing to land on. Measured on Mendix 11.15.0 with `run --local` and
// `playwright check`: a snippetcall carrying Class 'probe-x', Style
// 'border: 3px solid red', DynamicClasses 'probe-dyn' and the design properties
// 'Align self': 'Right' / 'Hide on': Tablet produced no element with any of
// them — the snippet's own container was a DIRECT child of the enclosing
// container — while a container on the same page with the same four settings
// carried every one. check, exec and mx check (0 errors) were all clean.
//
// A warning, not an error: the model is valid and Studio Pro stores it, the
// setting is only useless. The lint rule MPR013 reports the same for a project
// already holding such a call.

const snippetCallAppearanceRule = "MDL-WIDGET48"

const snippetCallAppearanceSuggestion = "Put the class or style on a container inside the snippet, " +
	"or wrap the snippet call in a container and style that container."

// snippetCallAppearanceMessage is the one wording the CREATE and the stored
// (SET / ALTER STYLING) paths share.
func snippetCallAppearanceMessage(widget string, fields []string) string {
	return fmt.Sprintf("snippet call `%s` sets %s, but a snippet call renders no element of its own "+
		"(the snippet's widgets are inlined into the parent), so the setting never reaches the page",
		widget, strings.Join(fields, ", "))
}

// validateSnippetCallAppearance reports a snippetcall the script writes with
// appearance settings — CREATE PAGE / SNIPPET and ALTER PAGE INSERT / REPLACE.
func validateSnippetCallAppearance(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "snippetcall") {
		return nil
	}
	var fields []string
	if strings.TrimSpace(w.GetClass()) != "" {
		fields = append(fields, "Class")
	}
	if strings.TrimSpace(w.GetStyle()) != "" {
		fields = append(fields, "Style")
	}
	if strings.TrimSpace(w.GetDynamicClasses()) != "" {
		fields = append(fields, "DynamicClasses")
	}
	for _, dp := range w.GetDesignProperties() {
		fields = append(fields, fmt.Sprintf("design property '%s'", dp.Key))
	}
	if len(fields) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:     snippetCallAppearanceRule,
		Severity:   linter.SeverityWarning,
		Message:    locationPrefix + ": " + snippetCallAppearanceMessage(w.Name, fields),
		Suggestion: snippetCallAppearanceSuggestion,
	}}
}

// validateStoredSnippetCallAppearance is the same finding for the statements
// that name a widget already STORED — `alter page … set (Class: …) on sc` (the
// reported case) and `alter styling … widget sc set …`. Whether the target is
// a snippet call lives only in the document, so this runs in the reference pass
// and adds warnings; it never blocks. A document that will not open, or a
// target that does not resolve, is silent: those are other passes' findings.
func validateStoredSnippetCallAppearance(ctx *ExecContext, prog *ast.Program, sc *scriptContext) {
	if prog == nil || sc == nil || !ctx.Connected() {
		return
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return
	}
	opened := map[model.ID]backend.PageMutator{}
	open := func(unitID model.ID) backend.PageMutator {
		m, seen := opened[unitID]
		if !seen {
			m, _ = ctx.Backend.OpenPageForMutation(unitID)
			opened[unitID] = m
		}
		return m
	}

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AlterPageStmt:
			if !hasSetPropertyOp(s) || alterTargetComesFromScript(sc, s) {
				continue
			}
			unitID, _, kind, err := resolveAlterPageUnit(ctx, s, h)
			if err != nil {
				continue
			}
			m := open(unitID)
			if m == nil {
				continue
			}
			for _, op := range s.Operations {
				set, ok := op.(*ast.SetPropertyOp)
				if !ok || set.Target.Widget == "" || set.Target.IsColumn() || !storedIsSnippetCall(m, set.Target.Widget) {
					continue
				}
				var fields []string
				for name, value := range set.Properties {
					if setsSnippetCallAppearance(ctx, m, set.Target.Widget, name, value) {
						fields = append(fields, name)
					}
				}
				warnSnippetCallAppearance(sc, fmt.Sprintf("alter %s %s", kind, s.PageName.String()), set.Target.Widget, fields)
			}
		case *ast.AlterStylingStmt:
			kind := strings.ToLower(s.ContainerType)
			var unitID model.ID
			switch kind {
			case "page":
				if sc.pages[s.ContainerName.String()] {
					continue
				}
				pg, err := findPageByName(ctx, s.ContainerName, h)
				if err != nil {
					continue
				}
				unitID = pg.ID
			case "snippet":
				if sc.snippets[s.ContainerName.String()] {
					continue
				}
				snp, _, err := findSnippetByName(ctx, s.ContainerName, h)
				if err != nil {
					continue
				}
				unitID = snp.ID
			default:
				continue
			}
			m := open(unitID)
			if m == nil || !storedIsSnippetCall(m, s.WidgetName) {
				continue
			}
			var fields []string
			for _, a := range s.Assignments {
				if a.Property == "" || (a.IsCSS && strings.TrimSpace(a.Value) == "") ||
					(a.IsToggle && !a.ToggleOn) {
					continue
				}
				if a.IsCSS {
					fields = append(fields, a.Property)
				} else {
					fields = append(fields, fmt.Sprintf("design property '%s'", a.Property))
				}
			}
			warnSnippetCallAppearance(sc, fmt.Sprintf("alter styling on %s %s", kind, s.ContainerName.String()), s.WidgetName, fields)
		}
	}
}

func warnSnippetCallAppearance(sc *scriptContext, label, widget string, fields []string) {
	if len(fields) == 0 {
		return
	}
	sort.Strings(fields)
	sc.warnings = append(sc.warnings, fmt.Sprintf("%s: %s [%s] — %s",
		label, snippetCallAppearanceMessage(widget, fields), snippetCallAppearanceRule, snippetCallAppearanceSuggestion))
}

// storedIsSnippetCall reports whether the stored widget is a snippet call.
func storedIsSnippetCall(m backend.PageMutator, widget string) bool {
	typer, ok := m.(widgetStorageTyper)
	if !ok {
		return false
	}
	bsonType, _ := typer.WidgetStorageType(widget)
	return bsonType == "Forms$SnippetCallWidget"
}

// setsSnippetCallAppearance reports whether one SET property writes a
// non-empty appearance setting: Class, Style, DynamicClasses, or a design
// property the theme declares for the stored widget. Clearing one — the fix
// this warning asks for — is not reported.
func setsSnippetCallAppearance(ctx *ExecContext, m backend.PageMutator, widget, name string, value any) bool {
	switch strings.ToLower(name) {
	case "class", "style", "dynamicclasses":
		s, ok := value.(string)
		return !ok || strings.TrimSpace(s) != ""
	}
	return designPropertyForStoredWidget(ctx.GetThemeRegistry(), m, widget, name) != nil
}
