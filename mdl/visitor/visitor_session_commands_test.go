// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// R7 (ako/mxcli#755): `helpStatement` was `IDENTIFIER helpTopicWord*`, the
// grammar's catch-all, so a misspelt statement keyword was a HELP statement.
// `craete module Foo;` parsed cleanly into nothing at all — the visitor dropped
// a help statement whose word was not help/exit/quit — and `craete entity
// M.E (…)` reported its error at the `(`, far from the typo. Under mdl 1 the
// word is an error, reported where it is.
func TestMisspeltStatementKeywordIsAnErrorAtTheWord(t *testing.T) {
	for _, tc := range []struct {
		input string
		near  string // the suggestion the error must carry
	}{
		{"craete module Foo;", "create"},
		{"dropp entity M.E;", "drop"},
		{"craete persistent entity Shop.Note (Text: string(200));", "create"},
		{"descibe entity M.E;", "describe"},
	} {
		prog, errs := Build("mdl 1;\n" + tc.input)
		if len(errs) == 0 {
			t.Errorf("%q: parsed without error into %d statement(s); a misspelt keyword must be an error",
				tc.input, len(prog.Statements))
			continue
		}
		first := errs[0].Error()
		if !strings.HasPrefix(first, "line 2:0 ") {
			t.Errorf("%q: first error is not at the misspelt word (line 2:0): %s", tc.input, first)
		}
		word := strings.Fields(tc.input)[0]
		want := "unknown statement '" + word + "' — did you mean '" + tc.near + "'?"
		if !strings.Contains(first, want) {
			t.Errorf("%q: error does not say %q: %s", tc.input, want, first)
		}
	}
}

// ADR-0011: the rejection is new, so it applies only under mdl 1. Without the
// header a statement the catch-all accepted — it built nothing — still parses
// into nothing, and warns MDL-V1-UNKNOWN; one it did not accept is the error it
// always was. Scripts following a skill that taught `… default ” PRIVATE;`
// ran under mdl 0, the `PRIVATE;` being such a statement.
func TestMisspeltStatementKeywordWarnsUnderMdl0(t *testing.T) {
	for _, in := range []string{"craete module Foo;", "PRIVATE;", "descibe entity M.E;"} {
		prog, errs := Build("create module A;\n" + in + "\ncreate module B;")
		if len(errs) > 0 {
			t.Errorf("%q: an error under mdl 0: %v", in, errs)
			continue
		}
		if len(prog.Statements) != 2 {
			t.Errorf("%q: %d statements, want the two around it", in, len(prog.Statements))
		}
		var note *ast.LanguageNote
		for i := range prog.LanguageNotes {
			if prog.LanguageNotes[i].Code == "MDL-V1-UNKNOWN" {
				note = &prog.LanguageNotes[i]
			}
		}
		if note == nil || note.Line != 2 || !strings.Contains(note.Message, strings.Fields(strings.TrimSuffix(in, ";"))[0]) {
			t.Errorf("%q: want an MDL-V1-UNKNOWN warning on line 2 naming the word, got %+v", in, prog.LanguageNotes)
		}
	}
	// Control: a misspelt statement the catch-all never accepted was an error
	// under mdl 0 before, and stays one.
	if _, errs := Build("craete persistent entity Shop.Note (Text: string(200));"); len(errs) == 0 {
		t.Error("a misspelt create with a body parsed under mdl 0")
	}
}

// The words the rule exists for still parse, with and without a topic, in any
// letter case.
func TestSessionWordsStillParse(t *testing.T) {
	for _, in := range []string{"help;", "HELP;", "Help workflow;", "help workflow.user-task;", "exit;", "QUIT;"} {
		prog, errs := Build(in)
		if len(errs) > 0 || len(prog.Statements) != 1 {
			t.Errorf("%q: %d statement(s), errors %v", in, len(prog.Statements), errs)
		}
	}
}

// R7 (ako/mxcli#755): a session command needs a session — a connection, an
// output format, a running app — so it belongs at the REPL or on the command
// line, not in a checked-in script. Under mdl 1 a script that holds one is
// refused; under mdl 0 it runs as before and warns, once per command.
func TestSessionCommandInAScript(t *testing.T) {
	session := []string{
		"connect local 'app.mpr';",
		"disconnect;",
		"status;",
		"set format = json;",
		"check;",
		"build;",
		"lint;",
		"use all;",
		"introspect api;",
		"debug 'x';",
		"execute script 'other.mdl';",
		"execute runtime 'reload_model';",
		"help;",
		"help workflow;",
	}
	for _, stmt := range session {
		prog, errs := Build(stmt)
		if len(errs) > 0 {
			t.Errorf("mdl 0: %q: %v", stmt, errs)
			continue
		}
		if n := countNotes(prog.LanguageNotes, sessionCommandInScript.Code); n != 1 {
			t.Errorf("mdl 0: %q: %d %s note(s), want 1", stmt, n, sessionCommandInScript.Code)
		}
		_, errs = Build("mdl 1;\n" + stmt)
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "session command") {
			t.Errorf("mdl 1: %q: want one session-command error, got %v", stmt, errs)
		}
	}

	// Controls: model statements, `exit` (which ends a script) and `show lint
	// rules` (a listing) are not session commands and are not reported.
	for _, stmt := range []string{"create module M;", "exit;", "show lint rules;", "refresh catalog;"} {
		for _, src := range []string{stmt, "mdl 1;\n" + stmt} {
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Errorf("%q: %v", src, errs)
				continue
			}
			if n := countNotes(prog.LanguageNotes, sessionCommandInScript.Code); n != 0 {
				t.Errorf("%q: reported as a session command", src)
			}
		}
	}
}

func countNotes(notes []ast.LanguageNote, code string) int {
	n := 0
	for _, note := range notes {
		if note.Code == code {
			n++
		}
	}
	return n
}
