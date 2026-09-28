// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// R10 (ako/mxcli#755) made `ai` and `sample` lexer keywords (`ai model`,
// `json structure … sample`). A word that becomes a keyword must stay usable as
// the name it already was: an enumeration value called Sample or AI could be
// created, but `alter enumeration` took only a bare IDENTIFIER for the value,
// so every alter of such a value stopped parsing.
func TestAlterEnumerationValueNamedLikeAKeyword(t *testing.T) {
	for _, v := range []string{"Sample", "AI", "Model"} {
		cases := []struct {
			src  string
			want *ast.AlterEnumerationStmt
		}{
			{"alter enumeration M.E add value " + v + " caption 'c';",
				&ast.AlterEnumerationStmt{Operation: ast.AlterEnumAdd, ValueName: v, Caption: "c"}},
			{"alter enumeration M.E drop value if exists " + v + ";",
				&ast.AlterEnumerationStmt{Operation: ast.AlterEnumDrop, ValueName: v, IfExists: true}},
			{"alter enumeration M.E rename value " + v + " to " + v + "2;",
				&ast.AlterEnumerationStmt{Operation: ast.AlterEnumRename, ValueName: v, NewName: v + "2"}},
			{"alter enumeration M.E rename value Old to " + v + ";",
				&ast.AlterEnumerationStmt{Operation: ast.AlterEnumRename, ValueName: "Old", NewName: v}},
			{"alter enumeration M.E modify value " + v + " caption 'c';",
				&ast.AlterEnumerationStmt{Operation: ast.AlterEnumModifyCaption, ValueName: v, Caption: "c"}},
		}
		for _, c := range cases {
			t.Run(c.src, func(t *testing.T) {
				prog := mustBuild(t, c.src)
				c.want.Name = ast.QualifiedName{Module: "M", Name: "E"}
				if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], c.want) {
					t.Errorf("got %#v, want %#v", prog.Statements, c.want)
				}
			})
		}
	}
}
