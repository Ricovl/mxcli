// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): the entity revoke mirrors the grant — rights (or `all`)
// first, `on entity`, the roles after `from`. The reversed form is the
// deprecated alias MDL-DEPR082 and builds the same statement.

func revokeStmt(t *testing.T, src string) (*ast.RevokeEntityAccessStmt, *ast.Program) {
	t.Helper()
	prog := mustBuild(t, src)
	if len(prog.Statements) != 1 {
		t.Fatalf("%q: %d statements, want 1", src, len(prog.Statements))
	}
	r, ok := prog.Statements[0].(*ast.RevokeEntityAccessStmt)
	if !ok {
		t.Fatalf("%q: built %T", src, prog.Statements[0])
	}
	return r, prog
}

func TestRevokeEntity_CanonicalFormAndAlias(t *testing.T) {
	for _, tc := range []struct {
		old, canonical string
		partial        bool
	}{
		{"revoke Shop.User, Shop.Admin on Shop.Order (write (Email), delete);",
			"revoke write (Email), delete on entity Shop.Order from Shop.User, Shop.Admin;", true},
		{"revoke Shop.User on Shop.Order;", "revoke all on entity Shop.Order from Shop.User;", false},
		{"REVOKE Shop.User ON Shop.Order (READ *);", "REVOKE READ * ON ENTITY Shop.Order FROM Shop.User;", true},
	} {
		old, oldProg := revokeStmt(t, tc.old)
		canon, canonProg := revokeStmt(t, tc.canonical)
		if !reflect.DeepEqual(old, canon) {
			t.Errorf("%s built %#v\n%s built %#v", tc.old, old, tc.canonical, canon)
		}
		if (len(canon.Rights) > 0) != tc.partial || len(canon.Roles) == 0 || canon.Entity.Name != "Order" {
			t.Errorf("%s built %#v", tc.canonical, canon)
		}
		if got := deprecationCodes(oldProg); !reflect.DeepEqual(got, []string{deprecation.ReversedEntityRevoke}) {
			t.Errorf("%s recorded %v, want [%s]", tc.old, got, deprecation.ReversedEntityRevoke)
		}
		if got := deprecationCodes(canonProg); len(got) != 0 {
			t.Errorf("%s recorded %v, want none", tc.canonical, got)
		}
	}
}
