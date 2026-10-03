// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `replace comboBox1 with { combobox comboBox1 (…) }` keeps what the statement
// does not state (#1247) by comparing the replacement's build with the stored
// widget's build as describe prints it. Once describe printed a pluggable
// widget's stored `Editable: Never`, a replacement that does not mention
// Editable differed from that baseline on it, read as stating `Always`, and
// overwrote the stored value. A visibility or editability property the
// statement leaves out is not in the baseline either, so both builds agree on
// it and the stored value stays.
func TestReplaceBaselineDropsUnstatedSystemProps(t *testing.T) {
	stored := &ast.WidgetV3{Type: "combobox", Name: "comboBox1", Properties: map[string]any{
		"Attribute": "Rules.BusinessRule_RuleCategory",
		"Editable":  "Never",
		"Visible":   "false",
	}}
	t.Run("unstated", func(t *testing.T) {
		stmt := &ast.WidgetV3{Type: "combobox", Name: "comboBox1", Properties: map[string]any{
			"Attribute": "Rules.BusinessRule_RuleCategory",
		}}
		got := replaceBaseline(stored, stmt)
		for _, k := range []string{"Editable", "Visible"} {
			if _, has := got.Properties[k]; has {
				t.Errorf("baseline keeps %s the statement does not state", k)
			}
		}
		if got.Properties["Attribute"] == nil {
			t.Error("baseline lost a property the statement states")
		}
		if stored.Properties["Editable"] != "Never" {
			t.Error("the stored description was modified")
		}
	})
	t.Run("stated (control)", func(t *testing.T) {
		stmt := &ast.WidgetV3{Type: "combobox", Name: "comboBox1", Properties: map[string]any{
			"Editable": "Always",
		}}
		if got := replaceBaseline(stored, stmt); got.Properties["Editable"] != "Never" {
			t.Errorf("a stated Editable must be compared against the stored one, baseline has %v", got.Properties["Editable"])
		}
	})
}
