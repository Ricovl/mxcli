// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#836: since #750 an expression property takes the expression
// as-is, and the old quoted spelling was refused in every script. Under mdl 0
// it keeps its old meaning — the literal's content is the expression — and
// notes MDL-V1-QUOTEDEXPR; only mdl 1 reads it as the string it spells.

// The shape mxcli-demo-2 (15 widgets) and CapTrackV6 (22) use.
const demo2Legacy = `'if $currentObject/X then ''on'' else '''''`
const demo2Bare = `if $currentObject/X then 'on' else ''`

func dynamicClassPage(header, value string) string {
	return header + `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (dynamicclasses: ` + value + `) { }
  datagrid dg (datasource: database M.Thing) {
    column col1 (attribute: Name, caption: 'N', DynamicCellClass: ` + value + `)
  }
};`
}

// dynamicClassValues returns the container's DynamicClasses and the column's
// DynamicCellClass.
func dynamicClassValues(t *testing.T, prog *ast.Program) (any, any) {
	t.Helper()
	ws := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets
	return ws[0].Properties["dynamicclasses"], ws[1].Children[0].Properties["DynamicCellClass"]
}

func quotedExprNotes(prog *ast.Program) []ast.LanguageNote {
	var out []ast.LanguageNote
	for _, n := range prog.LanguageNotes {
		if n.Code == quotedExpressionText.Code {
			out = append(out, n)
		}
	}
	return out
}

func TestQuotedWidgetExpression_Mdl0KeepsTheOldMeaning(t *testing.T) {
	prog := mustBuild(t, dynamicClassPage("", demo2Legacy))
	cls, cell := dynamicClassValues(t, prog)
	if cls != demo2Bare || cell != demo2Bare {
		t.Errorf("mdl 0 stored %#v / %#v, want the expression %q (the old meaning)", cls, cell, demo2Bare)
	}
	notes := quotedExprNotes(prog)
	if len(notes) != 2 {
		t.Fatalf("notes = %+v, want two %s", prog.LanguageNotes, quotedExpressionText.Code)
	}
	for _, n := range notes {
		if n.Fix == nil || len(n.Fix.Edits) != 1 || n.Fix.Edits[0].Text != demo2Bare {
			t.Errorf("note %+v: want one edit writing %q", n, demo2Bare)
		}
	}

	// The rewrite is a respelling under mdl 0: the bare form builds the same.
	bare := mustBuild(t, dynamicClassPage("", demo2Bare))
	bcls, bcell := dynamicClassValues(t, bare)
	if !reflect.DeepEqual([]any{cls, cell}, []any{bcls, bcell}) {
		t.Errorf("quoted built %#v/%#v, bare %#v/%#v", cls, cell, bcls, bcell)
	}
	if n := quotedExprNotes(bare); len(n) != 0 {
		t.Errorf("the bare form noted %+v", n)
	}
}

func TestQuotedWidgetExpression_Mdl1IsTheString(t *testing.T) {
	prog := mustBuild(t, dynamicClassPage("mdl 1;\n", demo2Legacy))
	cls, cell := dynamicClassValues(t, prog)
	if cls != demo2Legacy || cell != demo2Legacy {
		t.Errorf("mdl 1 stored %#v / %#v, want the string literal as written (MDL-WIDGET33 refuses it)", cls, cell)
	}
	if n := quotedExprNotes(prog); len(n) != 0 {
		t.Errorf("mdl 1 noted %+v", n)
	}
}

// A class name has no old meaning to keep: `'is-featured'` stored the bare
// word `is-featured` as the expression, which Mendix rejects. It is the
// string under both versions, and not noted.
func TestQuotedWidgetExpression_ClassNameIsTheStringInBoth(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		prog := mustBuild(t, dynamicClassPage(header, `'btn is-featured'`))
		cls, _ := dynamicClassValues(t, prog)
		if cls != `'btn is-featured'` {
			t.Errorf("%q stored %#v", header, cls)
		}
		if n := quotedExprNotes(prog); len(n) != 0 {
			t.Errorf("%q noted %+v", header, n)
		}
	}
}

func TestQuotedWidgetExpression_AlterPageSet(t *testing.T) {
	for _, src := range []string{
		`alter page M.P { set DynamicClasses = ` + demo2Legacy + ` on c1 };`,
		`alter page M.P { set (DynamicClasses: ` + demo2Legacy + `) on c1 };`,
	} {
		prog := mustBuild(t, src)
		var got any
		for _, op := range prog.Statements[0].(*ast.AlterPageStmt).Operations {
			if set, ok := op.(*ast.SetPropertyOp); ok {
				got = set.Properties["DynamicClasses"]
			}
		}
		if got != demo2Bare {
			t.Errorf("%s stored %#v, want %q", src, got, demo2Bare)
		}
		if n := quotedExprNotes(prog); len(n) != 1 || n[0].Fix == nil {
			t.Errorf("%s noted %+v, want one with a rewrite", src, n)
		}
	}
}

// A quoted constant reference was the constant; the rewrite writes it bare.
func TestQuotedWidgetExpression_ConstantReference(t *testing.T) {
	prog := mustBuild(t, dynamicClassPage("", `'@M.CardClass'`))
	if cls, _ := dynamicClassValues(t, prog); cls != "@M.CardClass" {
		t.Errorf("stored %#v, want the constant reference", cls)
	}
}

func odataClient(header, user, pass, hdr string) string {
	return header + `create odata client M.Api (
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  UseAuthentication: Yes,
  HttpUsername: ` + user + `,
  HttpPassword: ` + pass + `
)
headers (
  'X-Key': ` + hdr + `
);`
}

func TestQuotedODataExpression_Mdl0KeepsTheOldMeaning(t *testing.T) {
	old := mustBuild(t, odataClient("", `'''admin'''`, `'@M.ApiPassword'`, `'@M.ApiKey'`))
	bare := mustBuild(t, odataClient("", `'admin'`, `@M.ApiPassword`, `@M.ApiKey`))
	o := old.Statements[0].(*ast.CreateODataClientStmt)
	b := bare.Statements[0].(*ast.CreateODataClientStmt)
	if o.HttpUsername != "'admin'" || !o.HttpUsernameIsLiteral {
		t.Errorf("HttpUsername = %q (literal %v), want the string 'admin'", o.HttpUsername, o.HttpUsernameIsLiteral)
	}
	if o.HttpPassword != "@M.ApiPassword" || o.HttpPasswordIsLiteral {
		t.Errorf("HttpPassword = %q (literal %v), want the constant", o.HttpPassword, o.HttpPasswordIsLiteral)
	}
	if !reflect.DeepEqual(
		[]any{o.HttpUsername, o.HttpUsernameIsLiteral, o.HttpPassword, o.HttpPasswordIsLiteral, o.Headers},
		[]any{b.HttpUsername, b.HttpUsernameIsLiteral, b.HttpPassword, b.HttpPasswordIsLiteral, b.Headers}) {
		t.Errorf("the quoted spelling built %+v\nthe bare one %+v", o, b)
	}
	if n := quotedExprNotes(old); len(n) != 3 {
		t.Errorf("notes = %+v, want three", old.LanguageNotes)
	}
	if n := quotedExprNotes(bare); len(n) != 0 {
		t.Errorf("the bare form noted %+v", n)
	}
	// A plain credential is the string in both versions, and not noted.
	plain := mustBuild(t, odataClient("", `'admin'`, `'s3cret'`, `'k'`))
	if n := quotedExprNotes(plain); len(n) != 0 {
		t.Errorf("a plain credential noted %+v", n)
	}
}

func TestQuotedODataExpression_Mdl1IsTheString(t *testing.T) {
	prog := mustBuild(t, odataClient("mdl 1;\n", `'''admin'''`, `'@M.ApiPassword'`, `'@M.ApiKey'`))
	s := prog.Statements[0].(*ast.CreateODataClientStmt)
	if s.HttpUsername != `'''admin'''` || s.HttpPassword != `'@M.ApiPassword'` {
		t.Errorf("mdl 1 stored %q / %q, want the literals as written (MDL-ODATA07 refuses them)", s.HttpUsername, s.HttpPassword)
	}
	if n := quotedExprNotes(prog); len(n) != 0 {
		t.Errorf("mdl 1 noted %+v", n)
	}
}

func TestQuotedODataExpression_Alter(t *testing.T) {
	prog := mustBuild(t, `alter consumed odata service M.Api set (HttpPassword: '@M.ApiPassword');`)
	s := prog.Statements[0].(*ast.AlterODataClientStmt)
	if got := s.Changes["HttpPassword"]; got != "@M.ApiPassword" {
		t.Errorf("alter stored %#v, want the constant", got)
	}
	if n := quotedExprNotes(prog); len(n) != 1 {
		t.Errorf("notes = %+v, want one", prog.LanguageNotes)
	}
}

// The old describe quoted every stored expression (formatExprValue), so a
// Studio Pro header value holding `'Bearer ' + @M.Token` came out doubled at
// the START only, not at both ends:
//
//	'''Bearer '' + @M.Token'
//
// Under mdl 0 that is still the expression, not the string of its text.
// Likewise an expression with no `$`, quote or `if` in DynamicClasses — a
// constant in a function call or a concatenation of constants.
func TestQuotedExpression_Mdl0KeepsCompoundOldSpellings(t *testing.T) {
	prog := mustBuild(t, odataClient("", `'''Basic '' + @M.Creds'`, `'@M.A + @M.B'`, `'''Bearer '' + @M.Token'`))
	s := prog.Statements[0].(*ast.CreateODataClientStmt)
	if s.HttpUsername != `'Basic ' + @M.Creds` || s.HttpUsernameIsLiteral {
		t.Errorf("HttpUsername = %q (literal %v), want the expression", s.HttpUsername, s.HttpUsernameIsLiteral)
	}
	if s.HttpPassword != `@M.A + @M.B` {
		t.Errorf("HttpPassword = %q, want the expression", s.HttpPassword)
	}
	if len(s.Headers) != 1 || s.Headers[0].Value != `'Bearer ' + @M.Token` || s.Headers[0].ValueIsLiteral {
		t.Errorf("header = %+v, want the expression", s.Headers)
	}
	for _, v := range []string{`'toLowerCase(@M.Theme)'`, `'@M.Base + @M.Extra'`} {
		p := mustBuild(t, dynamicClassPage("", v))
		if cls, _ := dynamicClassValues(t, p); cls != v[1:len(v)-1] {
			t.Errorf("dynamicclasses %s stored %#v, want the expression", v, cls)
		}
	}
	// Controls: an e-mail user name, a password with @ and parentheses, and
	// Tailwind-style class names stay strings.
	plain := mustBuild(t, odataClient("", `'user@example.com'`, `'p@ss(w0rd)'`, `'k'`))
	if n := quotedExprNotes(plain); len(n) != 0 {
		t.Errorf("a plain credential noted %+v", n)
	}
	for _, v := range []string{`'@container md:flex'`, `'bg-[url(/a.png)] min-h-[calc(100vh-1rem)]'`, `'bg-(--brand)'`} {
		p := mustBuild(t, dynamicClassPage("", v))
		if cls, _ := dynamicClassValues(t, p); cls != v {
			t.Errorf("class list %s stored %#v, want the string", v, cls)
		}
	}
}
