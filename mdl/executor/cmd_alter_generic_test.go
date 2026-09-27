// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The generic ALTER resolves every operation's target through the document
// type's backend.AlterTargetResolver before the operation runs (ako/mxcli#712).

func alterPageCtxWith(t *testing.T, mut *mock.MockPageMutator) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(model.ID) (backend.PageMutator, error) {
			return mut, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func TestAlterPage_ResolvesEveryTargetThroughTheResolver(t *testing.T) {
	var resolved []string
	mut := &mock.MockPageMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			resolved = append(resolved, tg.String())
			return backend.AlterTargetMatch{Kind: "widget", Name: tg.String()}, nil
		},
		DropListViewTemplateFunc: func(string, string) error { return nil },
	}
	ctx := alterPageCtxWith(t, mut)
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{Target: ast.WidgetRef{Widget: "btnSave"}, Properties: map[string]any{"Caption": "Save"}},
			&ast.SetPropertyOp{Properties: map[string]any{"Title": "T"}}, // the document itself: nothing to resolve
			&ast.DropWidgetOp{Targets: []ast.WidgetRef{{Widget: "a"}, {Widget: "dg", Column: "Total"}}},
			&ast.ReplaceWidgetOp{Target: ast.WidgetRef{Widget: "hdr"}},
			&ast.InsertWidgetOp{Position: "INTO", Target: ast.WidgetRef{Widget: "ctn"}},
			&ast.DropListViewTemplateOp{ListView: "lv", Specialization: "M.E"},
		},
	}))
	sort.Strings(resolved)
	want := []string{"a", "btnSave", "ctn", "dg.Total", "hdr", "lv"}
	if strings.Join(resolved, ",") != strings.Join(want, ",") {
		t.Errorf("resolved targets: got %v, want %v", resolved, want)
	}
}

// A refusal from the resolver stops the statement before the operation runs
// and before anything is saved.
func TestAlterPage_ResolverRefusalStopsBeforeTheOperation(t *testing.T) {
	saved, dropped := false, false
	mut := &mock.MockPageMutator{
		ResolveAlterTargetFunc: func(tg backend.AlterTarget) (backend.AlterTargetMatch, error) {
			return backend.AlterTargetMatch{}, &backend.AlterTargetError{Target: tg,
				Matches: []backend.AlterTargetMatch{{Kind: "widget", Name: "x"}, {Kind: "widget", Name: "y"}}}
		},
		DropWidgetFunc: func([]backend.WidgetRef) error { dropped = true; return nil },
		SaveFunc:       func() error { saved = true; return nil },
	}
	ctx := alterPageCtxWith(t, mut)
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName:   ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{&ast.DropWidgetOp{Targets: []ast.WidgetRef{{Caption: "Save"}}}},
	})
	var te *backend.AlterTargetError
	if !errors.As(err, &te) {
		t.Fatalf("want the resolver's AlterTargetError, got %v", err)
	}
	if !strings.Contains(err.Error(), "@2 widget y") {
		t.Errorf("error should list the matches: %v", err)
	}
	if dropped || saved {
		t.Errorf("operation ran (%v) or document saved (%v) after a refused target", dropped, saved)
	}
}
