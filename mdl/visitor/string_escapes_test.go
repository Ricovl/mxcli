// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// defaultOf builds a one-attribute entity whose default is lit and returns the
// default's value and the MDL-V1-ESCAPE notes.
func defaultOf(t *testing.T, header, lit string) (any, []ast.LanguageNote, []error) {
	t.Helper()
	prog, errs := Build(header + "create persistent entity M.Note (Text: String(200) default " + lit + ");")
	if len(errs) > 0 {
		return nil, nil, errs
	}
	var notes []ast.LanguageNote
	for _, n := range prog.LanguageNotes {
		if n.Code == "MDL-V1-ESCAPE" {
			notes = append(notes, n)
		}
	}
	e := prog.Statements[len(prog.Statements)-1].(*ast.CreateEntityStmt)
	return e.Attributes[0].DefaultValue, notes, nil
}

// A doubled apostrophe is the only escape under mdl 1; under mdl 0 a backslash still escapes
// and each literal whose value would change warns MDL-V1-ESCAPE (#732).
func TestStringEscapesByVersion(t *testing.T) {
	for _, tc := range []struct {
		lit        string
		mdl0, mdl1 string
		warns      bool // under mdl 0
	}{
		{`'C:\temp'`, "C:\temp", `C:\temp`, true},
		{`'a\nb'`, "a\nb", `a\nb`, true},
		{`'one\\two'`, `one\two`, `one\\two`, true},
		// Control: no backslash, or one before a character mdl 0 keeps as
		// written, means the same under both, so nothing to warn about.
		{`'it''s'`, "it's", "it's", false},
		{`'C:\data'`, `C:\data`, `C:\data`, false},
	} {
		got, notes, errs := defaultOf(t, "", tc.lit)
		if len(errs) > 0 {
			t.Fatalf("mdl 0 %s: %v", tc.lit, errs)
		}
		if got != tc.mdl0 {
			t.Errorf("mdl 0 %s: got %q, want %q", tc.lit, got, tc.mdl0)
		}
		if (len(notes) == 1) != tc.warns || len(notes) > 1 {
			t.Errorf("mdl 0 %s: want warning=%v, got %v", tc.lit, tc.warns, notes)
		}
		got, notes, errs = defaultOf(t, "mdl 1;\n", tc.lit)
		if len(errs) > 0 {
			t.Fatalf("mdl 1 %s: %v", tc.lit, errs)
		}
		if got != tc.mdl1 {
			t.Errorf("mdl 1 %s: got %q, want %q", tc.lit, got, tc.mdl1)
		}
		if len(notes) != 0 {
			t.Errorf("mdl 1 %s: the new meaning must not warn: %v", tc.lit, notes)
		}
	}
}

// The escape rule decides where a literal ends. Under mdl 1 `'C:\'` is a
// complete literal and `\'` does not escape the quote; under mdl 0 it is the
// other way round.
func TestStringEscapesDecideWhereALiteralEnds(t *testing.T) {
	got, _, errs := defaultOf(t, "mdl 1;\n", `'C:\'`)
	if len(errs) > 0 || got != `C:\` {
		t.Fatalf("mdl 1: 'C:\\' must be the string C:\\, got %q %v", got, errs)
	}
	if _, _, errs := defaultOf(t, "", `'C:\'`); len(errs) == 0 {
		t.Fatal(`mdl 0: 'C:\' must stay unterminated`)
	}

	got, notes, errs := defaultOf(t, "", `'it\'s'`)
	if len(errs) > 0 || got != "it's" || len(notes) != 1 {
		t.Fatalf("mdl 0: 'it\\'s' must stay it's and warn, got %q %v %v", got, notes, errs)
	}
	_, _, errs = defaultOf(t, "mdl 1;\n", `'it\'s'`)
	if len(errs) == 0 {
		t.Fatal(`mdl 1: 'it\'s' must be refused — the literal ends at \'`)
	}
}

// The warning names the line of the literal, in source order among the other
// language notes.
func TestStringEscapeWarningLine(t *testing.T) {
	prog, errs := Build("show modules\n;\ncreate persistent entity M.Note (\n  Text: String(200) default 'a\\tb'\n);")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var lines []int
	for _, n := range prog.LanguageNotes {
		if n.Code == "MDL-V1-ESCAPE" {
			lines = append(lines, n.Line)
			if !strings.Contains(n.Message, `'a\tb'`) {
				t.Errorf("the warning must quote the literal: %s", n.Message)
			}
		}
	}
	if len(lines) != 1 || lines[0] != 4 {
		t.Fatalf("want one MDL-V1-ESCAPE on line 4, got %v", lines)
	}
}

// Every string literal the visitor reads goes through unquoteStringLit, which
// knows the escape rule the literal was lexed with. A call to unquoteString on
// a token's text reads an mdl 1 literal with mdl 0's escapes, silently: the
// script parses and the value is wrong. Only the two helpers may call it.
func TestStringLiteralsAreReadUnderTheirEscapeRule(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"visitor_helpers.go": true, "visitor_string_escapes.go": true}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || allowed[f] {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "unquoteString(") {
				t.Errorf("%s:%d: read a string literal with unquoteStringLit(node), not unquoteString(text)", f, i+1)
			}
		}
	}
}

// Under mdl 1 `\'` ends the literal and the parse error that follows is far
// from the cause; the hint names it.
func TestBackslashQuoteHintUnderMdl1(t *testing.T) {
	_, _, errs := defaultOf(t, "mdl 1;\n", `'it\'s'`)
	if len(errs) == 0 || !strings.Contains(errsText(errs), "Double the apostrophe") {
		t.Fatalf("want the doubled-apostrophe hint, got %v", errs)
	}
}
