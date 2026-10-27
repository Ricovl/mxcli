// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1701: walkTypedFlow did not type the output of `call microflow`, so
// every --references check built on it was silent on a call result — the
// Protected-module member check (TestValidateProtectedModuleRefs) and the
// member-existence check here alike.

// callResultFixture is memberFixture plus a stored Shop.GetOrder returning
// Shop.Order.
func callResultFixture(t *testing.T) *ExecContext {
	t.Helper()
	ctx := memberFixture(t)
	shop, err := ctx.Backend.GetModuleByName("Shop")
	if err != nil {
		t.Fatal(err)
	}
	get := mkMicroflow(shop.ID, "GetOrder")
	get.ReturnType = &microflows.ObjectType{EntityQualifiedName: "Shop.Order"}
	ctx.Backend.(*mock.MockBackend).ListMicroflowsFunc = func() ([]*microflows.Microflow, error) {
		return []*microflows.Microflow{get}, nil
	}
	return ctx
}

func memberMessages(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateMemberReferences(ctx, prog, sc)
}

func TestValidateMemberReferences_TypesACallResult(t *testing.T) {
	ctx := callResultFixture(t)
	msgs := memberMessages(t, ctx,
		"create microflow Shop.ACT () begin $o = call microflow Shop.GetOrder(); change $o (IsArchived = true); end;")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(msgs), msgs)
	}
	for _, want := range []string{"Shop.Order", "IsArchived", "CE1613"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("message should mention %q: %s", want, msgs[0])
		}
	}
}

// CONTROL: a call result types only as far as the callee is established, and
// the script's own signature for it wins over the stored one — that is the one
// exec writes. Typing a rewritten callee from the stored return type would
// refuse a member the new return type has: a false refusal that blocks exec.
func TestValidateMemberReferences_CallResultStaysSilentWhereUnestablished(t *testing.T) {
	ctx := callResultFixture(t)
	for _, tc := range []struct{ name, src string }{
		{"a member the returned entity has",
			"create microflow Shop.ACT () begin $o = call microflow Shop.GetOrder(); change $o (Status = 'x'); end;"},
		{"a callee rewritten to return another entity",
			"create or modify microflow Shop.GetOrder () returns Shop.Customer begin $c = create Shop.Customer; return $c; end;\n" +
				"create microflow Shop.ACT () begin $o = call microflow Shop.GetOrder(); change $o (Name = 'x'); end;"},
		{"a callee rewritten to return no object",
			"create or modify microflow Shop.GetOrder () returns Boolean begin return true; end;\n" +
				"create microflow Shop.ACT () begin $o = call microflow Shop.GetOrder(); change $o (Name = 'x'); end;"},
		{"a callee the project does not have",
			"create microflow Shop.ACT () begin $o = call microflow Shop.Nope(); change $o (Whatever = 'x'); end;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if msgs := memberMessages(t, ctx, tc.src); len(msgs) != 0 {
				t.Errorf("want no message, got %v", msgs)
			}
		})
	}
}
