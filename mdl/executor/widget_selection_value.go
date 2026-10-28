// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// A selection property — Data grid 2 and Gallery `itemSelection` — is written
// by the `Selection:` keyword or by its own key. The mapping pass read only the
// keyword, and the key fell through to the explicit-key pass, which stored it as
// the property's PrimitiveValue with its Selection left at the default: no
// selection on a grid, and no way to turn a gallery's default one off
// (mendixlabs/mxcli#1618). Both spellings are one property, read here.

// selectionValues are the values Studio Pro stores in a selection property's
// Selection, as canonicalSelection writes them. mxbuild 11.12.5 accepts any
// other string at 0 errors, so nothing after mxcli would catch one.
var selectionValues = []string{"None", "Single", "Multi"}

// isSelectionValue reports whether v names a selection, in any case ("Multiple"
// is the keyword grammar's spelling of Multi).
func isSelectionValue(v string) bool {
	switch strings.ToLower(v) {
	case "none", "single", "multi", "multiple":
		return true
	}
	return false
}

// selectionValueError is the refusal for a value that is not a selection.
func selectionValueError(widgetName, key, value string) string {
	return fmt.Sprintf("widget `%s` property `%s` takes None, Single or Multi, not `%s`", widgetName, key, value)
}

// checkedSelectionValue is value canonicalised, or an error naming key when it
// is not a selection.
func checkedSelectionValue(w *ast.WidgetV3, key, value string) (string, error) {
	if !isSelectionValue(value) {
		return "", mdlerrors.NewValidation(selectionValueError(w.Name, key, value))
	}
	return canonicalSelection(value), nil
}

// selectionMappingValue is the value w gives a selection mapping, as written:
// by the mapping's keyword (`Selection:`), its property key or an alias (the
// builder's SetSelection canonicalises it). Two different values for the one
// property are refused — keeping either would drop the other with nothing
// said. "" means unset.
func selectionMappingValue(mapping PropertyMapping, w *ast.WidgetV3) (string, error) {
	type written struct{ key, value string }
	var got []written
	if mapping.Source != "" {
		if v, ok := lookupProperty(w.Properties, mapping.Source); ok {
			got = append(got, written{mapping.Source, stringifyAny(v)})
		}
	}
	if v, key, ok := namedPropRaw(mapping, w); ok {
		got = append(got, written{key, stringifyAny(v)})
	}
	for _, g := range got {
		if _, err := checkedSelectionValue(w, g.key, g.value); err != nil {
			return "", err
		}
		if first := got[0]; canonicalSelection(g.value) != canonicalSelection(first.value) {
			return "", mdlerrors.NewValidation(selectionDisagreementError(w.Name, first.key, first.value, g.key, g.value))
		}
	}
	if len(got) == 0 {
		return "", nil
	}
	return got[0].value, nil
}

// selectionDisagreementError is the refusal for two different values given to
// one selection property under its two spellings.
func selectionDisagreementError(widgetName, keyA, valueA, keyB, valueB string) string {
	return fmt.Sprintf("widget `%s` gives its selection two values, `%s: %s` and `%s: %s` — both name the "+
		"one property; write one of them", widgetName, keyA, valueA, keyB, valueB)
}
