// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1403: a show page that passes a page or snippet parameter by
// name is stored the way Studio Pro stores it — an explicit
// Forms$PageParameterMapping with Argument "" and Variable a Forms$PageVariable
// naming the parameter (ako/TestApp WorkflowCommons
// TaskAssignmentHelper_UserTask_Reassign / Snip_UserTask_Detail). An inferred
// argument (no VariableKind) keeps the empty array #296 needs.
func TestClientActionToGen_ShowPageWritesNamedParameterMapping(t *testing.T) {
	mappings := func(a *pages.PageClientAction) []*genPg.PageParameterMapping {
		t.Helper()
		el, err := clientActionToGen(a)
		if err != nil {
			t.Fatalf("clientActionToGen: %v", err)
		}
		ps := el.(*genPg.PageClientAction).PageSettings().(*genPg.PageSettings)
		var out []*genPg.PageParameterMapping
		for _, m := range ps.ParameterMappingsItems() {
			out = append(out, m.(*genPg.PageParameterMapping))
		}
		return out
	}

	for _, tc := range []struct{ kind, wantPage, wantSnippet string }{
		{"parameter", "Task", ""},
		{"snippet", "", "Task"},
	} {
		got := mappings(&pages.PageClientAction{
			BaseElement: model.BaseElement{ID: "a"},
			PageName:    "M.Detail",
			ParameterMappings: []*pages.PageClientParameterMapping{
				{ParameterName: "Item", Variable: "$Task", VariableKind: tc.kind},
			},
		})
		if len(got) != 1 {
			t.Fatalf("%s: %d mappings written, want 1", tc.kind, len(got))
		}
		m := got[0]
		// gen's constructor stamps Forms$FormCallArgument (LayoutCallArgument),
		// which mxbuild 11.6.6 refuses to load under a PageSettings.
		if m.TypeName() != "Forms$PageParameterMapping" {
			t.Errorf("%s: mapping $Type = %q, want Forms$PageParameterMapping", tc.kind, m.TypeName())
		}
		if m.ParameterQualifiedName() != "M.Detail.Item" || m.Argument() != "" {
			t.Errorf("%s: mapping Parameter=%q Argument=%q, want M.Detail.Item and \"\"", tc.kind, m.ParameterQualifiedName(), m.Argument())
		}
		pv, ok := m.Variable().(*genPg.PageVariable)
		if !ok {
			t.Fatalf("%s: Variable = %T, want *PageVariable", tc.kind, m.Variable())
		}
		if pv.PageParameterQualifiedName() != tc.wantPage || pv.SnippetParameterQualifiedName() != tc.wantSnippet {
			t.Errorf("%s: PageVariable page=%q snippet=%q, want %q / %q", tc.kind,
				pv.PageParameterQualifiedName(), pv.SnippetParameterQualifiedName(), tc.wantPage, tc.wantSnippet)
		}
	}

	// Control: the inferred context object stays an empty array.
	if got := mappings(&pages.PageClientAction{
		BaseElement:       model.BaseElement{ID: "b"},
		PageName:          "M.Detail",
		ParameterMappings: []*pages.PageClientParameterMapping{{ParameterName: "Item", Variable: "$currentObject"}},
	}); len(got) != 0 {
		t.Errorf("inferred argument written as %d explicit mappings, want 0 (#296: CE0115)", len(got))
	}
}
