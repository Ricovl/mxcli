// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1379 — a Data grid 2 column over a junction entity, bound
// two associations away:
//
//	column (Attribute: "MyModule"."AssocA_2"/"MyModule"."AssocB_2"/"NId")
//
// reported "MDL-WIDGET34 … is not an attribute binding MDL can store" plus
// MDL-WIDGET42 "cannot hold the expression". The parse half is the qualified-hop
// grammar fix; this pins the builder half for the reported shape: two hops,
// each qualified, must store the final attribute on the far entity with one
// IndirectEntityRef step per hop — the same as the bare spelling.
func multiHopPB(t *testing.T) *pageBuilder {
	t.Helper()
	const modID = model.ID("mod")
	const junctionID = model.ID("e-junction")
	const aID = model.ID("e-a")
	const bID = model.ID("e-b")
	return &pageBuilder{
		entityContext: "MyModule.ParentChildAssociation",
		widgetScope:   map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "MyModule"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: junctionID}, Name: "ParentChildAssociation"},
					{BaseElement: model.BaseElement{ID: aID}, Name: "AssocA"},
					{BaseElement: model.BaseElement{ID: bID}, Name: "AssocB",
						Attributes: []*domainmodel.Attribute{{Name: "NId"}}},
				},
				Associations: []*domainmodel.Association{
					{Name: "AssocA_2", ParentID: junctionID, ChildID: aID, Type: domainmodel.AssociationTypeReference},
					{Name: "AssocB_2", ParentID: aID, ChildID: bID, Type: domainmodel.AssociationTypeReference},
				},
			}},
		},
	}
}

func TestMultiHopQualifiedColumnBuildsIndirectRef(t *testing.T) {
	build := func(attr string) (string, []pages.AttributeRefStep) {
		spec, err := multiHopPB(t).buildColumnSpecFromAST(&ast.WidgetV3{
			Type:       "column",
			Properties: map[string]any{"Attribute": attr, "Caption": "ID"},
		})
		if err != nil {
			t.Fatalf("build column %q: %v", attr, err)
		}
		return spec.Attribute, spec.AttributeRefSteps
	}
	want := []pages.AttributeRefStep{
		{Association: "MyModule.AssocA_2", DestinationEntity: "MyModule.AssocA"},
		{Association: "MyModule.AssocB_2", DestinationEntity: "MyModule.AssocB"},
	}
	for _, path := range []string{
		"AssocA_2/AssocB_2/NId",                   // bare: control
		"MyModule.AssocA_2/MyModule.AssocB_2/NId", // the issue's spelling, unquoted by the visitor
	} {
		attr, steps := build(path)
		if attr != "MyModule.AssocB.NId" {
			t.Errorf("%s: attribute = %q, want MyModule.AssocB.NId", path, attr)
		}
		if !reflect.DeepEqual(steps, want) {
			t.Errorf("%s: steps = %+v, want %+v", path, steps, want)
		}
	}
}

// The reported script, verbatim, through parse + widget validation: no
// MDL-WIDGET34 / MDL-WIDGET42, and the column's Attribute reaches the checker
// as the path string rather than a WidgetExpressionV3.
func TestIssue1379MultiHopColumnPassesCheck(t *testing.T) {
	const src = `mdl 1;
create or modify snippet "MyModule"."SN_Parent_Children"
folder 'Parent/Details'
( Params: ( $Parent: "MyModule"."Parent" ) )
{
dataview "dvParent" ( DataSource: $Parent ) {
datagrid "dgChildren" (
DataSource: DATABASE "MyModule"."ParentChildAssociation"
WHERE ["MyModule"."Parent_2" = $currentObject],
Selection: Single
) {
column (
Attribute: "MyModule"."AssocA_2"/"MyModule"."AssocB_2"/"NId",
Caption: 'ID'
) {
textfilter textFilter_1
}
}
}
};`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var snip *ast.CreateSnippetStmtV3
	for _, st := range prog.Statements {
		if s, ok := st.(*ast.CreateSnippetStmtV3); ok {
			snip = s
		}
	}
	if snip == nil {
		t.Fatalf("no snippet statement in %#v", prog.Statements)
	}
	col := snip.Widgets[0].Children[0].Children[0]
	if got := col.Properties["Attribute"]; got != "MyModule.AssocA_2/MyModule.AssocB_2/NId" {
		t.Errorf("column Attribute = %#v (%T), want the path string", got, got)
	}
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}
	for _, v := range validateWidgetTree(snip.Widgets, registry, "snippet MyModule.SN_Parent_Children") {
		if v.RuleID == "MDL-WIDGET34" || v.RuleID == "MDL-WIDGET42" {
			t.Errorf("%s: %s", v.RuleID, v.Message)
		}
	}
}
