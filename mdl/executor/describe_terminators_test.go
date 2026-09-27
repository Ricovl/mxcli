// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#744 — describe ends every statement with `;` and never prints a
// SQL*Plus `/` line (ADR-0010 R11/R12), so its output means the same under
// mdl 0 and mdl 1. testdata/pedapp covers the document types it contains
// (mdl/roundtrip TestPedAppDescribeIsValidMdl1); the tests here cover the rest.

// assertTerminated fails the test when a statement of describe output does not
// end with `;`, is followed by `/`, or when the output does not parse under an
// `mdl 1;` header.
func assertTerminated(t *testing.T, out string) {
	t.Helper()
	for _, p := range terminatorProblems(out) {
		t.Errorf("%s\n--- describe output ---\n%s", p, out)
	}
	if _, errs := visitor.Build("mdl 1;\n" + out); len(errs) > 0 {
		t.Errorf("describe output is not valid under mdl 1: %v\n--- describe output ---\n%s", errs, out)
	}
}

// terminatorProblems reports each top-level statement that does not end with
// `;` or is followed by `/`. Microflow, nanoflow and workflow rules end in
// `SEMICOLON? SLASH?` of their own, so the terminators are read off the
// statement's last tokens rather than the statement rule's own.
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

// The control: without it a check that never fires passes every test below.
func TestTerminatorProblems_Control(t *testing.T) {
	for script, want := range map[string]int{
		"create constant M.C type String default 'x';\n":                         0,
		"create constant M.C type String default 'x';\n/\n":                      1,
		"create constant M.C type String default 'x'\n":                          1,
		"create microflow M.F ()\nbegin\n  return;\nend;\n/\n":                   1,
		"create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n}\n":  1,
		"create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n};\n": 0,
	} {
		if got := terminatorProblems(script); len(got) != want {
			t.Errorf("%q: %d problems %v, want %d", script, len(got), got, want)
		}
	}
}

// A published OData service ends at its property list, its authentication
// clause or its entity block, whichever comes last; each must carry the `;`.
func TestDescribePublishedODataService_Terminated(t *testing.T) {
	for name, svc := range map[string]*model.PublishedODataService{
		"properties only": {Name: "S", Path: "odata/s/v1", Version: "1.0.0", ODataVersion: "OData4"},
		"authentication":  {Name: "S", Path: "odata/s/v1", ODataVersion: "OData4", AuthenticationTypes: []string{"Basic"}},
		"block": {Name: "S", Path: "odata/s/v1", ODataVersion: "OData4", AuthenticationTypes: []string{"Basic"},
			Microflows: []*model.PublishedMicroflow{{Microflow: "Shop.Act", ExposedName: "Act"}}},
		"grant": {Name: "S", Path: "odata/s/v1", ODataVersion: "OData4", AllowedModuleRoles: []string{"Shop.User"}},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			assertNoError(t, outputPublishedODataServiceMDL(&ExecContext{Output: &out}, svc, "Shop", ""))
			reparse(t, out.String())
			assertTerminated(t, out.String())
		})
	}
}

func TestDescribeExternalEntity_Terminated(t *testing.T) {
	attr := &domainmodel.Attribute{Name: "Code", Type: &domainmodel.StringAttributeType{Length: 20}}
	for name, attrs := range map[string][]*domainmodel.Attribute{
		"no attributes": nil,
		"attributes":    {attr},
	} {
		t.Run(name, func(t *testing.T) {
			e := &domainmodel.Entity{Name: "Order", Source: "Rest$ODataRemoteEntitySource", RemoteServiceName: "Shop.Remote", RemoteEntitySet: "Orders", Attributes: attrs}
			var out bytes.Buffer
			assertNoError(t, outputExternalEntityMDL(&ExecContext{Output: &out}, e, "Shop"))
			reparse(t, out.String())
			assertTerminated(t, out.String())
		})
	}
}
