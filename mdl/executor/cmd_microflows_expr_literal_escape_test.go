// SPDX-License-Identifier: Apache-2.0

// Regression tests for expression string literals: how a value is stored in a
// Mendix expression, and how describe spells the stored expression back.
//
// Mendix's expression engine has one escape — an apostrophe is doubled — and
// no backslash escapes: measured against a running 11.13 runtime, a microflow
// storing 'a\tb' put four bytes in the database (61 5c 74 62). So the stored
// literal holds every other character as itself: a control character, and a
// backslash wherever it is (ako/mxcli#810). Reading a stored expression back
// as MDL is describe's concern: under mdl 1 it is MDL as it stands; under
// mdl 0 a backslash in a string literal is written `\\`.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func TestQuoteExpressionLiteral_IsWhatStudioProStores(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`^\d+$`, `'^\d+$'`},
		{`\p{Lu}`, `'\p{Lu}'`},
		{"line1\nline2", "'line1\nline2'"},
		{"line1\r\nline2", "'line1\r\nline2'"},
		{"col\tcol", "'col\tcol'"},
		// A backslash before a letter the mdl 0 reader decodes is still a
		// backslash in the model; it used to be doubled (#810).
		{`C:\temp\new`, `'C:\temp\new'`},
		{`\n`, `'\n'`},
		{`\\`, `'\\'`},
		{`abc\`, `'abc\'`},
		{`it\'s`, `'it\''s'`},
		{"it's here", "'it''s here'"},
	} {
		if got := quoteExpressionLiteral(tc.in); got != tc.want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Whatever is stored reads back as the same value in the language describe
// writes: as it stands under mdl 1, and through describeExpr's spelling under
// mdl 0. Control: the stored form read under mdl 0 as it stands is another
// value wherever it holds a backslash escape, which is why describe cannot
// simply write it.
func TestStoredExpressionLiteral_ReadsBackInTheDescribeLanguage(t *testing.T) {
	mdl0, mdl1 := &ExecContext{}, &ExecContext{LanguageVersion: langver.V1}
	for _, raw := range []string{
		"multi\nline\twith 'quotes'",
		"col\tcol",
		"line1\r\nline2",
		`regex ^\d+$`,
		`literal \t backslash-t`,
		`C:\temp\new`,
		`trailing backslash \`,
		`it\'s`,
		`\\`,
		"it's here",
	} {
		stored := quoteExpressionLiteral(raw)
		if got := declaredString(t, "mdl 1;\n", describeExpr(mdl1, stored)); got != raw {
			t.Errorf("mdl 1: stored %q describes as %q, read back as %q", stored, describeExpr(mdl1, stored), got)
		}
		if got := declaredString(t, "", describeExpr(mdl0, stored)); got != raw {
			t.Errorf("mdl 0: stored %q describes as %q, read back as %q", stored, describeExpr(mdl0, stored), got)
		}
	}
	if got := declaredString(t, "", quoteExpressionLiteral(`C:\temp`)); got == `C:\temp` {
		t.Error("the stored form read under mdl 0 as it stands kept its value — the comparison cannot fail")
	}
}

func TestDescribeExpr_Mdl0SpellsBackslashesInStringsOnly(t *testing.T) {
	const stored = `replaceAll($s, '\n', '') + 'it''s C:\' + $p\q`
	if got, want := describeExpr(&ExecContext{}, stored), `replaceAll($s, '\\n', '') + 'it''s C:\\' + $p\q`; got != want {
		t.Errorf("mdl 0:\n got  %s\n want %s", got, want)
	}
	if got := describeExpr(&ExecContext{LanguageVersion: langver.V1}, stored); got != stored {
		t.Errorf("mdl 1 writes the stored expression as it is, got %s", got)
	}
}

// declaredString parses `declare $s String = <expr>` under header and returns
// the value of its string literal.
func declaredString(t *testing.T, header, expr string) string {
	t.Helper()
	src := header + "create microflow M.MF_RT()\nbegin\n  declare $s String = " + expr + ";\nend;\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", src, errs[0])
	}
	got, ok := firstDeclaredStringLiteral(prog)
	if !ok {
		t.Fatalf("could not read the literal back from %q", src)
	}
	return got
}

// firstDeclaredStringLiteral digs the value out of `declare $s String = '...'`.
func firstDeclaredStringLiteral(prog *ast.Program) (string, bool) {
	for _, st := range prog.Statements {
		mf, ok := st.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, body := range mf.Body {
			d, ok := body.(*ast.DeclareStmt)
			if !ok {
				continue
			}
			// A multi-line expression comes back wrapped in a SourceExpr, which
			// keeps the source text; the literal's value is in the tree.
			inner := d.InitialValue
			if se, ok := inner.(*ast.SourceExpr); ok {
				inner = se.Expression
			}
			lit, ok := inner.(*ast.LiteralExpr)
			if !ok || lit.Kind != ast.LiteralString {
				continue
			}
			s, ok := lit.Value.(string)
			return s, ok
		}
	}
	return "", false
}
