// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `drop microflow` printed only "Dropped microflow", so a drop in one run and a
// create in the next silently lost the module-role grants (CapTrack's apply.sh,
// CE0106; ako/mxcli#944). The drop must name the roles it removes and say that
// only a create in the same script or session carries them. The control is a
// flow with no grants, which has nothing to say.
func TestDropMicroflow_NamesTheGrantsItRemoves(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []model.ID
		want  []string
	}{
		{"granted", []model.ID{"MyModule.User", "MyModule.Admin"}, []string{
			"Removed its execute grants to MyModule.User, MyModule.Admin",
			"later in this script or session carries them",
			"grant execute on microflow MyModule.DoSomething to MyModule.User, MyModule.Admin;",
		}},
		{"control: no grants", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			mf := mkMicroflow(mod.ID, "DoSomething")
			mf.AllowedModuleRoles = tc.roles
			h := mkHierarchy(mod)
			withContainer(h, mf.ContainerID, mod.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc:     func() bool { return true },
				ListMicroflowsFunc:  func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
				DeleteMicroflowFunc: func(model.ID) error { return nil },
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, execDropMicroflow(ctx, &ast.DropMicroflowStmt{
				Name: ast.QualifiedName{Module: "MyModule", Name: "DoSomething"},
			}))
			out := buf.String()
			for _, w := range tc.want {
				assertContainsStr(t, out, w)
			}
			if tc.want == nil && strings.Contains(out, "Removed") {
				t.Errorf("a flow with no grants reported removed grants:\n%s", out)
			}
		})
	}
}

func TestDropNanoflow_NamesTheGrantsItRemoves(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "DoIt")
	nf.AllowedModuleRoles = []model.ID{"MyModule.User"}
	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		DeleteNanoflowFunc: func(model.ID) error { return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropNanoflow(ctx, &ast.DropNanoflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "DoIt"},
	}))
	assertContainsStr(t, buf.String(), "Removed its execute grants to MyModule.User")
	assertContainsStr(t, buf.String(), "grant execute on nanoflow MyModule.DoIt to MyModule.User;")
}

// A page is not remembered across its drop, so no create carries its grants;
// the note must not promise that one does.
func TestDropPage_NamesTheGrantsItRemoves(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "HomePage")
	pg.AllowedRoles = []model.ID{"MyModule.User"}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		DeletePageFunc:  func(model.ID) error { return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropPage(ctx, &ast.DropPageStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "HomePage"},
	}))
	out := buf.String()
	assertContainsStr(t, out, "Removed its view grants to MyModule.User. A create of MyModule.HomePage does not carry them")
	assertContainsStr(t, out, "grant view on page MyModule.HomePage to MyModule.User;")
	if strings.Contains(out, "carries them") {
		t.Errorf("a page drop promised a carry that does not happen:\n%s", out)
	}
}
