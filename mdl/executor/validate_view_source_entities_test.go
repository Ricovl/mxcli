// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1472 item 7: the entities a view entity's OQL reads from
// were never resolved — an unknown FROM entity inferred no type and was
// skipped, so the view passed check.

func viewSourceCtx(t *testing.T, listed bool) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	cust := mkEntity(mod.ID, "Customer")
	cust.Attributes = []*domainmodel.Attribute{{Name: "Name"}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			if !listed {
				return nil, nil
			}
			return []*domainmodel.DomainModel{mkDomainModel(mod.ID, cust)}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func checkViewSources(t *testing.T, ctx *ExecContext, src string) string {
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
	return strings.Join(out, "\n")
}

func TestViewSourceEntities_Reported(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"unknown FROM entity (the reported shape)": {
			`create view entity MyModule.K7 (Name: String) as select a.Name as Name from MyModule.NoSuchEntity as a;`,
			"MyModule.NoSuchEntity",
		},
		"unknown JOIN entity": {
			`create view entity MyModule.K7 (Name: String) as select c.Name as Name from MyModule.Customer as c join MyModule.NoSuchOther as o on o.Name = c.Name;`,
			"MyModule.NoSuchOther",
		},
		"unknown end of an association-path join": {
			`create view entity MyModule.K7 (Name: String) as select c.Name as Name from MyModule.Customer as c join c/MyModule.Customer_X/MyModule.NoSuchEnd as x;`,
			"MyModule.NoSuchEnd",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := checkViewSources(t, viewSourceCtx(t, true), c.src)
			if !strings.Contains(got, c.want) || !strings.Contains(got, "does not exist") {
				t.Errorf("got %q, want a finding naming %q", got, c.want)
			}
		})
	}
}

func TestViewSourceEntities_Controls(t *testing.T) {
	cases := map[string]string{
		"stored entity": `create view entity MyModule.V (Name: String) as select c.Name as Name from MyModule.Customer as c;`,
		"entity the script creates": `create persistent entity MyModule.Order (Ref: String(20));
			create view entity MyModule.V (Ref: String(20)) as select o.Ref as Ref from MyModule.Order as o;`,
		"System entity": `create view entity MyModule.V (Name: String) as select u.Name as Name from System.User as u;`,
		"module the script creates": `create module Fresh; create persistent entity Fresh.Thing (Name: String(10));
			create view entity MyModule.V (Name: String(10)) as select t.Name as Name from Fresh.Other as t;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if got := checkViewSources(t, viewSourceCtx(t, true), src); strings.Contains(got, "does not exist") {
				t.Errorf("got %q, want no source-entity finding", got)
			}
		})
	}
	// A backend that lists no entities cannot say one is missing.
	if got := checkViewSources(t, viewSourceCtx(t, false),
		`create view entity MyModule.V (Name: String) as select a.Name as Name from MyModule.Whatever as a;`); strings.Contains(got, "does not exist") {
		t.Errorf("empty listing: got %q, want silence", got)
	}
}
