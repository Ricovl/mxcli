// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1617: `show page M.P (NumberOfPagesToClose: 1)` is an
// argument for a page parameter called NumberOfPagesToClose, not the action's
// setting. `check --references` said "All references valid" and exec wrote it;
// mxbuild 11.12.5 then failed it with CE1613 "The selected page parameter
// 'MyFirstModule.Booking_Overview.NumberOfPagesToClose' no longer exists". The
// flow calls had this check (#953); show page did not.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestValidate_ShowPageParameterNames(t *testing.T) {
	arg := func(name string) []ast.ShowPageArg {
		return []ast.ShowPageArg{{ParamName: name, Value: &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: int64(1)}}}
	}
	show := func(page string, args []ast.ShowPageArg) ast.MicroflowStatement {
		return &ast.ShowPageStmt{PageName: qn("M", page), Arguments: args}
	}

	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	stored := &pages.Page{Name: "Stored", Parameters: []*pages.PageParameter{{Name: "Order"}}}
	stored.ContainerID = "mod1"
	storedNone := &pages.Page{Name: "StoredNone"}
	storedNone.ContainerID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{stored, storedNone}, nil },
	}

	cases := []struct {
		name    string
		stmt    ast.MicroflowStatement
		wantErr []string
	}{
		{"the reported spelling, on a page created in the script", show("Booking_Overview", arg("NumberOfPagesToClose")), []string{
			`page M.Booking_Overview has no parameter "NumberOfPagesToClose" (declared parameters: none)`,
			`CE1613 "The selected page parameter`,
			"with NumberOfPagesToClose = 1",
		}},
		{"a misspelt parameter of a stored page", show("Stored", arg("Ordr")), []string{
			`page M.Stored has no parameter "Ordr" (declared parameters: Order)`,
		}},
		{"a stored page taking no parameters", show("StoredNone", arg("Order")), []string{
			`(declared parameters: none)`,
		}},
		{"control: a declared parameter of a stored page", show("Stored", arg("Order")), nil},
		{"control: no arguments", show("StoredNone", nil), nil},
		{"control: a page that does not resolve is the not-found check's", show("Missing", arg("X")), []string{
			"page not found: M.Missing",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := newMockCtx(t, withBackend(mb))
			body := []ast.MicroflowStatement{tc.stmt}
			prog := &ast.Program{Statements: []ast.Statement{
				&ast.CreatePageStmtV3{Name: qn("M", "Booking_Overview")},
				&ast.CreateMicroflowStmt{Name: qn("M", "Caller"), Body: body},
			}}
			sc := newScriptContext()
			sc.collectDefinitions(prog)
			errs := validateFlowBodyReferences(ctx, body, sc)
			if len(tc.wantErr) == 0 {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want one", errs)
			}
			for _, w := range tc.wantErr {
				if !strings.Contains(errs[0], w) {
					t.Errorf("the error does not say %q:\n%s", w, errs[0])
				}
			}
		})
	}
}
