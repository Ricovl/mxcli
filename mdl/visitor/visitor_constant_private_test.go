// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#865: `create constant … private` was taught by the
// database-connections skill and used on credential constants in the wild.
// Up to v0.24.0 it parsed because the trailing word became a help statement of
// its own (the grammar's catch-all, closed by R7), which built nothing: the
// modifier was never stored. R7 turned it into a parse error, so a headerless
// script that ran stopped running, and `fmt --upgrade` could not remove it.
//
// Without the header it parses again, means nothing, and warns MDL-DEPR138;
// under mdl 1 it is refused.
func TestConstantPrivateIsANoOpAlias(t *testing.T) {
	cases := []struct{ old, canonical string }{
		{"create constant M.C type String default '' private;",
			"create constant M.C type String default '';"},
		{"create or modify constant M.Key type string\n  default 'k' PRIVATE;",
			"create or modify constant M.Key type string\n  default 'k';"},
		{"create constant M.C type String default 'x' Private exposed to client;",
			"create constant M.C type String default 'x' exposed to client;"},
		{"create constant M.C type String default 'x' exposed to client private;",
			"create constant M.C type String default 'x' exposed to client;"},
		{"create constant M.C ( Type: String, DefaultValue: '' ) private;",
			"create constant M.C ( Type: String, DefaultValue: '' );"},
		{"create constant M.C folder 'Cfg' ( Type: String, DefaultValue: '' ) PRIVATE;",
			"create constant M.C folder 'Cfg' ( Type: String, DefaultValue: '' );"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			canon := mustBuild(t, c.canonical)
			got := deprecationCodes(old)
			want := append(deprecationCodes(canon), deprecation.ConstantPrivate)
			if !sameCodes(got, want) {
				t.Errorf("old form recorded %v, want %v", got, want)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// Under mdl 1 the no-op is refused: the header is the opt-in to a language
// that never had it.
func TestConstantPrivateRefusedUnderMdl1(t *testing.T) {
	for _, stmt := range []string{
		"create constant M.C ( Type: String, DefaultValue: '' ) private;",
		"create constant M.C ( Type: String, DefaultValue: '' ) PRIVATE;",
	} {
		_, errs := Build("mdl 1;\n" + stmt)
		if len(errs) == 0 {
			t.Fatalf("%s: no error under mdl 1", stmt)
		}
		msg := errs[0].Error()
		for _, want := range []string{"line 2", "private", "never stored", "mxcli constant set"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error %q does not mention %q", stmt, msg, want)
			}
		}
	}
}

// `private` stays a word: it is not reserved, so it still names things.
func TestPrivateIsNotReserved(t *testing.T) {
	mustBuild(t, "create entity M.Private ( private: String(20) );")
	mustBuild(t, "create constant M.private ( Type: String, DefaultValue: '' );")
}

func sameCodes(a, b []string) bool {
	count := map[string]int{}
	for _, c := range a {
		count[c]++
	}
	for _, c := range b {
		count[c]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}
