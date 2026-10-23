// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateWidgetSelectionValues is MDL-WIDGET50, the check-time half of
// selectionMappingValue: a selection property given a value that is not a
// selection, or given two different values under its two spellings (the
// `Selection:` keyword and the property's own key). exec refuses both; before
// mendixlabs/mxcli#1618 the key's value was stored as a PrimitiveValue and
// `itemSelection: Bogus` passed check, exec and mx check alike.
//
// def is w's definition; schema stands in for it when there is none (a Data
// grid 2 without a project), its Selection-typed keys being the properties.
func validateWidgetSelectionValues(w *ast.WidgetV3, def *WidgetDefinition, schema *embeddedWidgetSchema, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	groups := selectionSpellings(def, schema)
	if len(groups) == 0 {
		return nil
	}
	var out []linter.Violation
	report := func(msg string) {
		out = append(out, linter.Violation{
			RuleID:     "MDL-WIDGET50",
			Severity:   linter.SeverityError,
			Message:    locationPrefix + ": " + msg,
			Suggestion: "Write one of `Selection: None`, `Selection: Single` or `Selection: Multi`.",
		})
	}
	checked := map[string]bool{}
	for _, names := range groups {
		var firstKey, firstVal string
		for _, name := range names {
			raw, ok := lookupProperty(w.Properties, name)
			if !ok {
				continue
			}
			key := writtenPropertyName(w, name)
			v := stringifyPropValue(raw)
			if !isSelectionValue(v) {
				if !checked[strings.ToLower(key)] {
					report(selectionValueError(w.Name, key, v))
				}
				checked[strings.ToLower(key)] = true
				continue
			}
			checked[strings.ToLower(key)] = true
			switch {
			case firstKey == "":
				firstKey, firstVal = key, v
			case canonicalSelection(v) != canonicalSelection(firstVal):
				report(selectionDisagreementError(w.Name, firstKey, firstVal, key, v))
			}
		}
	}
	return out
}

// selectionSpellings lists, per selection property, the names it can be
// written under: the mapping's keyword, its key and its aliases. Without a
// definition the schema's Selection-typed keys are the properties, and the
// keyword names one only when there is exactly one.
func selectionSpellings(def *WidgetDefinition, schema *embeddedWidgetSchema) [][]string {
	var groups [][]string
	if def != nil {
		mappings := append([]PropertyMapping(nil), def.PropertyMappings...)
		for _, m := range def.Modes {
			mappings = append(mappings, m.PropertyMappings...)
		}
		seen := map[string]bool{}
		for _, m := range mappings {
			if m.Operation != "selection" || seen[strings.ToLower(m.PropertyKey)] {
				continue
			}
			seen[strings.ToLower(m.PropertyKey)] = true
			var names []string
			if m.Source != "" {
				names = append(names, m.Source)
			}
			groups = append(groups, append(append(names, m.PropertyKey), m.MdlAliases...))
		}
		return groups
	}
	if schema == nil {
		return nil
	}
	var keys []string
	for k, t := range schema.types {
		if t == "Selection" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		names := []string{k}
		if len(keys) == 1 {
			names = append([]string{"Selection"}, k)
		}
		groups = append(groups, names)
	}
	return groups
}

// writtenPropertyName is the spelling w used for the property name matches
// case-insensitively, for a message that quotes the script.
func writtenPropertyName(w *ast.WidgetV3, name string) string {
	if _, ok := w.Properties[name]; ok {
		return name
	}
	for k := range w.Properties {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return name
}
