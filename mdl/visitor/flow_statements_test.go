// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestFlowStatementsLocatesEachStatementAndItsOwnClause(t *testing.T) {
	src := "mdl 1;\ncreate or modify microflow M.F ()\nbegin\n" +
		"  $a = call microflow M.A(Msg = 'on error continue'); -- on error rollback\n" +
		"  $b = call microflow M.B()\n    on error rollback;\n" +
		"  $c = call java action M.C() on error begin\n" +
		"    $d = call microflow M.D() on error continue;\n" +
		"  end error;\n" +
		"  log info node 'T' 'é';\n" +
		"end;\n"
	stmts, ok := FlowStatements(src)
	if !ok {
		t.Fatal("the script did not parse")
	}
	runes := []rune(src)
	text := func(start, stop int) string { return string(runes[start:stop]) }

	type want struct {
		rule, text string
		onError    ast.ErrorHandlingType // "" = no clause of its own
		clause     string
	}
	wants := []want{
		{"callMicroflowStatement", "$a = call microflow M.A(Msg = 'on error continue')", "", ""},
		{"callMicroflowStatement", "$b = call microflow M.B()\n    on error rollback", ast.ErrorHandlingRollback, "on error rollback"},
		{"callJavaActionStatement", "", ast.ErrorHandlingCustom, ""},
		{"callMicroflowStatement", "$d = call microflow M.D() on error continue", ast.ErrorHandlingContinue, "on error continue"},
		{"logStatement", "log info node 'T' 'é'", "", ""},
	}
	if len(stmts) != len(wants) {
		t.Fatalf("got %d statements, want %d: %+v", len(stmts), len(wants), stmts)
	}
	for i, w := range wants {
		s := stmts[i]
		if s.Rule != w.rule {
			t.Errorf("statement %d: rule %q, want %q", i, s.Rule, w.rule)
		}
		if w.text != "" && text(s.Start, s.Stop) != w.text {
			t.Errorf("statement %d: text %q, want %q", i, text(s.Start, s.Stop), w.text)
		}
		switch {
		case w.onError == "" && s.OnError != nil:
			t.Errorf("statement %d: found a clause %q that is not its own", i, text(s.OnError.Start, s.OnError.Stop))
		case w.onError != "" && s.OnError == nil:
			t.Errorf("statement %d: its %s clause was not found", i, w.onError)
		case w.onError != "" && s.OnError.Type != w.onError:
			t.Errorf("statement %d: clause type %s, want %s", i, s.OnError.Type, w.onError)
		case w.clause != "" && text(s.OnError.Start, s.OnError.Stop) != w.clause:
			t.Errorf("statement %d: clause text %q, want %q", i, text(s.OnError.Start, s.OnError.Stop), w.clause)
		}
	}
	// The custom handler's clause spans its whole body, nested call included.
	if c := stmts[2].OnError; c != nil {
		if got := text(c.Start, c.Stop); got[:14] != "on error begin" || got[len(got)-9:] != "end error" {
			t.Errorf("custom clause text %q", got)
		}
	}
}

func TestFlowStatementsRefusesAScriptThatDoesNotParse(t *testing.T) {
	if _, ok := FlowStatements("create microflow M.F () begin $a = call microflow M.A() on error rollback on error continue; end;"); ok {
		t.Error("a statement with two clauses parsed")
	}
}
