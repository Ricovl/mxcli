// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1472 item 5: an entity event handler's microflow was never
// resolved at check time. exec resolves it (buildEventHandlers) and refuses a
// missing one — after the statements before it are written.

func eventHandlerCtx(t *testing.T, listed bool) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	cust := mkEntity(mod.ID, "Customer")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{mkDomainModel(mod.ID, cust)}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			if !listed {
				return nil, nil
			}
			return []*microflows.Microflow{mkMicroflow(mod.ID, "BCo_Customer")}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func checkEventHandlers(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, stmt := range prog.Statements {
		if err := validateWithContext(ctx, stmt, sc); err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}

func TestEntityEventHandlerRefs_Reported(t *testing.T) {
	cases := map[string]string{
		"alter entity add event handler": `alter entity MyModule.Customer add event handler on before commit call MyModule.NoSuchHandler($currentObject) raise error;`,
		"create entity with a handler": `create persistent entity MyModule.Order (Total: Decimal)
			on after create call MyModule.NoSuchHandler($currentObject);`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(checkEventHandlers(t, eventHandlerCtx(t, true), src), "\n")
			if !strings.Contains(got, "MyModule.NoSuchHandler") || !strings.Contains(got, "event handler") {
				t.Errorf("got %q, want a finding naming the handler microflow", got)
			}
		})
	}
}

func TestEntityEventHandlerRefs_Controls(t *testing.T) {
	cases := map[string]string{
		"stored microflow": `alter entity MyModule.Customer add event handler on before commit call MyModule.BCo_Customer($currentObject) raise error;`,
		"microflow the script creates": `create microflow MyModule.BCo_New ($Customer: MyModule.Customer) returns Boolean begin return true; end;
			alter entity MyModule.Customer add event handler on before commit call MyModule.BCo_New($currentObject) raise error;`,
		"drop event handler names no microflow to resolve": `alter entity MyModule.Customer drop event handler on before commit;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			for _, e := range checkEventHandlers(t, eventHandlerCtx(t, true), src) {
				if strings.Contains(e, "event handler") {
					t.Errorf("got %q, want no event handler finding", e)
				}
			}
		})
	}
	// A backend that lists no microflows cannot say one is missing.
	for _, e := range checkEventHandlers(t, eventHandlerCtx(t, false),
		`alter entity MyModule.Customer add event handler on before commit call MyModule.Whatever($currentObject);`) {
		if strings.Contains(e, "event handler") {
			t.Errorf("empty listing: got %q, want silence", e)
		}
	}
}
