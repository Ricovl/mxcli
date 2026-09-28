// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// R7 (ako/mxcli#755): `helpStatement` was `IDENTIFIER helpTopicWord*`, the
// grammar's catch-all, so a misspelt statement keyword was a HELP statement.
// `craete module Foo;` parsed cleanly into nothing at all — the visitor dropped
// a help statement whose word was not help/exit/quit — and `craete entity
// M.E (…)` reported its error at the `(`, far from the typo.
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
		prog, errs := Build(tc.input)
		if len(errs) == 0 {
			t.Errorf("%q: parsed without error into %d statement(s); a misspelt keyword must be an error",
				tc.input, len(prog.Statements))
			continue
		}
		first := errs[0].Error()
		if !strings.HasPrefix(first, "line 1:0 ") {
			t.Errorf("%q: first error is not at the misspelt word (line 1:0): %s", tc.input, first)
		}
		word := strings.Fields(tc.input)[0]
		want := "unknown statement '" + word + "' — did you mean '" + tc.near + "'?"
		if !strings.Contains(first, want) {
			t.Errorf("%q: error does not say %q: %s", tc.input, want, first)
		}
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
