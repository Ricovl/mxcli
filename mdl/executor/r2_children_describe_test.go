// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// R2 (ako/mxcli#754): describe prints the canonical brackets — properties in
// ( ), children in { } — so its output re-parses without recording any
// deprecated spelling (MDL-DEPR070..073).

func assertCanonicalDescribe(t *testing.T, out string, wantInOutput ...string) {
	t.Helper()
	prog := reparse(t, out)
	if len(prog.Deprecations) != 0 {
		t.Errorf("describe output uses a deprecated spelling: %+v\n--- output ---\n%s", prog.Deprecations, out)
	}
	for _, w := range wantInOutput {
		if !strings.Contains(out, w) {
			t.Errorf("describe output lacks %q:\n%s", w, out)
		}
	}
}

func TestDescribeRestOperation_PropertiesInParens(t *testing.T) {
	svc := &model.ConsumedRestService{
		Name:    "Api",
		BaseUrl: "https://api.example.com",
		Operations: []*model.RestClientOperation{
			{Name: "GetUser", HttpMethod: "GET", Path: "/users/{id}", ResponseType: "NONE",
				Parameters: []*model.RestClientParameter{{Name: "id", DataType: "Integer"}}},
			{Name: "Ping", HttpMethod: "GET", Path: "/ping", ResponseType: "NONE"},
		},
	}
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputConsumedRestServiceMDL(ctx, svc, "M"))
	assertCanonicalDescribe(t, buf.String(), "operation GetUser (", "operation Ping (")
}

func TestDescribeAgentAttachments_PropertiesInParens(t *testing.T) {
	mod := mkModule("M")
	a := &agenteditor.Agent{
		BaseElement: model.BaseElement{ID: nextID("aea")},
		ContainerID: mod.ID,
		Name:        "Helper",
		UsageType:   "Task",
		Model:       &agenteditor.DocRef{QualifiedName: "M.GPT4"},
		Tools: []agenteditor.AgentTool{
			{Name: "Weather", ToolType: "mcp", Enabled: true, Document: &agenteditor.DocRef{QualifiedName: "M.WeatherMcp"}},
			{Name: "Lookup", ToolType: "Microflow", Enabled: true, Description: "Find",
				Document: &agenteditor.DocRef{QualifiedName: "M.Lookup"}},
		},
		KBTools: []agenteditor.AgentKBTool{
			{Name: "Docs", Enabled: true, Document: &agenteditor.DocRef{QualifiedName: "M.KB"},
				CollectionIdentifier: "docs", MaxResults: 3},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, a.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeAgentEditorAgent(ctx, ast.QualifiedName{Module: "M", Name: "Helper"}))
	assertCanonicalDescribe(t, buf.String(), "mcp service M.WeatherMcp (", "tool Lookup (", "knowledge base Docs (")
}

func TestDescribeImageCollection_ImagesInBraces(t *testing.T) {
	mod := mkModule("Icons")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "AppIcons",
		ExportLevel: "Public",
		Images:      []types.Image{{Name: "Logo", Format: "Png"}, {Name: "Home", Format: "Svg"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeImageCollection(ctx, ast.QualifiedName{Module: "Icons", Name: "AppIcons"}))
	out := buf.String()
	assertCanonicalDescribe(t, out, "export level 'Public' {", "image Logo ( Data: ", "image \"Home\" ( Data: ")
	s := findStmt[*ast.CreateImageCollectionStmt](t, reparse(t, out), out)
	if len(s.Images) != 2 || s.Images[0].Name != "Logo" || s.Images[1].Name != "Home" {
		t.Errorf("images = %+v", s.Images)
	}
}

func TestDescribeMessageDefinitionCollection_TreesInBraces(t *testing.T) {
	mod := mkModule("Sales")
	c := &model.MessageDefinitionCollection{
		BaseElement: model.BaseElement{ID: nextID("mdc")},
		ContainerID: mod.ID,
		Name:        "MD_Order",
		Definitions: []*model.MessageDefinition{
			{Name: "Order", Root: &model.MessageDefinitionElement{Kind: "Entity", Entity: "Sales.Order", ExposedName: "Order",
				Children: []*model.MessageDefinitionElement{
					{Kind: "Attribute", Attribute: "OrderId", OriginalName: "OrderId", ExposedName: "OrderId"},
					{Kind: "Entity", Association: "Sales.Order_Line", Entity: "Sales.Line", ExposedName: "Lines",
						Children: []*model.MessageDefinitionElement{
							{Kind: "Attribute", Attribute: "Sku", OriginalName: "Sku", ExposedName: "Sku"},
						}},
					{Kind: "Entity", Association: "Sales.Order_Customer", Entity: "Sales.Customer", ExposedName: "Customer"},
				}}},
			{Name: "Customer", Root: &model.MessageDefinitionElement{Kind: "Entity", Entity: "Sales.Customer", ExposedName: "Customer",
				Children: []*model.MessageDefinitionElement{
					{Kind: "Attribute", Attribute: "Name", OriginalName: "Name", ExposedName: "Name"},
				}}},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, c.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListMessageDefinitionCollectionsFunc: func() ([]*model.MessageDefinitionCollection, error) {
			return []*model.MessageDefinitionCollection{c}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDescribeMessageDefinitionCollection(ctx, ast.QualifiedName{Module: "Sales", Name: "MD_Order"}))
	out := buf.String()
	assertCanonicalDescribe(t, out, "definition Order for Sales.Order {", "Sales.Order_Line/Sales.Line as 'Lines' {",
		"Sales.Order_Customer/Sales.Customer { }")
	s := findStmt[*ast.CreateMessageDefinitionCollectionStmt](t, reparse(t, out), out)
	if len(s.Definitions) != 2 || len(s.Definitions[0].Members) != 3 || len(s.Definitions[0].Members[1].Members) != 1 {
		t.Errorf("definitions = %+v", s.Definitions)
	}
}
