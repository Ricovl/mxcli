// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#1486 (mendixlabs/mxcli#1581): `grant access on published rest
// service M.PingApi to M.User` reported "published rest service not found"
// when the service was created with a `folder '…'` clause earlier in the same
// script, and exec rolled the whole script back.
//
// The executor caches the container hierarchy (module ← folder ← folder) once
// per exec and resolves every document's module through it. CREATE PUBLISHED
// REST SERVICE built that cache in its own existence check, then created the
// folder — and nothing told the cache. The grant found the service, walked up
// from its container, hit a folder the cache had never seen, and so compared
// "" against "M". Every other write that changes containers (create module,
// move document, move/drop folder, apply folder) already kept the cache in
// step; creating a folder was the one that did not.

// folderStore is a mock project that behaves like a store: what a handler
// creates, the next statement lists. Its CreateFolder deliberately does NOT
// touch the hierarchy — the fixture in document_placement_test.go registers
// each new folder into the cached hierarchy itself, which is exactly the
// bookkeeping the executor was missing, and it hid this bug from that suite.
type folderStore struct {
	mod      *model.Module
	folders  []*types.FolderInfo
	services []*model.PublishedRestService
}

func newFolderStoreCtx(t *testing.T) (*folderStore, *ExecContext, *strings.Builder) {
	t.Helper()
	s := &folderStore{mod: mkModule("M")}
	ping := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: nextID("mf")},
		ContainerID: s.mod.ID,
		Name:        "PRS_Ping",
		Parameters: []*microflows.MicroflowParameter{{
			Name: "httpRequest",
			Type: &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"},
		}},
		ReturnType: &microflows.StringType{},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{s.mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return s.folders, nil },
		CreateFolderFunc: func(f *model.Folder) error {
			s.folders = append(s.folders, &types.FolderInfo{ID: f.ID, ContainerID: f.ContainerID, Name: f.Name})
			return nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{ping}, nil
		},
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) { return s.services, nil },
		CreatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error {
			if svc.ID == "" {
				svc.ID = nextID("prs")
			}
			s.services = append(s.services, svc)
			return nil
		},
		UpdatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error { return nil },
		MoveDocumentFunc:               func(unitID, containerID model.ID) error { return nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
		UpdatePublishedRestServiceRolesFunc: func(id model.ID, roles []string) error {
			for _, svc := range s.services {
				if svc.ID == id {
					svc.AllowedRoles = roles
				}
			}
			return nil
		},
	}
	// No hierarchy is seeded: the first statement that needs it builds it from
	// the store, as exec does.
	ctx, _ := newMockCtx(t, withBackend(mb))
	out := &strings.Builder{}
	ctx.Output = out
	return s, ctx, out
}

// execFolderScript runs a script statement by statement against one context,
// the way exec does, and stops at the first error.
func execFolderScript(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	for _, stmt := range parseMDL(t, src).Statements {
		var err error
		switch s := stmt.(type) {
		case *ast.CreatePublishedRestServiceStmt:
			err = execCreatePublishedRestService(ctx, s)
		case *ast.GrantPublishedRestServiceAccessStmt:
			err = execGrantPublishedRestServiceAccess(ctx, s)
		case *ast.RevokePublishedRestServiceAccessStmt:
			err = execRevokePublishedRestServiceAccess(ctx, s)
		default:
			t.Fatalf("unexpected statement %T", stmt)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func pingService(folderClause string) string {
	return `create or modify published rest service M.PingApi ` + folderClause + ` (
  Path: 'rest/ping/v1',
  Version: '1.0.0',
  ServiceName: 'PingApi'
)
{
  resource 'ping' {
    get '' microflow M.PRS_Ping;
  }
};
`
}

func TestGrantPublishedRestService_CreatedInFolderSameScript(t *testing.T) {
	cases := map[string]string{
		// The reported case: the create makes the folder.
		"new nested folder": `folder 'Api/Services'`,
		// Control: the module root. Passed before the fix; it proves the
		// failure is the folder, not the grant.
		"module root": ``,
	}
	for name, folder := range cases {
		t.Run(name, func(t *testing.T) {
			s, ctx, out := newFolderStoreCtx(t)
			err := execFolderScript(t, ctx, pingService(folder)+`
grant access on published rest service M.PingApi to M.User;`)
			if err != nil {
				t.Fatalf("grant after create failed: %v\noutput:\n%s", err, out)
			}
			if len(s.services) != 1 {
				t.Fatalf("want 1 service, got %d", len(s.services))
			}
			if got := strings.Join(s.services[0].AllowedRoles, ","); got != "M.User" {
				t.Errorf("stored roles = %q, want M.User", got)
			}

			if err := execFolderScript(t, ctx, `revoke access on published rest service M.PingApi from M.User;`); err != nil {
				t.Fatalf("revoke after create failed: %v", err)
			}
			if got := s.services[0].AllowedRoles; len(got) != 0 {
				t.Errorf("roles after revoke = %v, want none", got)
			}
		})
	}
}

// A re-run of the same CREATE OR MODIFY looks the service up by the same
// route. With a stale cache that lookup missed too, and the statement fell
// through to CREATE: a second service with the same name, silently.
func TestCreateOrModifyPublishedRestService_InNewFolderTwiceIsOneService(t *testing.T) {
	s, ctx, out := newFolderStoreCtx(t)
	if err := execFolderScript(t, ctx, pingService(`folder 'Api/Services'`)+pingService(`folder 'Api/Services'`)); err != nil {
		t.Fatalf("exec: %v\noutput:\n%s", err, out)
	}
	if len(s.services) != 1 {
		t.Errorf("the second statement created a duplicate: %d services named M.PingApi\noutput:\n%s", len(s.services), out)
	}
}

// Control: the not-found refusal itself is intact.
func TestGrantPublishedRestService_MissingServiceStillRefused(t *testing.T) {
	_, ctx, _ := newFolderStoreCtx(t)
	err := execFolderScript(t, ctx, pingService(`folder 'Api/Services'`)+`
grant access on published rest service M.Missing to M.User;`)
	if err == nil || !strings.Contains(err.Error(), "published rest service not found: M.Missing") {
		t.Fatalf("want the not-found refusal for M.Missing, got %v", err)
	}
}

// The mechanism, below every doctype: a folder created while the hierarchy is
// cached must be resolvable through that cache. Every handler with a FOLDER
// clause creates folders through resolveFolder, so this is what the audit's
// other failures (scheduled event, task queue, regular expression, consumed
// REST service) had in common with the reported one.
func TestCreatedFolderIsVisibleThroughTheCachedHierarchy(t *testing.T) {
	_, ctx, _ := newFolderStoreCtx(t)
	mod, err := findModule(ctx, "M")
	assertNoError(t, err)

	_, err = getHierarchy(ctx) // cached before the folder exists
	assertNoError(t, err)

	leaf, err := resolveFolder(ctx, mod.ID, "Api/Services")
	assertNoError(t, err)

	h, err := getHierarchy(ctx)
	assertNoError(t, err)
	if got := h.GetModuleName(h.FindModuleID(leaf)); got != "M" {
		t.Errorf("module of the new folder = %q, want M (the cached hierarchy never learned of the folder)", got)
	}
	if got := h.BuildFolderPath(leaf); got != "Api/Services" {
		t.Errorf("folder path = %q, want Api/Services", got)
	}

	// The page builder resolves FOLDER clauses on pages and snippets with its
	// own walk; it must create through the same helper.
	pb := &pageBuilder{ctx: ctx, backend: ctx.Backend, moduleID: mod.ID, moduleName: "M", execCache: ctx.Cache}
	pageLeaf, err := pb.resolveFolder("Pages/Admin")
	assertNoError(t, err)
	h, _ = getHierarchy(ctx)
	if got := h.BuildFolderPath(pageLeaf); got != "Pages/Admin" {
		t.Errorf("page folder path = %q, want Pages/Admin", got)
	}
}

// Every folder the executor creates goes through createFolder, which keeps the
// cached hierarchy in step. A second call site would be a second chance to
// forget that, which is how this bug happened: the hierarchy's other writers
// all remembered, the folder creator did not.
func TestFolderCreationHasOneCallSite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	assertNoError(t, err)
	call := regexp.MustCompile(`\.CreateFolder\(`)
	var sites []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		assertNoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			if call.MatchString(line) {
				sites = append(sites, f+":"+strconv.Itoa(i+1))
			}
		}
	}
	if len(sites) != 1 || !strings.HasPrefix(sites[0], "helpers.go:") {
		t.Errorf("Backend.CreateFolder is called from %v; route new call sites through createFolder (helpers.go), "+
			"which records the folder in the cached hierarchy (ako/mxcli#1486)", sites)
	}
}
