// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateRestClient_Basic(t *testing.T) {
	input := `CREATE REST CLIENT MyModule.PetAPI (
		BaseUrl: 'https://api.example.com/v1'
	) {
		OPERATION GetPets {
			Method: GET,
			Path: '/pets',
			Response: JSON AS $PetList
		}
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateRestClientStmt)
	if !ok {
		t.Fatalf("Expected CreateRestClientStmt, got %T", prog.Statements[0])
	}
	if stmt.BaseUrl != "https://api.example.com/v1" {
		t.Errorf("Got BaseUrl %q", stmt.BaseUrl)
	}
	if stmt.Authentication != nil {
		t.Error("Expected nil Authentication")
	}
	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}
	op := stmt.Operations[0]
	if op.Name != "GetPets" {
		t.Errorf("Got Name %q", op.Name)
	}
	if op.Method != "get" {
		t.Errorf("Got Method %q", op.Method)
	}
}

func TestCreatePublishedRestService(t *testing.T) {
	input := `CREATE PUBLISHED REST SERVICE MyModule.OrderAPI (
		Path: '/api/v1',
		Version: '1.0.0',
		ServiceName: 'Orders'
	) {
		RESOURCE 'orders' {
			GET '/{id}' MICROFLOW MyModule.GetOrder;
			POST '/' MICROFLOW MyModule.CreateOrder;
		}
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePublishedRestServiceStmt)
	if !ok {
		t.Fatalf("Expected CreatePublishedRestServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.Path != "/api/v1" {
		t.Errorf("Got Path %q", stmt.Path)
	}
	if stmt.ServiceName != "Orders" {
		t.Errorf("Got ServiceName %q", stmt.ServiceName)
	}
	if len(stmt.Resources) != 1 {
		t.Fatalf("Expected 1 resource, got %d", len(stmt.Resources))
	}
	if stmt.Resources[0].Name != "orders" {
		t.Errorf("Got resource name %q", stmt.Resources[0].Name)
	}
	if len(stmt.Resources[0].Operations) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(stmt.Resources[0].Operations))
	}
}

// TestCreatePublishedRestService_EndResourceSyntax_NoPanic verifies that the
// unsupported `end resource` keyword syntax does not cause a SIGSEGV (issue #429).
// ANTLR error recovery produces a PublishedRestResourceContext with a nil
// STRING_LITERAL token; the nil guard in buildPublishedRestResourceDef must
// prevent the panic and let Build return without crashing.
func TestCreatePublishedRestService_EndResourceSyntax_NoPanic(t *testing.T) {
	input := "create published rest service MyModule.TestREST (Version: '1.0', Path: '/api') resource 'items' get '/all' microflow MyModule.GetItems; end resource;"
	// Must not panic — parse errors are acceptable, a crash is not.
	prog, _ := Build(input)
	_ = prog
}

// TestCreatePublishedRestService_NonStringPropertyValue_NoPanic reproduces
// mendixlabs/mxcli#1331: a property value that is not a string literal
// (`Authentication: microflow M.F`, `Authentication: Basic`,
// `Folder: microflow M.F`) crashed the binary with a nil pointer dereference in
// unquoteStringLit. Build walks a failed parse on purpose, so the rule context
// exists with no STRING_LITERAL child; the author must get the syntax error.
func TestCreatePublishedRestService_NonStringPropertyValue_NoPanic(t *testing.T) {
	for name, value := range map[string]string{
		"microflow":      "Authentication: microflow MyFirstModule.AuthMf",
		"keyword":        "Authentication: Basic",
		"folder-non-str": "Folder: microflow MyFirstModule.AuthMf",
	} {
		t.Run(name, func(t *testing.T) {
			input := `CREATE OR MODIFY PUBLISHED REST SERVICE MyFirstModule.TestApi (
  Path: 'rest/test/v1',
  Version: '1.0.0',
  ServiceName: 'Test API',
  ` + value + `
)
{
  RESOURCE 'items' {
    GET '' MICROFLOW MyFirstModule.GetItems;
  }
};`
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic for %q: %v", value, r)
				}
			}()
			_, errs := Build(input)
			if len(errs) == 0 {
				t.Fatalf("expected a syntax error for %q, got none", value)
			}
		})
	}
}

// mendixlabs/mxcli#1608: ALTER CONSUMED REST SERVICE, every action, and the
// keys a modify names (only those are rewritten).
func TestAlterRestClient_Actions(t *testing.T) {
	prog, errs := Build(`alter consumed rest service M.Api
  add operation GetBooking (Method: get, Path: '/b/{id}', Parameters: ($id: String), Response: json),
  modify operation PostBooking (Timeout: 60, Method: put),
  drop operation OldBooking,
  set (BaseUrl: 'https://x/v2', Authentication: basic (Username: 'u', Password: @M.Pw));
alter rest client M.Api set (Authentication: none);`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 2 {
		t.Fatalf("got %d statements", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.AlterRestClientStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if stmt.Name.String() != "M.Api" || len(stmt.Actions) != 4 {
		t.Fatalf("got %s with %d actions", stmt.Name, len(stmt.Actions))
	}
	add := stmt.Actions[0].(*ast.RestClientAddOperationAction)
	if add.Operation.Name != "GetBooking" || add.Operation.Path != "/b/{id}" {
		t.Errorf("add: %+v", add.Operation)
	}
	mod := stmt.Actions[1].(*ast.RestClientModifyOperationAction)
	if got := strings.Join(mod.Operation.Keys, ","); got != "Timeout,Method" || mod.Operation.Timeout != 60 {
		t.Errorf("modify keys %q, timeout %d", got, mod.Operation.Timeout)
	}
	if drop := stmt.Actions[2].(*ast.RestClientDropOperationAction); drop.Name != "OldBooking" {
		t.Errorf("drop: %q", drop.Name)
	}
	set := stmt.Actions[3].(*ast.RestClientSetAction)
	if !set.BaseUrlSet || set.BaseUrl != "https://x/v2" || !set.AuthenticationSet || set.Authentication == nil ||
		set.Authentication.Password != "$M.Pw" {
		t.Errorf("set: %+v", set)
	}
	none := prog.Statements[1].(*ast.AlterRestClientStmt).Actions[0].(*ast.RestClientSetAction)
	if !none.AuthenticationSet || none.Authentication != nil || none.BaseUrlSet {
		t.Errorf("set (Authentication: none): %+v", none)
	}
}

// A property ALTER cannot set is reported, not dropped.
func TestAlterRestClient_SetRejectsUnknownProperty(t *testing.T) {
	_, errs := Build(`mdl 1;
alter consumed rest service M.Api set (OpenApi: 'spec.json');`)
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "OpenApi") {
		t.Errorf("errs = %v", errs)
	}
}
