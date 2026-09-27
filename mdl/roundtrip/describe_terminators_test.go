// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestPedAppDescribeIsValidMdl1 holds describe to ADR-0010 R11/R12 (#744):
// every statement it prints ends with `;` and none is followed by a SQL*Plus
// `/` line, so the output of every document type means the same under mdl 0
// and mdl 1. It checks the terminators on the parse tree itself, so it does not
// depend on the parser enforcing them, and then parses the output again under
// an `mdl 1;` header, which must report nothing.
func TestPedAppDescribeIsValidMdl1(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	targets := describeTargets(h)
	if len(targets) < 100 {
		t.Fatalf("only %d describe targets — the enumeration is broken", len(targets))
	}
	checked := 0
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			out, err := h.describe(target)
			if err != nil || strings.TrimSpace(out) == "" {
				t.Skipf("describe %s: err=%v (judged by TestPedAppRoundTrip)", target, err)
			}
			prog, errs := visitor.Build(out)
			if len(errs) > 0 {
				if hasKnownLaw(target, lawParse) {
					t.Skipf("does not parse under mdl 0 either (allowlisted): %v", errs[0])
				}
				t.Fatalf("describe output does not parse: %v\n%s", errs[0], out)
			}
			checked++
			// describe never emits a deprecated spelling (proposal §6.1).
			for _, d := range prog.Deprecations {
				t.Errorf("describe output uses a deprecated spelling, %s at line %d\n--- describe output ---\n%s",
					d.Code, d.Line, out)
			}
			for _, problem := range terminatorProblems(out) {
				t.Errorf("%s\n--- describe output ---\n%s", problem, out)
			}
			if _, errs := visitor.Build("mdl 1;\n" + out); len(errs) > 0 {
				t.Errorf("describe output is not valid under mdl 1: %v\n--- describe output ---\n%s", errs, out)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no describe output was checked — the test proves nothing")
	}
}

// TestDescribeTerminatorCheck is the control for terminatorProblems: without
// it, a check that never fires would make TestPedAppDescribeIsValidMdl1 pass
// on the output #744 was filed against.
func TestDescribeTerminatorCheck(t *testing.T) {
	for script, want := range map[string]int{
		"create constant M.C type String default 'x';\n":                        0,
		"create constant M.C type String default 'x';\n/\n":                     1,
		"create constant M.C type String default 'x'\n/\n":                      2,
		"create constant M.C type String default 'x'\n":                         1,
		"create microflow M.F ()\nbegin\n  return;\nend;\n":                     0,
		"create microflow M.F ()\nbegin\n  return;\nend;\n/\n":                  1,
		"create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n}\n": 1,
	} {
		if got := terminatorProblems(script); len(got) != want {
			t.Errorf("%q: %d problems %v, want %d", script, len(got), got, want)
		}
	}
}

// describeTargets is every document the round trip enumerates, plus each
// module (alone and with all its objects), navigation and settings.
func describeTargets(h *harness) []string {
	var out []string
	modules := map[string]bool{}
	for _, d := range h.documents() {
		out = append(out, d.target())
		if mod, _, ok := strings.Cut(d.name, "."); ok && !strings.HasPrefix(d.name, "'") {
			modules[mod] = true
		}
	}
	var mods []string
	for m := range modules {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		out = append(out, "module "+m, "module "+m+" with all")
	}
	return append(out, "navigation", "settings")
}

func hasKnownLaw(key string, l law) bool {
	for _, k := range knownFailures[key].laws {
		if k == l {
			return true
		}
	}
	return false
}

// terminatorProblems reports each top-level statement of script that does not
// end with `;` or is followed by `/`. It reads the terminators off the
// statement's last tokens: microflow, nanoflow and workflow bodies end in
// `SEMICOLON? SLASH?` of their own, so the statement rule's may be empty.
func terminatorProblems(script string) []string {
	lexer := parser.NewMDLLexer(antlr.NewInputStream(script))
	lexer.RemoveErrorListeners()
	p := parser.NewMDLParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	var problems []string
	for _, stmt := range p.Program().AllStatement() {
		last := lastTokens(stmt, 2)
		if len(last) == 0 {
			continue
		}
		if last[0].GetTokenType() == parser.MDLParserSLASH {
			problems = append(problems, fmt.Sprintf("line %d: a `/` line follows the statement", last[0].GetLine()))
			last = last[1:]
		}
		if len(last) > 0 && last[0].GetTokenType() != parser.MDLParserSEMICOLON {
			problems = append(problems, fmt.Sprintf("line %d: the statement ending at %q has no `;`", last[0].GetLine(), last[0].GetText()))
		}
	}
	return problems
}

// lastTokens returns up to n of the tree's last tokens, the last first.
func lastTokens(tree antlr.Tree, n int) []antlr.Token {
	var out []antlr.Token
	var walk func(antlr.Tree)
	walk = func(t antlr.Tree) {
		if len(out) == n {
			return
		}
		if tn, ok := t.(antlr.TerminalNode); ok {
			if tok := tn.GetSymbol(); tok != nil && tok.GetTokenType() != antlr.TokenEOF {
				out = append(out, tok)
			}
			return
		}
		for i := t.GetChildCount() - 1; i >= 0 && len(out) < n; i-- {
			walk(t.GetChild(i))
		}
	}
	walk(tree)
	return out
}
