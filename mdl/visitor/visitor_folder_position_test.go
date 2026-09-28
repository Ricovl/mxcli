// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R9 (ako/mxcli#755): the folder is a clause right after the document's name,
// on every document. A constant's `folder` among its trailing options and a
// snippet's second `folder` after the header are deprecated positions of the
// same clause (MDL-DEPR134) and build the same statement.
func TestFolderClauseAfterTheName(t *testing.T) {
	cases := []struct{ old, canonical string }{
		{"create constant M.Url type String default 'x' folder 'Config';",
			"create constant M.Url folder 'Config' type String default 'x';"},
		{"create or modify constant M.Url type String default 'x' folder 'Config' exposed to client;",
			"create or modify constant M.Url folder 'Config' type String default 'x' exposed to client;"},
		{"create snippet M.S (Params: { $C: M.E }) folder 'Common' { };",
			"create snippet M.S folder 'Common' (Params: { $C: M.E }) { };"},
		{"create snippet M.S folder 'Common' { };", ""}, // control: already canonical
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if c.canonical == "" {
				if got := deprecationCodes(old); len(got) != 0 {
					t.Errorf("canonical form recorded %v", got)
				}
				return
			}
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.FolderClausePosition}) {
				t.Errorf("old recorded %v, want [%s]", got, deprecation.FolderClausePosition)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// Both positions on one statement: the later one is what is stored, as before,
// and there is no rewrite (the author has to pick one).
func TestFolderClauseInBothPositionsIsNotRewritten(t *testing.T) {
	prog := mustBuild(t, "create constant M.Url folder 'A' type String default 'x' folder 'B';")
	if len(prog.Deprecations) != 1 || prog.Deprecations[0].Fix != nil || prog.Deprecations[0].NoFix == "" {
		t.Fatalf("deprecations = %+v, want one with a reason and no fix", prog.Deprecations)
	}
}
