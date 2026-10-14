// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// Service-level authentication parameters (mendixlabs/mxcli#1417):
// Rest$PublishedRestService.Parameters binds the custom-authentication
// microflow's own parameters to query or header values of the request. The
// Siemens FSM template stores one (NativeRestAPI.DocumentShare: query `jwt`
// bound to $Token); before, MDL could neither print nor state it, and MDL-REST04
// refused the microflow outright.

func authParamsOf(ps []*model.PublishedRestOperationParameter) string {
	var parts []string
	for _, p := range ps {
		s := fmt.Sprintf("%s %s %s->%s", p.ParameterType, p.DataType, p.Name, p.MicroflowParameter)
		if p.Description != "" {
			s += " [" + p.Description + "]"
		}
		if p.ID != "" {
			s += " #" + string(p.ID)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

func createWithAuth(auth string) string {
	return `create or modify published rest service RestQ.Orders (
  Path: 'rest/orders/v1',
  Authentication: (` + auth + `)
) {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`
}

const tenantMf = "RestQ.AuthenticateTenant."

// An unlisted parameter is a query parameter under its own name, in the
// microflow's order; the request and the response are never parameters. So the
// issue's microflow works with no list at all.
func TestPublishedRestAuthParameters_Defaulted(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t)
	assertNoError(t, execPublishedRest(t, ctx, createWithAuth("basic, microflow RestQ.AuthenticateTenant")))
	want := "Query String Token->" + tenantMf + "Token; Query Integer Tenant->" + tenantMf + "Tenant; Query String Lang->" + tenantMf + "Lang"
	if got := authParamsOf(f.created.Parameters); got != want {
		t.Errorf("parameters =\n  %s\nwant\n  %s", got, want)
	}
}

// The approved syntax: kind, an HTTP name when it differs, a description. The
// listed entries come first in the order written, the unlisted ones after them
// in the microflow's order.
func TestPublishedRestAuthParameters_Listed(t *testing.T) {
	cases := []struct{ list, want string }{
		{`$Token: header 'X-Api-Token', $Tenant: query, $Lang: query 'lang' description 'UI language, defaults to en'`,
			"Header String X-Api-Token->" + tenantMf + "Token; Query Integer Tenant->" + tenantMf + "Tenant; Query String lang->" + tenantMf + "Lang [UI language, defaults to en]"},
		{`$Lang: header,`,
			"Header String Lang->" + tenantMf + "Lang; Query String Token->" + tenantMf + "Token; Query Integer Tenant->" + tenantMf + "Tenant"},
		{``,
			"Query String Token->" + tenantMf + "Token; Query Integer Tenant->" + tenantMf + "Tenant; Query String Lang->" + tenantMf + "Lang"},
	}
	for _, c := range cases {
		t.Run(c.list, func(t *testing.T) {
			f, ctx, _ := newPublishedRestFixture(t)
			assertNoError(t, execPublishedRest(t, ctx, createWithAuth("microflow RestQ.AuthenticateTenant ("+c.list+")")))
			if got := authParamsOf(f.created.Parameters); got != c.want {
				t.Errorf("parameters =\n  %s\nwant\n  %s", got, c.want)
			}
		})
	}
}

// MDL-REST04 refuses what Studio Pro could not store: a parameter the
// microflow does not have, the request or the response (Mendix supplies those),
// a kind other than query or header, and one parameter bound twice.
func TestPublishedRestAuthParameters_Refused(t *testing.T) {
	cases := []struct{ list, want string }{
		{`$Nope: query`, "$Nope"},
		{`$HttpRequest: header`, "$HttpRequest"},
		{`$Token: path`, "path"},
		{`$Token: body`, "body"},
		{`$Token: query, $Token: header 'X'`, "twice"},
	}
	for _, c := range cases {
		t.Run(c.list, func(t *testing.T) {
			f, ctx, _ := newPublishedRestFixture(t)
			err := execPublishedRest(t, ctx, createWithAuth("microflow RestQ.AuthenticateTenant ("+c.list+")"))
			if err == nil || !strings.Contains(err.Error(), "MDL-REST04") || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want MDL-REST04 naming %q", err, c.want)
			}
			if f.created != nil {
				t.Errorf("wrote the service despite the refusal")
			}
			// check --references reports the same before anything runs.
			_, vctx, _ := newPublishedRestFixture(t)
			errs := enumErrors(validateProgram(vctx, parseMDL(t, createWithAuth("microflow RestQ.AuthenticateTenant ("+c.list+")"))))
			if !strings.Contains(errs, "MDL-REST04") || !strings.Contains(errs, c.want) {
				t.Errorf("check: errors %q, want MDL-REST04 naming %q", errs, c.want)
			}
		})
	}
}

// storedTenantService is a Studio Pro service whose authentication
// parameters carry stored $IDs.
func storedTenantService() *model.PublishedRestService {
	s := storedAuthService([]string{"Microflow"}, "RestQ.AuthenticateTenant")
	s.Parameters = []*model.PublishedRestOperationParameter{
		{ID: "p-token", Name: "jwt", ParameterType: "Query", MicroflowParameter: tenantMf + "Token", DataType: "String"},
		{ID: "p-tenant", Name: "X-Tenant", ParameterType: "Header", MicroflowParameter: tenantMf + "Tenant", DataType: "Integer", Description: "tenant id"},
		{ID: "p-lang", Name: "Lang", ParameterType: "Query", MicroflowParameter: tenantMf + "Lang", DataType: "String"},
	}
	return s
}

// A statement that does not state authentication keeps the stored parameters,
// as it keeps the stored methods.
func TestPublishedRestAuthParameters_UnstatedAreKept(t *testing.T) {
	for name, src := range map[string]string{
		"create or modify": `create or modify published rest service RestQ.Orders (Path: 'rest/orders/v2') {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`,
		"alter": `alter published rest service RestQ.Orders set (Version: '2.0.0');`,
	} {
		t.Run(name, func(t *testing.T) {
			stored := storedTenantService()
			want := authParamsOf(stored.Parameters)
			f, ctx, _ := newPublishedRestFixture(t, stored)
			assertNoError(t, execPublishedRest(t, ctx, src))
			if got := authParamsOf(f.updated.Parameters); got != want {
				t.Errorf("parameters =\n  %s\nwant the stored\n  %s", got, want)
			}
		})
	}
}

// Stating the parameters rewrites them, and each keeps the stored $ID of the
// element bound to the same microflow parameter — not of the one in the same
// position, which is what a reorder would otherwise hand it.
func TestPublishedRestAuthParameters_CarryStoredIDsByBinding(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t, storedTenantService())
	assertNoError(t, execPublishedRest(t, ctx, createWithAuth("microflow RestQ.AuthenticateTenant ($Lang: header 'X-Lang', $Token: query)")))
	want := "Header String X-Lang->" + tenantMf + "Lang #p-lang; Query String Token->" + tenantMf + "Token #p-token; Query Integer Tenant->" + tenantMf + "Tenant #p-tenant"
	if got := authParamsOf(f.updated.Parameters); got != want {
		t.Errorf("parameters =\n  %s\nwant\n  %s", got, want)
	}
}

// Methods without the microflow clear the parameters: they bind the
// microflow's parameters, and Studio Pro clears the microflow.
func TestPublishedRestAuthParameters_ClearedWithTheMicroflow(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t, storedTenantService())
	assertNoError(t, execPublishedRest(t, ctx, `alter published rest service RestQ.Orders set (Authentication: (basic));`))
	if len(f.updated.Parameters) != 0 {
		t.Errorf("parameters = %s, want none", authParamsOf(f.updated.Parameters))
	}
}

// describe prints the list only when the stored parameters are not what
// defaulting produces, so existing output does not change; and executing what
// it prints writes the stored parameters back.
func TestDescribePublishedRestService_AuthParameters(t *testing.T) {
	defaulted := func() *model.PublishedRestService {
		s := storedAuthService([]string{"Microflow"}, "RestQ.AuthenticateTenant")
		s.Parameters = []*model.PublishedRestOperationParameter{
			{Name: "Token", ParameterType: "Query", MicroflowParameter: tenantMf + "Token", DataType: "String"},
			{Name: "Tenant", ParameterType: "Query", MicroflowParameter: tenantMf + "Tenant", DataType: "Integer"},
			{Name: "Lang", ParameterType: "Query", MicroflowParameter: tenantMf + "Lang", DataType: "String"},
		}
		return s
	}
	reordered := defaulted()
	reordered.Parameters[0], reordered.Parameters[2] = reordered.Parameters[2], reordered.Parameters[0]

	cases := []struct {
		name   string
		stored *model.PublishedRestService
		line   string
	}{
		{"default", defaulted(), "Authentication: (microflow RestQ.AuthenticateTenant)"},
		{"header, rename, description", storedTenantService(),
			"Authentication: (microflow RestQ.AuthenticateTenant ($Token: query 'jwt', $Tenant: header 'X-Tenant' description 'tenant id', $Lang: query))"},
		{"order", reordered,
			"Authentication: (microflow RestQ.AuthenticateTenant ($Lang: query, $Tenant: query, $Token: query))"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := authParamsOf(c.stored.Parameters)
			f, ctx, out := newPublishedRestFixture(t, c.stored)
			assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "RestQ", Name: "Orders"}))
			text := out.String()
			if !strings.Contains(text, "  "+c.line+"\n") {
				t.Fatalf("describe lacks %q:\n%s", c.line, text)
			}
			// Replay as a fresh create, so nothing stored can fill a gap.
			f.stored = nil
			stmt := text[:strings.Index(text, "};")+2]
			assertNoError(t, execPublishedRest(t, ctx, stmt))
			if got := authParamsOf(f.created.Parameters); got != stripIDs(want) {
				t.Errorf("replayed as\n  %s\nwant\n  %s", got, stripIDs(want))
			}
		})
	}
}

func stripIDs(s string) string {
	var out []string
	for _, part := range strings.Split(s, "; ") {
		if i := strings.Index(part, " #"); i >= 0 {
			part = part[:i]
		}
		out = append(out, part)
	}
	return strings.Join(out, "; ")
}
