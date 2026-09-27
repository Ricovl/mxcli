// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

func notesWithCode(t *testing.T, src, code string) []string {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("mdl 0 must keep accepting it, got errors: %v\n%s", errs, src)
	}
	var out []string
	for _, n := range prog.LanguageNotes {
		if n.Code == code {
			out = append(out, n.Message)
		}
	}
	return out
}

// Under mdl 0 a missing `;` keeps parsing and warns (MDL-V1-SEMI); under mdl 1
// it is an error. A terminated script is the control: no note, no error.
func TestSemicolonRequiredUnderMdl1(t *testing.T) {
	src := "show entities\nshow modules;"
	if got := notesWithCode(t, src, "MDL-V1-SEMI"); len(got) != 1 {
		t.Fatalf("mdl 0: want one MDL-V1-SEMI warning, for line 1, got %v", got)
	}
	_, errs := Build("mdl 1;\n" + src)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "no terminating `;`") {
		t.Fatalf("mdl 1: want one missing-`;` error, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "line 2:") {
		t.Errorf("mdl 1: the error must name the statement's line, got %v", errs[0])
	}

	// Control: every statement terminated.
	if got := notesWithCode(t, "show entities;\nshow modules;", "MDL-V1-SEMI"); len(got) != 0 {
		t.Fatalf("mdl 0: a terminated script warned: %v", got)
	}
	if _, errs := Build("mdl 1;\nshow entities;\nshow modules;"); len(errs) != 0 {
		t.Fatalf("mdl 1: a terminated script was refused: %v", errs)
	}
}

// A statement rule that consumes its own `;` (a java action body) is
// terminated; it must not be reported as missing one.
func TestSemicolonConsumedByTheStatementRule(t *testing.T) {
	src := "mdl 1;\ncreate java action M.J () returns Boolean as $$return true;$$;\nshow modules;"
	if _, errs := Build(src); len(errs) != 0 {
		t.Fatalf("a statement ending in its own `;` was refused: %v", errs)
	}
}

// The SQL*Plus `/` terminator: accepted with a warning under mdl 0, an error
// under mdl 1.
func TestSlashTerminatorRejectedUnderMdl1(t *testing.T) {
	src := "create persistent entity M.Note (Text: String(200))\n/\nshow modules;"
	if got := notesWithCode(t, src, "MDL-V1-SLASH"); len(got) != 1 {
		t.Fatalf("mdl 0: want one MDL-V1-SLASH warning, got %v", got)
	}
	_, errs := Build("mdl 1;\n" + src)
	var slash int
	for _, e := range errs {
		if strings.Contains(e.Error(), "`/` is not a statement terminator") {
			slash++
		}
	}
	if slash != 1 {
		t.Fatalf("mdl 1: want one `/` error, got %v", errs)
	}
}

// The microflow, nanoflow and workflow rules end in `SEMICOLON? SLASH?`
// themselves, so their terminators are the inner rule's: `end;` followed by
// `/` is the `/` form, and `end;` alone is terminated.
func TestTerminatorsOfRulesThatEndInTheirOwn(t *testing.T) {
	mf := "create microflow M.F () begin end;"
	if got := notesWithCode(t, mf+"\n/\nshow modules;", "MDL-V1-SLASH"); len(got) != 1 {
		t.Fatalf("mdl 0: want one MDL-V1-SLASH for a microflow's `/`, got %v", got)
	}
	_, errs := Build("mdl 1;\n" + mf + "\n/\nshow modules;")
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "`/` is not a statement terminator") {
		t.Fatalf("mdl 1: want only the `/` error for a microflow's `/`, got %v", errs)
	}
	if _, errs := Build("mdl 1;\n" + mf + "\nshow modules;"); len(errs) != 0 {
		t.Fatalf("mdl 1: a microflow ending `end;` was refused: %v", errs)
	}
	_, errs = Build("mdl 1;\ncreate microflow M.F () begin end\nshow modules;")
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `ending at "end"`) {
		t.Fatalf("mdl 1: want a missing-`;` error naming `end`, got %v", errs)
	}
}
