// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#865: `fmt --upgrade` removes a constant's `private`, a modifier
// that was never stored, in the clause form and the property-list form, and
// composes with the clause-to-property-list rewrite on the same statement.
func TestUpgrade_ConstantPrivateRemoved(t *testing.T) {
	src := "create or modify constant M.A type string default '' PRIVATE;\n" +
		"create or modify constant M.B type string\n  default 'k' private;\n" +
		"create constant M.C type String default 'x' private exposed to client;\n" +
		"create constant M.D ( Type: String, DefaultValue: '' ) Private;\n" +
		"create constant M.E folder 'Cfg' ( Type: String, DefaultValue: '' ) private; -- a credential\n"
	want := "create or modify constant M.A ( Type: string, DefaultValue: '' );\n" +
		"create or modify constant M.B ( Type: string, DefaultValue: 'k' );\n" +
		"create constant M.C ( Type: String, DefaultValue: 'x', ExposedToClient: true );\n" +
		"create constant M.D ( Type: String, DefaultValue: '' );\n" +
		"create constant M.E folder 'Cfg' ( Type: String, DefaultValue: '' ); -- a credential\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten[deprecation.ConstantPrivate] != 5 {
		t.Errorf("Rewritten[%s] = %d, want 5 (all: %v)", deprecation.ConstantPrivate, res.Rewritten[deprecation.ConstantPrivate], res.Rewritten)
	}
	if len(res.Unrewritten) != 0 {
		t.Errorf("Unrewritten = %+v, want none", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
