// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1472 item 3: a DataGrid 2 column's Attribute, and a header
// filter's, were never resolved. Measured on 11.13.0, the column is CE1613
// "The selected attribute 'Administration.Account.NoSuchColumnAttr' no longer
// exists." at Columns (1/1) of data grid 2 'dg'.

func columnAttrsCtx(t *testing.T, gridFlow string) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	cust := mkEntity(mod.ID, "Customer")
	cust.Attributes = []*domainmodel.Attribute{{Name: "Name"}, {Name: "Email"}}
	pg := mkPage(mod.ID, "P_Grid")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{mkDomainModel(mod.ID, cust)}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				EnclosingEntityForChildrenFunc: func(string) string { return "MyModule.Customer" },
				EnclosingDataSourceFlowFunc: func(string, bool) (string, string) {
					return gridFlow, ""
				},
			}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func checkColumnAttrs(t *testing.T, ctx *ExecContext, src string) []string {
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
	for _, err := range validateAlterPageReferences(ctx, prog, sc) {
		out = append(out, err.Error())
	}
	return out
}

func TestDataGridColumnAttrs_Reported(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"create page column": {
			`create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
				datagrid dg (DataSource: database from MyModule.Customer) {
					column (Attribute: NoSuchColumnAttr, Caption: 'x') { textfilter tf1 }
				}
			}`,
			[]string{"NoSuchColumnAttr", "MyModule.Customer", "CE1613"},
		},
		"create snippet column": {
			`create snippet MyModule.S_New {
				datagrid dg (DataSource: database from MyModule.Customer) { column (Attribute: Nmae) }
			}`,
			[]string{"`Nmae` is not an attribute of MyModule.Customer"},
		},
		"alter page inserts a new grid": {
			`alter page MyModule.P_Grid { insert after anything {
				datagrid dg2 (DataSource: database from MyModule.Customer) { column (Attribute: Nmae) } } }`,
			[]string{"`Nmae`"},
		},
		"alter page inserts a column into a stored grid": {
			`alter page MyModule.P_Grid { insert after dgStored.Name { column (Attribute: Nmae) } }`,
			[]string{"`Nmae`"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(checkColumnAttrs(t, columnAttrsCtx(t, ""), c.src), "\n")
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("errors %q do not name %q", got, w)
				}
			}
		})
	}
}

func TestDataGridColumnAttrs_Controls(t *testing.T) {
	cases := map[string]string{
		"existing attributes": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) {
				column (Attribute: Name) { textfilter tf1 (Attribute: Email) }
			} }`,
		// Measured on 11.13.0: the column builder writes no filter attribute
		// (the filter filters the column's), so a misspelt one builds clean.
		"a header filter's attribute is not written": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) {
				column (Attribute: Name) { textfilter tf1 (Attribute: NoSuchFilterAttr) }
			} }`,
		"case-insensitive like the builder": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) { column (Attribute: name) } }`,
		"association path": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) { column (Attribute: Customer_Order/Total) } }`,
		"system member": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) { column (Attribute: createdDate) } }`,
		"flow source is not established": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: microflow MyModule.DS) { column (Attribute: Whatever) } }`,
		"entity unknown to the index": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from Other.Thing) { column (Attribute: Whatever) } }`,
		"attribute added by the script": `alter entity MyModule.Customer add attribute Phone: String(20);
			create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Customer) { column (Attribute: Phone) } }`,
		"entity declared by the script": `create entity MyModule.Order (Total: Decimal);
			create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			datagrid dg (DataSource: database from MyModule.Order) { column (Attribute: Total) } }`,
		"layout grid column is not a grid column": `create page MyModule.P_New (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
			dataview dv (DataSource: database from MyModule.Customer) {
				layoutgrid lg { row { column (DesktopWidth: 6) { textbox tb (Attribute: Name) } } } } }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			for _, e := range checkColumnAttrs(t, columnAttrsCtx(t, ""), src) {
				if strings.Contains(e, "is not an attribute of") {
					t.Errorf("got %q, want no column attribute finding", e)
				}
			}
		})
	}
	// A stored grid sourced by a flow: its entity is not established here.
	for _, e := range checkColumnAttrs(t, columnAttrsCtx(t, "MyModule.DS_Grid"),
		`alter page MyModule.P_Grid { insert after dgStored.Name { column (Attribute: Nmae) } }`) {
		if strings.Contains(e, "is not an attribute of") {
			t.Errorf("flow-sourced stored grid: got %q, want silence", e)
		}
	}
}
