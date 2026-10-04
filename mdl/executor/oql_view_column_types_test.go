// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#981 items 4 and 5 — view-entity column types check accepted and
// mxbuild reports as CE6770 (measured, 11.13.0).
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Item 5 — `r.Name + ' x'` is String(200) in mxbuild whatever Name's length:
// declared String(200) builds, `string` and `string(100)` are CE6770.
func TestStringConcatenationIsADerivedString200(t *testing.T) {
	const oql = `select r.Name + ' x' as S from MyFirstModule.Race as r`
	for _, c := range []struct {
		declared ast.DataType
		refused  bool
	}{
		{ast.DataType{Kind: ast.TypeString}, true},
		{ast.DataType{Kind: ast.TypeString, Length: 100}, true},
		{ast.DataType{Kind: ast.TypeString, Length: 102}, true},
		{ast.DataType{Kind: ast.TypeString, Length: 200}, false},
	} {
		vs := ValidateOQLTypes(oql, []ast.ViewAttribute{{Name: "S", Type: c.declared}})
		if (len(vs) > 0) != c.refused {
			t.Errorf("declared %s: refused=%v, want %v (%v)", formatDataTypeForError(c.declared), len(vs) > 0, c.refused, vs)
		}
	}
	// Control: a literal on the left builds as String(200) too (measured).
	if vs := ValidateOQLTypes(`select 'Season ' + r.Name as S from MyFirstModule.Race as r`,
		[]ast.ViewAttribute{{Name: "S", Type: ast.DataType{Kind: ast.TypeString, Length: 200}}}); len(vs) > 0 {
		t.Errorf("'Season ' + r.Name declared String(200) was refused: %v", vs)
	}
	// Numeric `+` is not a string.
	if got := inferTypeStatic("r.Season + 1"); got.Kind == ast.TypeString {
		t.Errorf("r.Season + 1 inferred as a string")
	}
}

// Item 4 — a view attribute declared AutoNumber over an AutoNumber column is
// CE6770 (measured, 11.13.0); Long builds. A new rejection, so per ADR-0011 it
// is an error under `mdl 1;` and a warning without the header.
func TestViewAutoNumberOverAutoNumberIsGatedOnTheHeader(t *testing.T) {
	race := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-race", TypeName: "DomainModels$Entity"},
		Name:        "Race",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "a-nr"}, Name: "Nr", Type: &domainmodel.AutoNumberAttributeType{}},
		},
	}
	view := func(kind ast.DataTypeKind) *ast.CreateViewEntityStmt {
		return &ast.CreateViewEntityStmt{
			Name:       ast.QualifiedName{Module: "ServiceCore", Name: "V5"},
			Attributes: []ast.ViewAttribute{{Name: "Nr", Type: ast.DataType{Kind: kind}}},
			Query:      ast.OQLQuery{RawQuery: "select r.Nr as Nr from ServiceCore.Race as r"},
		}
	}
	for _, c := range []struct {
		name     string
		lang     langver.Version
		declared ast.DataTypeKind
		refused  bool
		warned   bool
	}{
		{"autonumber under mdl 1", langver.V1, ast.TypeAutoNumber, true, false},
		{"autonumber without the header", langver.V0, ast.TypeAutoNumber, false, true},
		// Control: Long builds at 0 errors, under either version.
		{"long under mdl 1", langver.V1, ast.TypeLong, false, false},
		{"long without the header", langver.V0, ast.TypeLong, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, race)
			ctx.LanguageVersion = c.lang
			sc := newScriptContext()
			err := validateWithContext(ctx, view(c.declared), sc)
			if (err != nil) != c.refused {
				t.Fatalf("refused=%v, want %v: %v", err != nil, c.refused, err)
			}
			if c.refused && !strings.Contains(err.Error(), "'Nr: Long'") {
				t.Errorf("the refusal should suggest Long: %v", err)
			}
			warned := false
			for _, w := range sc.warnings {
				if strings.Contains(w, viewAutoNumberRefused.Code) {
					warned = true
				}
			}
			if warned != c.warned {
				t.Errorf("warned=%v, want %v: %v", warned, c.warned, sc.warnings)
			}
		})
	}
}
