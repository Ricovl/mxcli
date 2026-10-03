// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A conditionally editable widget stores Editable "Conditional" beside its
// ConditionalEditabilitySettings. describe printed both — `Editable:
// Conditional, Editable: <expr>` — so the key appeared twice and the first one
// was not something an author can write: the expression is what makes it
// conditional. Measured on a text box.
func TestDescribeConditionalEditable_PrintsTheExpressionOnly(t *testing.T) {
	const expr = "$currentObject/Notes != empty"
	props := appendAppearanceProps(nil, nil, rawWidget{Editable: "Conditional", EditableIf: expr})
	var editable []string
	for _, p := range props {
		if strings.HasPrefix(p, "Editable:") {
			editable = append(editable, p)
		}
	}
	if len(editable) != 1 || editable[0] != "Editable: "+expr {
		t.Fatalf("describe printed %v, want exactly [Editable: %s]", editable, expr)
	}

	// The control: a static value with no expression still prints.
	props = appendAppearanceProps(nil, nil, rawWidget{Editable: "Never"})
	if len(props) != 1 || props[0] != "Editable: Never" {
		t.Fatalf("describe printed %v for Editable Never", props)
	}
}
