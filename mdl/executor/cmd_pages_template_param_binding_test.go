// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#721 L3. A dynamic-text parameter `{1} = $Task.Name` inside a
// snippet is stored by Studio Pro as an AttributeRef plus a Forms$PageVariable
// whose SnippetParameter slot names the variable; describe prints it back as
// `$Task.Name`. Executing that describe output wrote the variable into the
// PageParameter slot instead — a page parameter named Task that a snippet does
// not have — measured on TestApp's WorkflowCommons.Snip_UserTask_Header:
//
//	Content/Parameters[1]/SourceVariable/SnippetParameter: WorkflowUserTask ->
//	Content/Parameters[1]/SourceVariable/PageParameter:  -> WorkflowUserTask
//
// And a non-String attribute read from a parameter (`{1} = $Task.State`, an
// enumeration) was rewritten as `toString($Task/State)`: an Expression that
// bypasses the parameter's FormattingInfo and the enumeration caption, and that
// describe prints back as the expression, not the binding. Studio Pro binds
// it exactly like a String attribute (WorkflowCommons.Snip_UserTask_State).
func templateBindingPB(isSnippet bool) *pageBuilder {
	const modID = model.ID("mod-m")
	pb := visibleWhenPB("")
	pb.isSnippet = isSnippet
	pb.paramScope = map[string]model.ID{"Task": "e-base"}
	pb.paramEntityNames["Task"] = "M.Job"
	pb.localVariables = map[string]bool{}
	return pb
}

func TestTemplateParameterBinding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isSnippet bool
		value     string
		wantKind  string
		wantAttr  string
	}{
		{"page parameter, String attribute", false, "$Task.Title", "", "M.Job.Title"},
		{"page parameter, enumeration attribute", false, "$Task.Status", "", "M.Job.Status"},
		{"snippet parameter, String attribute", true, "$Task.Title", "snippet", "M.Job.Title"},
		{"snippet parameter, enumeration attribute", true, "$Task.Status", "snippet", "M.Job.Status"},
		{"snippet parameter, Boolean attribute", true, "$Task.IsLocal", "snippet", "M.Job.IsLocal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pages.ClientTemplateParameter{}
			templateBindingPB(tc.isSnippet).resolveTemplateAttributePathFull(tc.value, p)
			if p.Expression != "" {
				t.Errorf("%s bound as Expression %q, want an AttributeRef on the variable", tc.value, p.Expression)
			}
			if p.SourceVariable != "Task" || p.SourceVariableKind != tc.wantKind {
				t.Errorf("%s: SourceVariable = (%q, kind %q), want (Task, kind %q)",
					tc.value, p.SourceVariable, p.SourceVariableKind, tc.wantKind)
			}
			if p.AttributeRef != tc.wantAttr {
				t.Errorf("%s: AttributeRef = %q, want %q", tc.value, p.AttributeRef, tc.wantAttr)
			}
		})
	}
}

// The association data source is the other place a snippet's `$Param` was
// written as a page parameter (Snip_UserTask_TaskTimeline:
// `dataview (DataSource: $WorkflowUserTask/System.WorkflowUserTask_Workflow)`).
func TestAssociationDataSource_SnippetParameterSlot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isSnippet bool
		ctxVar    string
		want      bool
	}{
		{"snippet parameter", true, "Customer", true},
		{"page parameter", false, "Customer", false},
		{"current object in a snippet", true, "currentObject", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb := assocPageBuilder("Bench.Customer")
			pb.isSnippet = tc.isSnippet
			pb.paramScope = map[string]model.ID{"Customer": "e-cust"}
			ds, _, err := pb.buildDataSourceV3(&ast.DataSourceV3{
				Type: "association", Reference: "Bench.Order_Customer", ContextVariable: tc.ctxVar,
			})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			as, ok := ds.(*pages.AssociationSource)
			if !ok {
				t.Fatalf("got %T, want *pages.AssociationSource", ds)
			}
			if as.IsSnippetParameter != tc.want {
				t.Errorf("IsSnippetParameter = %v, want %v", as.IsSnippetParameter, tc.want)
			}
		})
	}
}
