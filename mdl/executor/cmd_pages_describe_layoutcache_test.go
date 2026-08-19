// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestResolveLayoutName_ListsLayoutsOncePerSession pins the fix for the second
// full-type decode hidden inside DESCRIBE PAGE: resolveLayoutName looked its
// layout up by scanning ListLayouts(), so describing N pages re-listed and
// re-converted every layout in the project N times.
//
// The call count is the assertion — the resolved name only proves the lookup
// still works, not that it stopped re-listing.
func TestResolveLayoutName_ListsLayoutsOncePerSession(t *testing.T) {
	mod := mkModule("Sales")
	layoutID := model.ID("layout-atlas-default")
	calls := 0
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		// InvalidateCache drops the hierarchy along with everything else, so the
		// post-write leg below needs it to be rebuildable from the backend.
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListLayoutsFunc: func() ([]*pages.Layout, error) {
			calls++
			l := &pages.Layout{ContainerID: mod.ID, Name: "Atlas_Default"}
			l.ID = layoutID
			return []*pages.Layout{l}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	for range 3 {
		if got := resolveLayoutName(ctx, layoutID); got != "Sales.Atlas_Default" {
			t.Fatalf("resolveLayoutName = %q, want %q", got, "Sales.Atlas_Default")
		}
	}
	if calls != 1 {
		t.Fatalf("ListLayouts called %d times while describing 3 pages, want 1", calls)
	}

	// An unknown ID must still fall back to the raw ID rather than reporting a
	// wrong layout, and must not trigger a re-list hoping for a different answer.
	if got := resolveLayoutName(ctx, model.ID("layout-absent")); got != "layout-absent" {
		t.Fatalf("unknown layout = %q, want the raw ID back", got)
	}
	if calls != 1 {
		t.Fatalf("ListLayouts called %d times after an unknown ID, want 1", calls)
	}

	// A write drops the whole executor cache; the next resolve must rebuild
	// rather than answer from the pre-write model.
	ctx.InvalidateCache()
	if got := resolveLayoutName(ctx, layoutID); got != "Sales.Atlas_Default" {
		t.Fatalf("post-invalidation resolveLayoutName = %q", got)
	}
	if calls != 2 {
		t.Fatalf("ListLayouts called %d times after cache invalidation, want 2", calls)
	}
}
