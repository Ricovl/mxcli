// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// mendixlabs/mxcli#1578: the Marketplace Calendar declares
// `<property key="editable" type="expression" defaultValue="true">`, and a page
// built from its generated template failed `mx check` with
// `[CE0642] "Property 'Editable' is required."` — the template wrote the
// expression empty. Studio Pro writes the declared default; so must the
// template, or every widget with a defaulted expression is unbuildable until
// each one is spelled out.
func TestGenerateFromMPK_ExpressionDefault(t *testing.T) {
	ResetPlaceholderCounter()
	def := &mpk.WidgetDefinition{
		ID: "com.example.Cal", Name: "Cal", Version: "1.0.0",
		Properties: []mpk.PropertyDef{
			{Key: "editable", Type: "expression", Caption: "Editable", DefaultValue: "true"},
			{Key: "titleExpression", Type: "expression", Caption: "Title expression"},
		},
	}
	tmpl := GenerateFromMPK(def)

	got := map[string]string{}
	keyByTypeID := map[string]string{}
	objType := tmpl.Type["ObjectType"].(map[string]any)
	for _, pt := range objType["PropertyTypes"].([]any) {
		m, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		keyByTypeID[m["$ID"].(string)] = m["PropertyKey"].(string)
	}
	for _, p := range tmpl.Object["Properties"].([]any) {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		key := keyByTypeID[m["TypePointer"].(string)]
		got[key], _ = m["Value"].(map[string]any)["Expression"].(string)
	}
	if got["editable"] != "true" {
		t.Errorf("editable Expression = %q, want the declared default %q", got["editable"], "true")
	}
	// Control: an expression with no declared default stays empty.
	if got["titleExpression"] != "" {
		t.Errorf("titleExpression Expression = %q, want empty", got["titleExpression"])
	}
}
