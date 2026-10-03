// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `HashedString` parsed as a type keyword (HASHEDSTRING_TYPE is in the dataType
// rule) but buildDataType had no branch for it, so it fell through to the
// function's last line — `TypeString` — and the attribute was written as
// DomainModels$StringAttributeType, unlimited length: no error, no warning. A
// password column stored in clear text is the result.

func TestHashedStringAttributeKind(t *testing.T) {
	stmts := buildOK(t, `create persistent entity M.E (Pwd: HashedString, Name: String(100));`)
	ce, ok := stmts[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateEntityStmt", stmts[0])
	}
	if got := ce.Attributes[0].Type.Kind; got != ast.TypeHashedString {
		t.Errorf("Pwd: HashedString built kind %v, want HashedString", got)
	}
	// Control: the neighbouring String attribute is still a String.
	if got := ce.Attributes[1].Type; got.Kind != ast.TypeString || got.Length != 100 {
		t.Errorf("Name: String(100) built %v(%d), want String(100)", got.Kind, got.Length)
	}
}

func TestHashedStringModifyAttributeKind(t *testing.T) {
	stmts := buildOK(t, `alter entity M.E modify attribute Pwd: HashedString;`)
	ae, ok := stmts[0].(*ast.AlterEntityStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.AlterEntityStmt", stmts[0])
	}
	if ae.DataType.Kind != ast.TypeHashedString {
		t.Errorf("modify attribute Pwd: HashedString built kind %v, want HashedString", ae.DataType.Kind)
	}
}

// Guard the class, not the instance: every primitive type keyword an attribute
// can be declared with must build its own kind. A keyword missing from
// buildDataType falls through to String, which is a legal type — so the loss is
// silent all the way to disk. (Date is the documented exception: a deprecated
// spelling of DateTime, MDL-DEPR160.)
func TestEveryAttributeTypeKeywordBuildsItsOwnKind(t *testing.T) {
	cases := []struct {
		typ  string
		want ast.DataTypeKind
	}{
		{"String(10)", ast.TypeString},
		{"Integer", ast.TypeInteger},
		{"Long", ast.TypeLong},
		{"Decimal", ast.TypeDecimal},
		{"Boolean", ast.TypeBoolean},
		{"DateTime", ast.TypeDateTime},
		{"AutoNumber", ast.TypeAutoNumber},
		{"AutoOwner", ast.TypeAutoOwner},
		{"AutoChangedBy", ast.TypeAutoChangedBy},
		{"AutoCreatedDate", ast.TypeAutoCreatedDate},
		{"AutoChangedDate", ast.TypeAutoChangedDate},
		{"Binary", ast.TypeBinary},
		{"HashedString", ast.TypeHashedString},
		{"Enumeration(M.Color)", ast.TypeEnumeration},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			stmts := buildOK(t, `create persistent entity M.E (A: `+c.typ+`);`)
			ce := stmts[0].(*ast.CreateEntityStmt)
			if got := ce.Attributes[0].Type.Kind; got != c.want {
				t.Errorf("A: %s built kind %v, want %v", c.typ, got, c.want)
			}
		})
	}
}

// HashedString exists only as a DomainModels attribute type. Microflows,
// constants, Java actions and the like have no hashed-string type, so there the
// word was stored as something else (a String; a microflow parameter as
// Void). It is refused where it is written, like Float and Currency (#706).
func TestHashedStringRefusedOutsideAttributes(t *testing.T) {
	wantRejected(t, `create microflow M.MF ($p: HashedString) begin end;`, "HashedString", "String")
	wantRejected(t, `create microflow M.MF () begin declare $x HashedString = empty; end;`, "HashedString", "String")
	wantRejected(t, `create microflow M.MF () returns HashedString begin return empty; end;`, "HashedString", "String")
	wantRejected(t, `create constant M.C (Type: HashedString, DefaultValue: 'x');`, "HashedString", "String")
	wantRejected(t, `create java action M.JA (P: HashedString) returns Boolean;`, "HashedString", "String")
}
