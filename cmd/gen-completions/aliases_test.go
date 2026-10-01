// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

func TestAliasOnlyTokens(t *testing.T) {
	src := `parser grammar P;
options { tokenVocab = L; }
@parser::members { func f() { _ = 'x' } }
// SHOW_PAGE is the old spelling.
action
    : SHOW PAGE name
    | SHOW_PAGE /* @alias MDL-DEPR020 */ name   // show_page X
    | OPEN_LINK /* @alias MDL-DEPR020 */ STRING_LITERAL
    ;
showOrList: SHOW /* @alias MDL-DEPR002 */ | LIST_KW ;
other: OPEN_LINK SEMICOLON ;
keyword: SHOW_PAGE | DEFINE ;
`
	got := aliasOnlyTokens([]string{src})
	if !got["SHOW_PAGE"] {
		t.Error("SHOW_PAGE is used only as an alias (the keyword rule does not count)")
	}
	for _, tok := range []string{"SHOW", "OPEN_LINK", "PAGE", "DEFINE", "LIST_KW"} {
		if got[tok] {
			t.Errorf("%s has a canonical use, or none at all, but is reported alias-only", tok)
		}
	}
}

// Against the real grammar: the snake-case page actions are aliases only,
// while `show` (show page, show message) is canonical.
func TestAliasOnlyTokensInMDLGrammar(t *testing.T) {
	sources, err := parserGrammarSources("../../mdl/grammar/MDLLexer.g4")
	if err != nil {
		t.Fatal(err)
	}
	got := aliasOnlyTokens(sources)
	for _, tok := range []string{"SHOW_PAGE", "CLOSE_PAGE", "CREATE_OBJECT", "DELETE_OBJECT", "OPEN_LINK", "DELETE_BEHAVIOR", "DEFINE"} {
		if !got[tok] {
			t.Errorf("%s should be alias-only", tok)
		}
	}
	for _, tok := range []string{"SHOW", "LIST_KW", "CREATE", "SAVE_CHANGES", "ERROR_MESSAGE", "REFERENCE_SET", "MICROFLOW"} {
		if got[tok] {
			t.Errorf("%s has canonical uses but is reported alias-only", tok)
		}
	}
}
