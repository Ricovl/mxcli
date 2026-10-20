// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A page's ContainerID is its immediate parent, which for most Studio Pro pages is
// a folder. resolvePageUnit handed that ID straight to GetModule, so ALTER PAGE over
// MCP failed for every foldered page with "module not found: <folder id>" before any
// pg_* call (mendixlabs/mxcli#1593). Root pages, whose parent IS the module, worked.
func TestResolvePageUnit_FolderedPages(t *testing.T) {
	mod := &model.Module{Name: "MyFirstModule"}
	mod.ID = "mod1"
	folders := []*types.FolderInfo{
		{ID: "fA", ContainerID: "mod1", Name: "A"},
		{ID: "fB", ContainerID: "fA", Name: "B"},
		{ID: "fPages", ContainerID: "fB", Name: "Pages"},
	}
	saved := map[model.ID]*pages.Page{}
	for _, p := range []struct{ id, container, name string }{
		{"pRoot", "mod1", "RootPage"},
		{"pFolder", "fA", "FolderPage"},
		{"pNested", "fPages", "NestedPage"},
	} {
		pg := &pages.Page{}
		pg.ID = model.ID(p.id)
		pg.ContainerID = model.ID(p.container)
		pg.Name = p.name
		saved[pg.ID] = pg
	}
	reader := &mock.MockBackend{
		// The local reader resolves only true module IDs, exactly as modelsdk does.
		GetModuleFunc: func(id model.ID) (*model.Module, error) {
			if id == mod.ID {
				return mod, nil
			}
			return nil, fmt.Errorf("module not found: %s", id)
		},
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return folders, nil },
		GetPageFunc: func(id model.ID) (*pages.Page, error) {
			if p, ok := saved[id]; ok {
				return p, nil
			}
			return nil, fmt.Errorf("page not found: %s", id)
		},
	}
	session := &pages.Page{}
	session.ID = "pSession"
	session.ContainerID = "fA"
	session.Name = "SessionPage"
	b := &Backend{reader: reader, sessionPages: []*pages.Page{session}}

	cases := []struct {
		id       model.ID
		wantPage string
	}{
		{"pRoot", "RootPage"},       // control: module-root page always resolved
		{"pFolder", "FolderPage"},   // one folder deep (the reported case)
		{"pNested", "NestedPage"},   // A/B/Pages
		{"pSession", "SessionPage"}, // created over MCP this session, in a folder
	}
	for _, c := range cases {
		mn, pn, err := b.resolvePageUnit(c.id)
		if err != nil {
			t.Errorf("resolvePageUnit(%s): %v", c.id, err)
			continue
		}
		if mn != "MyFirstModule" || pn != c.wantPage {
			t.Errorf("resolvePageUnit(%s) = (%q, %q), want (%q, %q)", c.id, mn, pn, "MyFirstModule", c.wantPage)
		}
	}

	// A page whose container chain is genuinely broken still errors.
	saved["pOrphan"] = &pages.Page{}
	saved["pOrphan"].ID = "pOrphan"
	saved["pOrphan"].ContainerID = "nowhere"
	if _, _, err := b.resolvePageUnit("pOrphan"); err == nil {
		t.Error("orphaned page should fail to resolve")
	}
}

// End to end over a fake Studio Pro: the reporter's trace showed no PED call before
// the error. With the module resolved, the read goes out addressed by module name.
func TestOpenPageForMutation_FolderedPageReachesPgReadPage(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "pg_read_page" {
			return `{"layout":"Atlas_Core.Atlas_Default","widgets":[]}`, false
		}
		return "{}", false
	})
	mod := &model.Module{Name: "MyFirstModule"}
	mod.ID = "mod1"
	pg := &pages.Page{}
	pg.ID = "p1"
	pg.ContainerID = "fPages"
	pg.Name = "Customer_Overview"
	b := &Backend{
		client: f.connectClient(t),
		reader: &mock.MockBackend{GetModuleFunc: func(id model.ID) (*model.Module, error) {
			return nil, fmt.Errorf("module not found: %s", id)
		}},
		sessionModules: []*model.Module{mod},
		sessionFolders: []*types.FolderInfo{{ID: "fPages", ContainerID: "mod1", Name: "Pages"}},
		sessionPages:   []*pages.Page{pg},
	}
	if _, err := b.OpenPageForMutation("p1"); err != nil {
		t.Fatalf("OpenPageForMutation: %v", err)
	}
	call, ok := f.callByName("pg_read_page")
	if !ok {
		t.Fatal("pg_read_page was never called")
	}
	if call.Args["moduleName"] != "MyFirstModule" || call.Args["pageName"] != "Customer_Overview" {
		t.Errorf("pg_read_page args = %v, want MyFirstModule.Customer_Overview", call.Args)
	}
}

// The same immediate-container lookup sat in the constant and workflow update paths
// (CREATE OR MODIFY on a foldered document) and in the session-registry cleanup.
func folderedBackend(t *testing.T, f *fakePED) *Backend {
	t.Helper()
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{
		sessionModules: []*model.Module{mod},
		sessionFolders: []*types.FolderInfo{
			{ID: "f1", ContainerID: "mod1", Name: "Config"},
			{ID: "f2", ContainerID: "f1", Name: "Nested"},
		},
	}
	if f != nil {
		b.client = f.connectClient(t)
	}
	return b
}

func documentNamesSent(f *fakePED) []string {
	var out []string
	for _, c := range f.calls {
		if n, _ := c.Args["documentName"].(string); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func TestUpdateConstant_FolderedConstant(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := folderedBackend(t, f)
	c := &model.Constant{Name: "ApiUrl", Type: model.ConstantDataType{Kind: "String"}, DefaultValue: "x"}
	c.ID = "c1"
	c.ContainerID = "f2"
	if err := b.UpdateConstant(c); err != nil {
		t.Fatalf("UpdateConstant: %v", err)
	}
	names := documentNamesSent(f)
	if len(names) == 0 || names[0] != "M.ApiUrl" {
		t.Errorf("PED calls addressed %v, want M.ApiUrl", names)
	}
}

func TestUpdateWorkflow_FolderedWorkflow(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := folderedBackend(t, f)
	wf := &workflows.Workflow{
		Name: "WF",
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
		}},
	}
	wf.ID = "wf1"
	wf.ContainerID = "f1"
	err := b.UpdateWorkflow(wf)
	if err != nil && strings.Contains(err.Error(), "resolve module") {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	names := documentNamesSent(f)
	if len(names) == 0 || names[0] != "M.WF" {
		t.Errorf("PED calls addressed %v, want M.WF", names)
	}
}

func TestForgetSessionDocuments_Foldered(t *testing.T) {
	b := folderedBackend(t, nil)
	c := &model.Constant{Name: "ApiUrl"}
	c.ContainerID = "f2"
	e := &model.Enumeration{Name: "Status"}
	e.ContainerID = "f1"
	b.sessionConstants = []*model.Constant{c}
	b.sessionEnums = []*model.Enumeration{e}
	b.forgetSessionConstant("M", "ApiUrl")
	b.forgetSessionEnum("M", "Status")
	if len(b.sessionConstants) != 0 || len(b.sessionEnums) != 0 {
		t.Errorf("dropped foldered documents still registered: %d constant(s), %d enumeration(s)",
			len(b.sessionConstants), len(b.sessionEnums))
	}
}
