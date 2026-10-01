// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// Freeze decision 5 (ako/mxcli#714): every describe output starts with
// `mdl 1;`, so concatenated outputs repeat the header. The same header again
// is accepted; a header naming another version is refused.

func TestLanguageHeader_RepeatedIdenticalHeaderIsAccepted(t *testing.T) {
	src := "mdl 1;\ncreate entity M.A ( N: String(20) );\n\nmdl 1;\ncreate entity M.B ( N: String(20) );\n"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("concatenated describe output was refused: %v", errs)
	}
	if prog.LanguageVersion != langver.V1 || prog.LanguageHeaderLine != 1 {
		t.Fatalf("got %v from line %d, want mdl 1 from line 1", prog.LanguageVersion, prog.LanguageHeaderLine)
	}
	if len(prog.Statements) != 2 {
		t.Fatalf("got %d statements, want 2 (a header is not a statement)", len(prog.Statements))
	}
	// A repeated explicit `mdl 0;` is the same rule.
	if _, errs := Build("mdl 0;\nlist entities;\nmdl 0;\nlist modules;"); len(errs) > 0 {
		t.Fatalf("a repeated mdl 0 header was refused: %v", errs)
	}
}

// The repeated header is not a reset: the second statement is still read as
// mdl 1, so a backslash in it is an ordinary character.
func TestLanguageHeader_RepeatedHeaderKeepsTheLanguage(t *testing.T) {
	prog, errs := Build("mdl 1;\ncreate constant M.A type String default 'x';\nmdl 1;\n" +
		`create constant M.B type String default 'C:\temp';` + "\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	c, ok := prog.Statements[1].(*ast.CreateConstantStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[1])
	}
	if got := c.DefaultValue.(string); got != `C:\temp` {
		t.Fatalf("default = %q, want the backslash kept (mdl 1)", got)
	}
}

func TestLanguageHeader_ConflictingHeaderIsRefused(t *testing.T) {
	_, errs := Build("mdl 1;\nlist entities;\nmdl 0;\nlist modules;")
	if len(errs) == 0 {
		t.Fatal("`mdl 0;` after `mdl 1;` was accepted")
	}
	msg := errs[0].Error()
	for _, want := range []string{"line 3", "`mdl 0;` conflicts with `mdl 1;`", "one language version"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q: %s", want, msg)
		}
	}
}

// A headerless script is mdl 0: a later `mdl 1;` would not reach the
// statements above it, so it is refused as misplaced, not as a conflict.
func TestLanguageHeader_LateHeaderInAHeaderlessScriptIsRefused(t *testing.T) {
	_, errs := Build("list entities;\nmdl 1;\nlist modules;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "must be the first statement") {
		t.Fatalf("want the header-placement error, got %v", errs)
	}
	// Control: an unknown version later in the script is unknown, not misplaced.
	_, errs = Build("mdl 1;\nlist entities;\nmdl 7;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown MDL language version 7") {
		t.Fatalf("want the unknown-version error, got %v", errs)
	}
}

// BuildSession: input typed at the REPL or given with -c is read in the
// session's language when it states none (freeze decision 6), keeping every
// line and column its own.
func TestBuildSession_HeaderlessInputIsTheSessionLanguage(t *testing.T) {
	prog, errs := BuildSession(`create constant M.A type String default 'C:\temp';`, langver.V1)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if prog.LanguageVersion != langver.V1 {
		t.Fatalf("got %v, want the session's mdl 1", prog.LanguageVersion)
	}
	if prog.LanguageHeaderLine != 0 {
		t.Errorf("an implicit header was given line %d; it is on no line of the input", prog.LanguageHeaderLine)
	}
	if got := prog.Statements[0].(*ast.CreateConstantStmt).DefaultValue.(string); got != `C:\temp` {
		t.Errorf("default = %q, want the backslash kept (mdl 1)", got)
	}

	// Control: the same text as a script file is mdl 0, where `\t` is a tab.
	prog, errs = Build(`create constant M.A type String default 'C:\temp';`)
	if len(errs) > 0 || prog.LanguageVersion != langver.V0 {
		t.Fatalf("script: %v %v", prog.LanguageVersion, errs)
	}
	if got := prog.Statements[0].(*ast.CreateConstantStmt).DefaultValue.(string); got != "C:\temp" {
		t.Errorf("script default = %q, want the mdl 0 tab escape", got)
	}

	// A session switched to mdl 0 reads headerless input as mdl 0.
	if prog, _ := BuildSession("list entities;", langver.V0); prog.LanguageVersion != langver.V0 {
		t.Errorf("an mdl 0 session read input as %v", prog.LanguageVersion)
	}
}

// Positions are the input's own: the implicit header is tokens, not text.
func TestBuildSession_ErrorPositionsAreTheInputs(t *testing.T) {
	_, script := Build("list entities;\ncrate entity M.A ( N: String(20) );")
	_, session := BuildSession("list entities;\ncrate entity M.A ( N: String(20) );", langver.V1)
	if len(script) == 0 || len(session) == 0 {
		t.Fatalf("a typo parsed: script %v, session %v", script, session)
	}
	if script[0].Error() != session[0].Error() {
		t.Errorf("the session reports another position or message:\n  script:  %v\n  session: %v", script[0], session[0])
	}
	_, script = Build("crate entity M.A ( N: String(20) );")
	_, session = BuildSession("crate entity M.A ( N: String(20) );", langver.V1)
	if len(script) == 0 || len(session) == 0 || script[0].Error() != session[0].Error() {
		t.Errorf("first-line typo:\n  script:  %v\n  session: %v", script, session)
	}
}

// A header the input states is honoured (the REPL switches on it), and a
// later conflicting one is refused as in a script.
func TestBuildSession_StatedHeaderWins(t *testing.T) {
	prog, errs := BuildSession("mdl 0;\nlist entities;", langver.V1)
	if len(errs) > 0 || prog.LanguageVersion != langver.V0 || prog.LanguageHeaderLine != 1 {
		t.Fatalf("got %v (line %d) %v, want the stated mdl 0", prog.LanguageVersion, prog.LanguageHeaderLine, errs)
	}
	if _, errs := BuildSession("list entities;\nmdl 0;", langver.V1); len(errs) == 0 ||
		!strings.Contains(errs[0].Error(), "the session's language") {
		t.Fatalf("a header switching the language part-way through one input was accepted: %v", errs)
	}
	if _, errs := BuildSession("list entities;\nmdl 1;", langver.V1); len(errs) > 0 {
		t.Fatalf("restating the session's own language was refused: %v", errs)
	}
}

// R7: a session command is refused in an mdl 1 script, and accepted at the
// REPL and in -c, which start in mdl 1 since the freeze.
func TestBuildSession_AcceptsSessionCommands(t *testing.T) {
	const input = "connect local 'app.mpr';\nset format = json;\nstatus;\nshow catalog status;"
	if _, errs := Build("mdl 1;\n" + input); len(errs) == 0 {
		t.Fatal("control: an mdl 1 script accepted a session command")
	}
	prog, errs := BuildSession(input, langver.V1)
	if len(errs) > 0 {
		t.Fatalf("the session refused its own commands: %v", errs)
	}
	if len(prog.Statements) != 4 {
		t.Fatalf("got %d statements, want 4", len(prog.Statements))
	}
	for _, n := range prog.LanguageNotes {
		if n.Code == "MDL-V1-SESSION" {
			t.Errorf("a session command typed in the session warned: %+v", n)
		}
	}
}

// The end of the input terminates the last statement typed at the REPL or
// given with -c (`mxcli -c "list entities"`), as it always has; a statement
// before it still needs its `;`.
func TestBuildSession_LastStatementNeedsNoSemicolon(t *testing.T) {
	if _, errs := Build("mdl 1;\nlist entities"); len(errs) == 0 {
		t.Fatal("control: an mdl 1 script accepted a statement without `;`")
	}
	if _, errs := BuildSession("list entities", langver.V1); len(errs) > 0 {
		t.Fatalf("a one-liner without `;` was refused: %v", errs)
	}
	if _, errs := BuildSession("list entities\nlist modules;", langver.V1); len(errs) == 0 {
		t.Fatal("a statement before the last one was accepted without `;`")
	}
}
