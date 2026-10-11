// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `Attribute: Module.Assoc/Attr` and `Attribute: Assoc/Attr` name the same
// binding: the qualified hop is how XPath and data view sources spell the
// association, so a page that names it that way must store exactly the
// AttributeRef the bare spelling does. Before the grammar accepted the
// qualified form it never reached the builder (it parsed as a division and
// check refused it, MDL-WIDGET34 + MDL-WIDGET42); this pins what the builder
// does with it now that it does.
func TestQualifiedAssociationHopBuildsSameAttributeRef(t *testing.T) {
	t.Run("datagrid column", func(t *testing.T) {
		build := func(attr string) (string, any) {
			spec, err := ruleActionPB(t).buildColumnSpecFromAST(&ast.WidgetV3{
				Type:       "column",
				Properties: map[string]any{"Attribute": attr, "Caption": "Rule"},
			})
			if err != nil {
				t.Fatalf("build column %q: %v", attr, err)
			}
			return spec.Attribute, spec.AttributeRefSteps
		}
		bareAttr, bareSteps := build("RuleAction_BusinessRule/Name")
		qualAttr, qualSteps := build("Rules.RuleAction_BusinessRule/Name")
		if bareAttr != "Rules.BusinessRule.Name" {
			t.Fatalf("bare column attribute = %q, want Rules.BusinessRule.Name (control)", bareAttr)
		}
		if qualAttr != bareAttr || !reflect.DeepEqual(qualSteps, bareSteps) {
			t.Errorf("qualified column = (%q, %+v), bare = (%q, %+v); want identical",
				qualAttr, qualSteps, bareAttr, bareSteps)
		}
	})

	t.Run("textbox", func(t *testing.T) {
		build := func(attr string) (string, any) {
			built, err := ruleActionPB(t).buildWidgetV3(&ast.WidgetV3{
				Type: "textbox", Name: "tb", Properties: map[string]any{"Attribute": attr},
			})
			if err != nil {
				t.Fatalf("build textbox %q: %v", attr, err)
			}
			path, steps := inputAttributeBinding(t, built)
			return path, steps
		}
		bareAttr, bareSteps := build("RuleAction_BusinessRule/Name")
		qualAttr, qualSteps := build("Rules.RuleAction_BusinessRule/Name")
		if qualAttr != bareAttr || !reflect.DeepEqual(qualSteps, bareSteps) {
			t.Errorf("qualified textbox = (%q, %+v), bare = (%q, %+v); want identical",
				qualAttr, qualSteps, bareAttr, bareSteps)
		}
	})
}
