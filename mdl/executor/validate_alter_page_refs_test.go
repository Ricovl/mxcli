// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1472 items 1 and 2: `check -p --references` resolved only an
// ALTER PAGE's document. The widget it inserts after, replaces or drops, and
// every flow / page / entity the inserted or replacing widgets name, went
// unresolved — "All references valid", then exec refused the script.

func alterRefsCtx(t *testing.T) (*ExecContext, *countingDeps) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Scopes")
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mkMicroflow(mod.ID, "DS_Ok")}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedScopesPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, deps
}

func checkAlterPageRefs(t *testing.T, ctx *ExecContext, src string) []error {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateAlterPageReferences(ctx, prog, sc)
}

func TestAlterPageRefs_Reported(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"missing insert target": {
			`alter page MyModule.P_Scopes { insert after tbTypo { textbox t (Attribute: Name) } }`,
			[]string{"MyModule.P_Scopes", "tbTypo"},
		},
		"missing replace target": {
			`alter page MyModule.P_Scopes { replace tbTypo with { textbox t (Attribute: Name) } }`,
			[]string{"tbTypo"},
		},
		"missing drop target": {
			`alter page MyModule.P_Scopes { drop widget tbTypo }`,
			[]string{"tbTypo"},
		},
		"inserted widget's data source flow": {
			`alter page MyModule.P_Scopes { insert after tbTop { dataview dvX (DataSource: microflow MyModule.NoSuchMf) { } } }`,
			[]string{"microflow not found: MyModule.NoSuchMf"},
		},
		"replacing widgets' action page and flow": {
			`alter page MyModule.P_Scopes { replace tbTop with {
				actionbutton b1 (Caption: 'x', Action: show page MyModule.NoSuchPage)
				dataview dvY (DataSource: microflow MyModule.NoSuchMf2) { }
			} }`,
			[]string{"page not found: MyModule.NoSuchPage", "microflow not found: MyModule.NoSuchMf2"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, deps := alterRefsCtx(t)
			errs := checkAlterPageRefs(t, ctx, c.src)
			if len(errs) == 0 {
				t.Fatalf("got no errors, want one naming %v", c.want)
			}
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			joined := strings.Join(all, "\n")
			for _, w := range c.want {
				if !strings.Contains(joined, w) {
					t.Errorf("errors %q do not name %q", joined, w)
				}
			}
			if deps.saves != 0 {
				t.Errorf("validation wrote to storage %d times, want 0", deps.saves)
			}
		})
	}
}

// Controls: what exec resolves must stay clean.
func TestAlterPageRefs_Controls(t *testing.T) {
	cases := map[string]string{
		"stored target, stored flow": `alter page MyModule.P_Scopes {
			insert after tbTop { dataview dvX (DataSource: microflow MyModule.DS_Ok) { } } }`,
		"target added earlier in the statement": `alter page MyModule.P_Scopes {
			insert after tbTop { container cNew { textbox tInner (Attribute: Name) } }
			insert after tInner { textbox t2 (Attribute: Name) } }`,
		"target added by an earlier statement": `alter page MyModule.P_Scopes { insert after tbTop { textbox tNew (Attribute: Name) } };
			alter page MyModule.P_Scopes { replace tNew with { textbox t3 (Attribute: Name) } };`,
		"target renamed by SET Name": `alter page MyModule.P_Scopes { set Name = 'tbRenamed' on tbTop };
			alter page MyModule.P_Scopes { insert after tbRenamed { textbox t4 (Attribute: Name) } };`,
		"flow created by the script": `create microflow MyModule.DS_New () returns MyModule.Customer begin return empty; end;
			alter page MyModule.P_Scopes { insert after tbTop { dataview dvX (DataSource: microflow MyModule.DS_New) { } } }`,
		"page created by the script is resolved where it is written": `create page MyModule.P_New (Title: 'n', Layout: Atlas_Core.Atlas_Default) { };
			alter page MyModule.P_New { insert after anything { textbox t (Attribute: Name) } }`,
		"SET target is the SET dry run's finding": `alter page MyModule.P_Scopes { set Caption = 'x' on tbTypo }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := alterRefsCtx(t)
			if errs := checkAlterPageRefs(t, ctx, src); len(errs) != 0 {
				t.Errorf("got %v, want none", errs)
			}
		})
	}
}
