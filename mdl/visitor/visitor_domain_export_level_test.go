// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1498 (point 2 of the 2026-10-16 follow-up): `check` reports
// CE5400 when a Usable microflow in a Protected module exposes a Hidden entity
// or enumeration, and MDL had no way to make either Usable — `export level` on
// an entity, attribute, association or enumeration was a parse error
// ("extraneous input 'export' expecting the start of a statement").
//
// Each case parses the clause both ways: Hidden alone could not tell a visitor
// that reads the keyword from one that writes the default.
func TestDomainModelExportLevelClause(t *testing.T) {
	api, hidden := "API", "Hidden"
	for _, tc := range []struct {
		name string
		src  string
		want *string
		get  func(t *testing.T, s ast.Statement) *string
	}{
		{"entity api", `create persistent entity M.E (A: String(10)) export level api;`, &api, entityLevel},
		{"entity hidden", `create persistent entity M.E (A: String(10)) export level hidden;`, &hidden, entityLevel},
		{"entity without body", `create non-persistent entity M.E export level api;`, &api, entityLevel},
		{"entity with index", `create persistent entity M.E (A: String(10)) index (A) export level api;`, &api, entityLevel},
		{"entity absent", `create persistent entity M.E (A: String(10));`, nil, entityLevel},
		{"attribute api", `create persistent entity M.E (A: String(10) not null export level api);`, &api, attrLevel},
		{"attribute hidden", `create persistent entity M.E (A: String(10) export level hidden);`, &hidden, attrLevel},
		{"attribute absent", `create persistent entity M.E (A: String(10));`, nil, attrLevel},
		{"association api", `create association M.A_B from M.A to M.B type Reference export level api;`, &api, assocLevel},
		{"association hidden", `create association M.A_B from M.A to M.B export level hidden;`, &hidden, assocLevel},
		{"association absent", `create association M.A_B from M.A to M.B;`, nil, assocLevel},
		{"enumeration api", `create enumeration M.K (A 'A') export level api;`, &api, enumLevel},
		{"enumeration with folder", `create enumeration M.K (A 'A') folder 'X' export level hidden;`, &hidden, enumLevel},
		{"enumeration absent", `create enumeration M.K (A 'A');`, nil, enumLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			got := tc.get(t, prog.Statements[0])
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("ExportLevel = %q, want nil (an absent clause keeps the stored level)", *got)
			case tc.want != nil && got == nil:
				t.Errorf("ExportLevel = nil, want %q", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("ExportLevel = %q, want %q", *got, *tc.want)
			}
		})
	}
}

func entityLevel(t *testing.T, s ast.Statement) *string {
	t.Helper()
	e, ok := s.(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("statement is %T", s)
	}
	return e.ExportLevel
}

func attrLevel(t *testing.T, s ast.Statement) *string {
	t.Helper()
	e, ok := s.(*ast.CreateEntityStmt)
	if !ok || len(e.Attributes) != 1 {
		t.Fatalf("statement is %T", s)
	}
	return e.Attributes[0].ExportLevel
}

func assocLevel(t *testing.T, s ast.Statement) *string {
	t.Helper()
	a, ok := s.(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("statement is %T", s)
	}
	return a.ExportLevel
}

func enumLevel(t *testing.T, s ast.Statement) *string {
	t.Helper()
	e, ok := s.(*ast.CreateEnumerationStmt)
	if !ok {
		t.Fatalf("statement is %T", s)
	}
	return e.ExportLevel
}

// The ALTER spellings, so a stored element changes level without restating it.
func TestAlterDomainModelExportLevel(t *testing.T) {
	t.Run("alter entity", func(t *testing.T) {
		for src, want := range map[string]string{
			`alter entity M.E set export level api;`:    "API",
			`alter entity M.E set export level hidden;`: "Hidden",
		} {
			s := onlyStatement[*ast.AlterEntityStmt](t, src)
			if s.Operation != ast.AlterEntitySetExportLevel || s.ExportLevel != want {
				t.Errorf("%s: op %v level %q, want SetExportLevel %q", src, s.Operation, s.ExportLevel, want)
			}
		}
	})
	t.Run("modify attribute", func(t *testing.T) {
		s := onlyStatement[*ast.AlterEntityStmt](t, `alter entity M.E modify attribute A: String(10) export level api;`)
		if s.Operation != ast.AlterEntityModifyAttribute || s.ModifyExportLevel == nil || *s.ModifyExportLevel != "API" {
			t.Errorf("op %v ModifyExportLevel %v, want ModifyAttribute API", s.Operation, s.ModifyExportLevel)
		}
		s = onlyStatement[*ast.AlterEntityStmt](t, `alter entity M.E modify attribute A: String(10);`)
		if s.ModifyExportLevel != nil {
			t.Errorf("ModifyExportLevel = %q without the clause, want nil", *s.ModifyExportLevel)
		}
	})
	t.Run("add attribute", func(t *testing.T) {
		s := onlyStatement[*ast.AlterEntityStmt](t, `alter entity M.E add attribute A: String(10) export level api;`)
		if s.Attribute == nil || s.Attribute.ExportLevel == nil || *s.Attribute.ExportLevel != "API" {
			t.Errorf("added attribute ExportLevel = %v, want API", s.Attribute)
		}
	})
	t.Run("alter association", func(t *testing.T) {
		for src, want := range map[string]string{
			`alter association M.A_B set export level api;`:    "API",
			`alter association M.A_B set export level hidden;`: "Hidden",
		} {
			s := onlyStatement[*ast.AlterAssociationStmt](t, src)
			if s.Operation != ast.AlterAssociationSetExportLevel || s.ExportLevel != want {
				t.Errorf("%s: op %v level %q, want SetExportLevel %q", src, s.Operation, s.ExportLevel, want)
			}
		}
	})
	t.Run("alter enumeration", func(t *testing.T) {
		for src, want := range map[string]string{
			`alter enumeration M.K set export level api;`:    "API",
			`alter enumeration M.K set export level hidden;`: "Hidden",
		} {
			s := onlyStatement[*ast.AlterEnumerationStmt](t, src)
			if s.Operation != ast.AlterEnumSetExportLevel || s.ExportLevel != want {
				t.Errorf("%s: op %v level %q, want SetExportLevel %q", src, s.Operation, s.ExportLevel, want)
			}
		}
	})
}

func onlyStatement[T ast.Statement](t *testing.T, src string) T {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("%s: parse errors: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("%s: %d statements", src, len(prog.Statements))
	}
	s, ok := prog.Statements[0].(T)
	if !ok {
		t.Fatalf("%s: statement is %T", src, prog.Statements[0])
	}
	return s
}
