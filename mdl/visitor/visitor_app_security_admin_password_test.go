// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#624 (ako/mxcli#1259): CE0129 "Administrator password has
// not been set" had no MDL remedy — `alter app security` took no AdminPassword.
func TestAlterAppSecurityAdminPassword(t *testing.T) {
	prog := mustBuild(t, "alter app security (AdminPassword: 'N3w-Admin-Passw0rd');")
	want := &ast.AlterProjectSecurityStmt{AdminPassword: "N3w-Admin-Passw0rd"}
	if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], want) {
		t.Fatalf("got %#v, want %#v", prog.Statements, want)
	}
	for src, msg := range map[string]string{
		// An empty password is what CE0129 reports; writing one cannot be the fix.
		"alter app security ( AdminPassword: '' );":   "CE0129",
		"alter app security ( AdminPassword: true );": "string",
		"alter app security ( AdminPassword: 12 );":   "string",
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Errorf("%s: accepted, want an error mentioning %q", src, msg)
			continue
		}
		if !strings.Contains(errs[0].Error(), msg) {
			t.Errorf("%s: error %q does not mention %q", src, errs[0], msg)
		}
	}
}
