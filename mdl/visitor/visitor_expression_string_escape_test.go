// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
)

// declaredExpression builds `declare $s String = <expr>` under header and
// returns the Mendix expression the builder stores for it: the source of an
// expression stored as written, or the rendering of its tree.
func declaredExpression(t *testing.T, header, expr string) (stored string, asWritten bool) {
	t.Helper()
	prog, errs := Build(header + "create microflow M.F () begin\n  declare $s String = " + expr + ";\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("%s%s does not parse: %v", header, expr, errs[0])
	}
	mf := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)
	v := mf.Body[0].(*ast.DeclareStmt).InitialValue
	if se, ok := v.(*ast.SourceExpr); ok {
		return se.Source, true
	}
	return mendixexpr.String(v), false
}

// A string literal in a microflow expression stores its value, spelled as
// Studio Pro spells it — in an expression rendered from its tree and in one
// stored as written alike (ako/mxcli#810, #820).
func TestExpressionStringLiteralStoresItsValue(t *testing.T) {
	for _, c := range []struct {
		name, header, expr, stored string
		asWritten                  bool
	}{
		// mdl 0: a backslash before n, r, t, \ or ' escapes.
		{"mdl 0 backslash, rendered", "", `'C:\\temp'`, `'C:\temp'`, false},
		{"mdl 0 backslash, as written", "", "'C:\\\\temp'\n + 'x'", "'C:\\temp'\n + 'x'", true},
		{"mdl 0 escaped apostrophe, rendered", "", `'it\'s' + 'x'`, `'it''s' + 'x'`, false},
		// The scanner used to end the literal at `\'`, so the unspaced `+`
		// was not seen and the expression was rendered instead (#820).
		{"mdl 0 escaped apostrophe, unspaced", "", `'it\'s'+'x'`, `'it''s'+'x'`, true},
		{"mdl 0 escaped apostrophe, as written", "", "'it\\'s'\n + 'x'", "'it''s'\n + 'x'", true},
		{"mdl 0 escaped line break, as written", "", "'a\\nb'\n + 'x'", "'a\nb'\n + 'x'", true},
		{"mdl 0 regex, as written", "", "'^\\d+$'\n + 'x'", "'^\\d+$'\n + 'x'", true},
		{"mdl 0 trailing backslash, unspaced", "", `'C:\\'+'x'`, `'C:\'+'x'`, true},
		// mdl 1: a backslash is itself; `''` is the only escape.
		{"mdl 1 backslash, rendered", "mdl 1;\n", `'C:\temp\new'`, `'C:\temp\new'`, false},
		{"mdl 1 doubled backslash, rendered", "mdl 1;\n", `'C:\\temp'`, `'C:\\temp'`, false},
		{"mdl 1 trailing backslash, unspaced", "mdl 1;\n", `'C:\'+'x'`, `'C:\'+'x'`, true},
		{"mdl 1 backslash n, as written", "mdl 1;\n", "'a\\nb'\n + 'x'", "'a\\nb'\n + 'x'", true},
	} {
		got, asWritten := declaredExpression(t, c.header, c.expr)
		if got != c.stored || asWritten != c.asWritten {
			t.Errorf("%s: %s stores %q (as written: %v), want %q (as written: %v)",
				c.name, c.expr, got, asWritten, c.stored, c.asWritten)
		}
	}
}

func TestStringLiteralEnd(t *testing.T) {
	for _, c := range []struct {
		s      string
		strict bool
		want   int
	}{
		{`'ab' + 'c'`, false, 4},
		{`'it''s' + 'c'`, false, 7},
		{`'it\'s' + 'c'`, false, 7},
		{`'it\'s' + 'c'`, true, 5},
		{`'C:\\'+'x'`, false, 6},
		{`'C:\'+'x'`, true, 5},
		{`'open`, false, 5},
	} {
		if got := stringLiteralEnd(c.s, 0, c.strict); got != c.want {
			t.Errorf("stringLiteralEnd(%q, strict=%v) = %d, want %d", c.s, c.strict, got, c.want)
		}
	}
}

// A retrieve's XPath constraint stored as written — one navigating an
// association, one bracketed predicate, several — holds each string as its
// value, as the rendered constraint does through QuoteLiteral; under mdl 1 the
// source is the value already.
func TestRetrieveXPathStoredAsWrittenStoresItsValue(t *testing.T) {
	for _, c := range []struct {
		name, header, where, stored string
	}{
		{"mdl 0 association path", "", `M.E_F/M.F/Name = 'C:\\x'`, `M.E_F/M.F/Name = 'C:\x'`},
		{"mdl 0 escaped apostrophe", "", `M.E_F/M.F/Name = 'it\'s'`, `M.E_F/M.F/Name = 'it''s'`},
		{"mdl 0 one predicate", "", `[Name = 'C:\\']`, `Name = 'C:\'`},
		{"mdl 0 predicates", "", `[Name = 'a\\b'][Name != 'c']`, `[Name = 'a\b'][Name != 'c']`},
		{"mdl 1 association path", "mdl 1;\n", `M.E_F/M.F/Name = 'C:\x'`, `M.E_F/M.F/Name = 'C:\x'`},
		{"mdl 1 predicates", "mdl 1;\n", `[Name = 'C:\'][Name != 'c']`, `[Name = 'C:\'][Name != 'c']`},
	} {
		prog, errs := Build(c.header + "create microflow M.F () begin\n  retrieve $r from M.E where " + c.where + ";\nend;\n")
		if len(errs) > 0 {
			t.Fatalf("%s: does not parse: %v", c.name, errs[0])
		}
		mf := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)
		se, ok := mf.Body[0].(*ast.RetrieveStmt).Where.(*ast.SourceExpr)
		if !ok {
			t.Errorf("%s: %s is not stored as written", c.name, c.where)
			continue
		}
		if se.Source != c.stored {
			t.Errorf("%s: %s stores %q, want %q", c.name, c.where, se.Source, c.stored)
		}
	}
}
