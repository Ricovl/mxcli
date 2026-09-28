// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R9 (ako/mxcli#755): an element's documentation is its documentation, never a
// "comment". The alter form is `set documentation '…'` on an entity,
// association and enumeration; `set comment '…'` is its deprecated alias
// (MDL-DEPR135) and builds the same statement.
func TestSetCommentIsSetDocumentation(t *testing.T) {
	cases := []struct{ old, canonical string }{
		{"alter entity M.E set comment 'Orders';", "alter entity M.E set documentation 'Orders';"},
		{"ALTER ASSOCIATION M.A SET COMMENT 'Link';", "ALTER ASSOCIATION M.A SET DOCUMENTATION 'Link';"},
		{"alter enumeration M.Color set comment 'Colours';", "alter enumeration M.Color set documentation 'Colours';"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.SetComment}) {
				t.Errorf("old recorded %v, want [%s]", got, deprecation.SetComment)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if len(old.Statements) != 1 || !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// `alter enumeration … set comment` parsed and built nothing: the script
// reported success and the documentation was never written (R11).
func TestAlterEnumerationSetDocumentationBuildsAStatement(t *testing.T) {
	prog := mustBuild(t, "alter enumeration M.Color set documentation 'Colours';")
	want := &ast.AlterEnumerationStmt{Name: ast.QualifiedName{Module: "M", Name: "Color"},
		Operation: ast.AlterEnumSetDocumentation, Documentation: "Colours"}
	if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], want) {
		t.Errorf("got %#v, want %#v", prog.Statements, want)
	}
}
