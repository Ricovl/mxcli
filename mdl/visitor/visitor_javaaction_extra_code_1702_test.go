// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1702 (mendixlabs/mxcli#1658): `create java action` had no clause
// for the .java file's EXTRA CODE section. `extra code $$ … $$` after the body
// states it; omitted, the stored section is kept.

func parseJavaAction(t *testing.T, src string) *ast.CreateJavaActionStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateJavaActionStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateJavaActionStmt", prog.Statements[0])
	}
	return stmt
}

func TestJavaAction_ExtraCodeClause(t *testing.T) {
	s := parseJavaAction(t, `create or modify java action ModJ.JA_Twice(N: Integer not null) returns Integer
as $$
import java.util.Objects;

return twice(N);
$$
extra code $$

    // indentation is kept as written
	private static long twice(long n) {
		return Objects.requireNonNull(n) * 2L;
	}
import java.util.List;

$$;`)
	if !s.ExtraCodeSet {
		t.Fatal("ExtraCodeSet = false, want true")
	}
	want := "    // indentation is kept as written\n\tprivate static long twice(long n) {\n" +
		"\t\treturn Objects.requireNonNull(n) * 2L;\n\t}"
	if s.ExtraCode != want {
		t.Errorf("ExtraCode = %q, want %q", s.ExtraCode, want)
	}
	if s.JavaCode != "return twice(N);" {
		t.Errorf("JavaCode = %q", s.JavaCode)
	}
	// An import line belongs to the file's import list wherever it is written.
	if want := []string{"import java.util.Objects;", "import java.util.List;"}; !reflect.DeepEqual(s.Imports, want) {
		t.Errorf("Imports = %q, want %q", s.Imports, want)
	}
}

func TestJavaAction_ExtraCodeOmittedOrEmpty(t *testing.T) {
	s := parseJavaAction(t, "create java action M.A() returns Boolean as $$ return true; $$;")
	if s.ExtraCodeSet || s.ExtraCode != "" {
		t.Errorf("omitted clause: ExtraCodeSet = %v, ExtraCode = %q — want unset, so the stored section is kept", s.ExtraCodeSet, s.ExtraCode)
	}
	s = parseJavaAction(t, "create java action M.A() returns Boolean as $$ return true; $$ extra code $$ $$;")
	if !s.ExtraCodeSet || s.ExtraCode != "" {
		t.Errorf("empty clause: ExtraCodeSet = %v, ExtraCode = %q — want set and empty, which clears the section", s.ExtraCodeSet, s.ExtraCode)
	}
}

// CODE and EXTRA are new keywords; both stay usable as names.
func TestJavaAction_ExtraAndCodeStayNames(t *testing.T) {
	src := `create persistent entity M.Product (Code: String(20), Extra: Integer);
create microflow M.F ($Product: M.Product) returns String
begin
  return $Product/Code;
end;`
	if _, errs := Build(src); len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
}
