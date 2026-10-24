// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1560: a flow argument that is a widget's selection is stored as
// Studio Pro stores it on TestApp's WorkflowCommons.UserTask_Assign — a
// Forms$PageVariable naming only the widget, Expression "". Kind "widget" fell
// to the page-parameter slot, which names a page parameter `grid2` the page does
// not have.
func TestClientActionToGen_FlowCallWritesSelectionMapping(t *testing.T) {
	check := func(t *testing.T, variable any, expression string) {
		t.Helper()
		pv, ok := variable.(*genPg.PageVariable)
		if !ok {
			t.Fatalf("Variable = %T, want *PageVariable", variable)
		}
		if pv.WidgetQualifiedName() != "grid2" || pv.PageParameterQualifiedName() != "" ||
			pv.SnippetParameterQualifiedName() != "" || pv.LocalVariableQualifiedName() != "" {
			t.Errorf("PageVariable widget=%q page=%q snippet=%q local=%q, want grid2 and three empty slots",
				pv.WidgetQualifiedName(), pv.PageParameterQualifiedName(),
				pv.SnippetParameterQualifiedName(), pv.LocalVariableQualifiedName())
		}
		if expression != "" {
			t.Errorf("Expression = %q, want \"\" (Studio Pro writes both keys, the unused one empty)", expression)
		}
	}

	t.Run("microflow", func(t *testing.T) {
		el, err := clientActionToGen(&pages.MicroflowClientAction{
			BaseElement:   model.BaseElement{ID: "a"},
			MicroflowName: "WorkflowCommons.ACT_WorkflowUserTask_Assignees_Remove",
			ParameterMappings: []*pages.MicroflowParameterMapping{
				{ParameterName: "UserList", Variable: "$grid2", VariableKind: "widget"},
			},
		})
		if err != nil {
			t.Fatalf("clientActionToGen: %v", err)
		}
		items := el.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings).ParameterMappingsItems()
		if len(items) != 1 {
			t.Fatalf("%d mappings written, want 1", len(items))
		}
		m := items[0].(*genPg.MicroflowParameterMapping)
		check(t, m.Variable(), m.Expression())
	})

	t.Run("nanoflow", func(t *testing.T) {
		el, err := clientActionToGen(&pages.NanoflowClientAction{
			BaseElement:  model.BaseElement{ID: "b"},
			NanoflowName: "M.NF_Remove",
			ParameterMappings: []*pages.NanoflowParameterMapping{
				{ParameterName: "Items", Variable: "$grid2", VariableKind: "widget"},
			},
		})
		if err != nil {
			t.Fatalf("clientActionToGen: %v", err)
		}
		items := el.(*genPg.CallNanoflowClientAction).ParameterMappingsItems()
		if len(items) != 1 {
			t.Fatalf("%d mappings written, want 1", len(items))
		}
		m := items[0].(*genPg.NanoflowParameterMapping)
		check(t, m.Variable(), m.Expression())
	})

	// Control: a page parameter still fills the PageParameter slot alone.
	t.Run("page parameter control", func(t *testing.T) {
		el, err := clientActionToGen(&pages.MicroflowClientAction{
			BaseElement:   model.BaseElement{ID: "c"},
			MicroflowName: "M.ACT",
			ParameterMappings: []*pages.MicroflowParameterMapping{
				{ParameterName: "Order", Variable: "$Order", VariableKind: "parameter"},
			},
		})
		if err != nil {
			t.Fatalf("clientActionToGen: %v", err)
		}
		m := el.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings).
			ParameterMappingsItems()[0].(*genPg.MicroflowParameterMapping)
		pv := m.Variable().(*genPg.PageVariable)
		if pv.PageParameterQualifiedName() != "Order" || pv.WidgetQualifiedName() != "" {
			t.Errorf("page parameter written as page=%q widget=%q, want Order and \"\"",
				pv.PageParameterQualifiedName(), pv.WidgetQualifiedName())
		}
	})
}
