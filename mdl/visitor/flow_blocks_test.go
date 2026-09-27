// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R2 (ako/mxcli#754): inside a microflow, flow is `begin … end <keyword>`.
// An error handler is `on error [without rollback] begin … end error;`, and the
// brace form is its deprecated spelling (MDL-DEPR540). `while` takes `begin` and
// `end while` like `loop`: both are required under mdl 1 (MDL-V1-WHILE), and a
// headerless script keeps parsing without them and warns.

func mfBodyOf(t *testing.T, src string) ([]ast.MicroflowStatement, *ast.Program) {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q: unexpected errors: %v", src, errs)
	}
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateMicroflowStmt); ok {
			return m.Body, prog
		}
	}
	t.Fatalf("%q: no microflow", src)
	return nil, nil
}

func errorHandlerMicroflow(clause string) string {
	return "create microflow M.F ($O: M.E) begin\n  commit $O " + clause + ";\nend;"
}

func countDeprecations(prog *ast.Program, code string) int {
	n := 0
	for _, d := range prog.Deprecations {
		if d.Code == code {
			n++
		}
	}
	return n
}

func TestOnErrorBeginEndError_BuildsTheHandler(t *testing.T) {
	for _, c := range []struct {
		clause string
		want   ast.ErrorHandlingType
	}{
		{"on error begin\n    log warning 'x';\n  end error", ast.ErrorHandlingCustom},
		{"on error without rollback begin\n    log warning 'x';\n  end error", ast.ErrorHandlingCustomWithoutRollback},
		{"ON ERROR WITHOUT ROLLBACK BEGIN LOG WARNING 'x'; END ERROR", ast.ErrorHandlingCustomWithoutRollback},
	} {
		for _, header := range []string{"", "mdl 1;\n"} {
			body, prog := mfBodyOf(t, header+errorHandlerMicroflow(c.clause))
			commit, ok := body[0].(*ast.MfCommitStmt)
			if !ok {
				t.Fatalf("%q: body[0] = %T", c.clause, body[0])
			}
			eh := commit.ErrorHandling
			if eh == nil || eh.Type != c.want || len(eh.Body) != 1 {
				t.Errorf("header %q, %q: handler = %+v; want %v with one statement", header, c.clause, eh, c.want)
			}
			if len(prog.Deprecations) != 0 || len(prog.LanguageNotes) != 0 {
				t.Errorf("header %q, %q: the canonical form warned: %+v %+v", header, c.clause, prog.Deprecations, prog.LanguageNotes)
			}
		}
	}
}

func TestOnErrorBeginEndError_EmptyHandler(t *testing.T) {
	body, _ := mfBodyOf(t, errorHandlerMicroflow("on error without rollback begin end error"))
	eh := body[0].(*ast.MfCommitStmt).ErrorHandling
	if eh == nil || eh.Type != ast.ErrorHandlingCustomWithoutRollback || len(eh.Body) != 0 {
		t.Errorf("empty handler = %+v", eh)
	}
}

// The brace form is a respelling: it keeps parsing under both versions, warns
// MDL-DEPR540 once per handler, and builds what `begin … end error` builds.
func TestOnErrorBraces_IsADeprecatedAlias(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		for _, pair := range [][2]string{
			{"on error { log warning 'x'; }", "on error begin log warning 'x'; end error"},
			{"on error without rollback {\n    log warning 'x';\n  }", "on error without rollback begin\n    log warning 'x';\n  end error"},
			{"on error without rollback { }", "on error without rollback begin end error"},
		} {
			oldBody, oldProg := mfBodyOf(t, header+errorHandlerMicroflow(pair[0]))
			newBody, _ := mfBodyOf(t, header+errorHandlerMicroflow(pair[1]))
			if !reflect.DeepEqual(oldBody, newBody) {
				t.Errorf("header %q: %q and %q build different statements", header, pair[0], pair[1])
			}
			if n := countDeprecations(oldProg, deprecation.OnErrorBraces); n != 1 {
				t.Errorf("header %q, %q: %d %s records, want 1", header, pair[0], n, deprecation.OnErrorBraces)
			}
		}
	}
}

// Nested handlers are one use each, and each carries its own rewrite.
func TestOnErrorBraces_NestedHandlersEachRecorded(t *testing.T) {
	src := errorHandlerMicroflow("on error {\n    commit $O on error { log info 'inner'; };\n  }")
	_, prog := mfBodyOf(t, src)
	if n := countDeprecations(prog, deprecation.OnErrorBraces); n != 2 {
		t.Fatalf("got %d %s records, want 2", n, deprecation.OnErrorBraces)
	}
	for _, d := range prog.Deprecations {
		if d.Fix == nil || len(d.Fix.Edits) != 2 {
			t.Errorf("line %d: want a two-edit rewrite ({ -> begin, } -> end error), got %+v (%s)", d.Line, d.Fix, d.NoFix)
		}
	}
}

func whileMicroflow(loop string) string {
	return "create microflow M.F () begin\n  declare $n Integer = 0;\n" + loop + "\nend;"
}

func whileNotes(prog *ast.Program) []ast.LanguageNote {
	var out []ast.LanguageNote
	for _, n := range prog.LanguageNotes {
		if n.Code == "MDL-V1-WHILE" {
			out = append(out, n)
		}
	}
	return out
}

var whileForms = []string{
	"  while $n < 3\n    set $n = $n + 1;\n  end while;", // no begin
	"  while $n < 3 begin\n    set $n = $n + 1;\n  end;", // no `while` after end
	"  while $n < 3\n    set $n = $n + 1;\n  end;",       // neither
	"  WHILE $n < 3\n    SET $n = $n + 1;\n  END;",       // upper case
}

func TestWhileBeginEndWhile_CanonicalUnderBothVersions(t *testing.T) {
	canon := "  while $n < 3 begin\n    set $n = $n + 1;\n  end while;"
	for _, header := range []string{"", "mdl 1;\n"} {
		body, prog := mfBodyOf(t, header+whileMicroflow(canon))
		if _, ok := body[1].(*ast.WhileStmt); !ok {
			t.Fatalf("body[1] = %T", body[1])
		}
		if n := whileNotes(prog); len(n) != 0 {
			t.Errorf("header %q: the canonical form warned: %+v", header, n)
		}
	}
}

func TestWhileWithoutBeginOrEndWhile_Mdl0WarnsAndBuildsTheSameLoop(t *testing.T) {
	canonBody, _ := mfBodyOf(t, whileMicroflow("  while $n < 3 begin\n    set $n = $n + 1;\n  end while;"))
	for _, form := range whileForms {
		body, prog := mfBodyOf(t, whileMicroflow(form))
		if !reflect.DeepEqual(body, canonBody) {
			t.Errorf("%q builds a different loop from the canonical form", form)
		}
		notes := whileNotes(prog)
		if len(notes) != 1 {
			t.Fatalf("%q: got %d MDL-V1-WHILE notes, want 1", form, len(notes))
		}
		if notes[0].Fix == nil || len(notes[0].Fix.Edits) == 0 {
			t.Errorf("%q: the note carries no rewrite (%s)", form, notes[0].NoFix)
		}
		if !strings.Contains(notes[0].Message, "end while") {
			t.Errorf("%q: the warning should name the required form: %s", form, notes[0].Message)
		}
	}
}

func TestWhileWithoutBeginOrEndWhile_Mdl1IsAnError(t *testing.T) {
	for _, form := range whileForms {
		_, errs := Build("mdl 1;\n" + whileMicroflow(form))
		if len(errs) == 0 {
			t.Errorf("%q: accepted under mdl 1", form)
			continue
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "end while") || !strings.Contains(msg, "begin") {
			t.Errorf("%q: the error should name `begin … end while`: %s", form, msg)
		}
	}
}
