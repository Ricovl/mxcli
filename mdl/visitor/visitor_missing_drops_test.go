// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// R6 (ako/mxcli#755): the missing drops parse to their statements.
func TestMissingDropsBuildTheirStatements(t *testing.T) {
	cases := []struct {
		src  string
		want ast.Statement
	}{
		{"drop database connection Shop.Erp;",
			&ast.DropDatabaseConnectionStmt{Name: ast.QualifiedName{Module: "Shop", Name: "Erp"}}},
		{"drop database connection if exists Shop.Erp;",
			&ast.DropDatabaseConnectionStmt{DropGuard: ast.DropGuard{IfExists: true}, Name: ast.QualifiedName{Module: "Shop", Name: "Erp"}}},
		{"drop external entity Shop.Customer;",
			&ast.DropEntityStmt{Name: ast.QualifiedName{Module: "Shop", Name: "Customer"}, External: true}},
		{"drop external entity if exists Shop.Customer;",
			&ast.DropEntityStmt{DropGuard: ast.DropGuard{IfExists: true}, Name: ast.QualifiedName{Module: "Shop", Name: "Customer"}, External: true}},
		{"drop validation rule for Shop.Product.Email;",
			&ast.DropValidationRuleStmt{Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Email"}}},
		{"drop validation rule for Shop.Product.Email regex;",
			&ast.DropValidationRuleStmt{Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Email"}, Kind: ast.ValidationRuleRegEx}},
		{"DROP VALIDATION RULE IF EXISTS FOR Shop.Product.Guests RANGE;",
			&ast.DropValidationRuleStmt{DropGuard: ast.DropGuard{IfExists: true},
				Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Guests"}, Kind: ast.ValidationRuleRange}},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog := mustBuild(t, c.src)
			if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], c.want) {
				t.Errorf("got %#v\nwant %#v", prog.Statements, c.want)
			}
		})
	}
}
