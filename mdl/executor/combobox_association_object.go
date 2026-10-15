// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A combo box in association mode edits a reference OF AN OBJECT, and the object
// has to come from somewhere (mendixlabs/mxcli#1420):
//
//	-- in a snippet with parameter $Asset, outside every data container:
//	combobox x (Association: M.Asset_Customer, DataSource: database M.Customer,
//	            CaptionAttribute: CustomerName)
//
// passed check and exec, and mx check 11.6.6 then reported
//
//	[CE7007] "Selected value is not valid for entity 'Asset_Customer/Customer'.
//	Please select the value again." at Combo box 'x'
//
// because the writer stores the association with no SourceVariable — "the
// enclosing object" — and there is none. Studio Pro stores a combo box over a
// parameter's reference with SourceVariable { SnippetParameter / PageParameter }
// (measured on TestApp's WorkflowCommons snippets, cmd_pages_parameter_binding.go),
// which MDL already writes from `Attribute: $Asset.M.Asset_Customer` — 0 errors
// on 11.6.6, and what describe prints for it. So the construct is expressible;
// the bare spelling just names no object. Which parameter the author meant is
// not mxcli's to guess (a snippet may have several of the same entity), so the
// bare form is refused, naming the spelling that builds.
//
// One decision, three callers, so check and exec cannot disagree about it:
// the widget engine at build time (no enclosing entity), the widget-tree walk at
// check time with no project (MDL-WIDGET46, document root known empty), and the
// ALTER insertion-point pass with a project (validate_alter_unscoped.go).

// comboBoxAssociationNeedingObject returns the association binding of a combo
// box in association mode that names no object of its own to start from — so it
// starts at the enclosing data container's object, and fails CE7007 without
// one. ok is false for anything else: another widget, enumeration mode, no
// binding, or a binding that names its object (`$Param.Module.Assoc`).
func comboBoxAssociationNeedingObject(w *ast.WidgetV3) (key, binding string, ok bool) {
	if w == nil || !strings.EqualFold(w.Type, "combobox") {
		return "", "", false
	}
	// Association mode is selected by an option list, given generically or
	// under the schema key (the def's hasDataSource condition).
	if w.GetDataSource() == nil {
		if _, named := lookupProperty(w.Properties, comboBoxOptionsDataSourceKey); !named {
			return "", "", false
		}
	}
	key, binding = "Association", w.GetStringProp("Association")
	if binding == "" {
		key, binding = "Attribute", w.GetAttribute()
	}
	if binding == "" || strings.HasPrefix(binding, "$") {
		// `$Param.Module.Assoc` names its object; any other `$…` spelling is
		// another rule's to judge.
		return "", "", false
	}
	return key, binding, true
}

// comboBoxAssociationObjectProblem is the refusal for one such combo box.
func comboBoxAssociationObjectProblem(w *ast.WidgetV3, key, binding string) string {
	return fmt.Sprintf("combobox `%s`: `%s: %s` names no object for the association to start from — no data "+
		"view, list view, gallery or data grid with a known entity encloses the combo box, so it would be written "+
		"with no source object and mxbuild reports CE7007 \"Selected value is not valid for entity '…'\". Start it at the page "+
		"or snippet parameter that holds the object, as Studio Pro stores it — `Attribute: $Param.%s` — or place "+
		"the combo box inside `dataview dv (DataSource: $Param) { … }`",
		w.Name, key, binding, binding)
}

// validateComboBoxAssociationObject is the check-time rule with no project
// (MDL-WIDGET46): a combo box at a document root the walk knows to be empty.
// c is the context the widget SITS in — not the one its own option list
// creates, which says nothing about where the association starts.
func validateComboBoxAssociationObject(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
	if !c.known || c.present {
		return nil
	}
	key, binding, ok := comboBoxAssociationNeedingObject(w)
	if !ok {
		return nil
	}
	return []linter.Violation{{
		RuleID:     "MDL-WIDGET46",
		Severity:   linter.SeverityError,
		Message:    locationPrefix + ": " + comboBoxAssociationObjectProblem(w, key, binding),
		Suggestion: "A combo box in association mode sets a reference of an object: bind it through the parameter (`Attribute: $Param.Module.Assoc`) or put it inside a data container over that object.",
	}}
}

// unanchoredComboBoxes lists the problems of the association-mode combo boxes
// among ws that would start at the insertion point's object, for an insertion
// point with none. A widget with a data source of its own supplies an object to
// its children, so they are not descended into.
func unanchoredComboBoxes(ws []*ast.WidgetV3) []string {
	var out []string
	for _, w := range ws {
		if w == nil {
			continue
		}
		if key, binding, ok := comboBoxAssociationNeedingObject(w); ok {
			out = append(out, comboBoxAssociationObjectProblem(w, key, binding))
			continue
		}
		if w.GetDataSource() != nil {
			continue
		}
		out = append(out, unanchoredComboBoxes(w.Children)...)
	}
	return out
}
