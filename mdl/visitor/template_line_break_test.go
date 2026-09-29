// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

const templateFlow = "create microflow M.F ($o: M.E, $n: Integer) begin\n  "

// templateMessage builds the one statement of a microflow and returns the
// message expression of its log / show message / validation feedback, and the
// MDL-V1-TEMPLATE notes the script recorded.
func templateMessage(t *testing.T, header, stmt string) (ast.Expression, []ast.LanguageNote) {
	t.Helper()
	prog, errs := Build(header + templateFlow + stmt + "\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("%q: %v", stmt, errs[0])
	}
	var notes []ast.LanguageNote
	for _, n := range prog.LanguageNotes {
		if n.Code == templateLineBreak.Code {
			notes = append(notes, n)
		}
	}
	mf := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)
	switch s := mf.Body[0].(type) {
	case *ast.LogStmt:
		return s.Message, notes
	case *ast.ShowMessageStmt:
		return s.Message, notes
	case *ast.ValidationFeedbackStmt:
		return s.Message, notes
	}
	t.Fatalf("%q: not a template statement: %T", stmt, mf.Body[0])
	return nil, nil
}

func isTemplateText(e ast.Expression, want string) bool {
	lit, ok := e.(*ast.LiteralExpr)
	return ok && lit.Kind == ast.LiteralString && lit.Value == want
}

// Under mdl 1 a message template that is one string literal is the template
// text, whether or not the literal spans lines: a line break is written into
// the literal (#746). Under mdl 0 a literal that spans lines stays what it
// was, an expression stored as written (a `{1}` parameter), and warns.
func TestTemplateLineBreak_LoneLiteral(t *testing.T) {
	for _, stmt := range []string{
		"log info 'line 1\nline 2';",
		"log warning node 'N' 'line 1\nline 2';",
		"show message 'line 1\nline 2' type Error blocking;",
		"validation feedback $o/Name message 'line 1\nline 2';",
	} {
		msg, notes := templateMessage(t, "mdl 1;\n", stmt)
		if !isTemplateText(msg, "line 1\nline 2") {
			t.Errorf("mdl 1 %q: want the template text, got %#v", stmt, msg)
		}
		if len(notes) != 0 {
			t.Errorf("mdl 1 %q: the new meaning must not warn: %v", stmt, notes)
		}

		msg, notes = templateMessage(t, "", stmt)
		if se, ok := msg.(*ast.SourceExpr); !ok || se.Source != "'line 1\nline 2'" {
			t.Errorf("mdl 0 %q: want the expression stored as written, got %#v", stmt, msg)
		}
		if len(notes) != 1 || notes[0].Line != 2 || notes[0].Fix == nil {
			t.Errorf("mdl 0 %q: want one MDL-V1-TEMPLATE on line 2 with a rewrite, got %+v", stmt, notes)
		}
	}
}

// With template parameters, a literal that spans lines is the template text
// under both versions: mdl 0 wrote the literal's source, quotes and all, as
// the log's template text, and made a message's text a `{1}` parameter that
// shifted every stated parameter one place, so `{1}` in the text was never
// the value bound to it.
func TestTemplateLineBreak_WithParameters(t *testing.T) {
	for _, stmt := range []string{
		"log info 'Deleted {1} records.\n' with ({1} = toString($n));",
		"show message 'Deleted {1} records.\n' type Information with ({1} = toString($n)) blocking;",
		"validation feedback $o/Name message 'Deleted {1} records.\n' with ({1} = toString($n));",
	} {
		for _, header := range []string{"", "mdl 1;\n"} {
			msg, notes := templateMessage(t, header, stmt)
			if !isTemplateText(msg, "Deleted {1} records.\n") {
				t.Errorf("%q %q: want the template text, got %#v", header, stmt, msg)
			}
			if len(notes) != 0 {
				t.Errorf("%q %q: nothing to warn about, got %v", header, stmt, notes)
			}
		}
	}
}

// Control: a message that is not one literal is an expression under both
// versions, and a one-line literal is the template text under both.
func TestTemplateLineBreak_Controls(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		msg, notes := templateMessage(t, header, "log info 'a' +\n  toString($n);")
		if _, ok := msg.(*ast.SourceExpr); !ok || len(notes) != 0 {
			t.Errorf("%q: an expression over two lines stays an expression, got %#v %v", header, msg, notes)
		}
		msg, notes = templateMessage(t, header, "log info 'one line';")
		if !isTemplateText(msg, "one line") || len(notes) != 0 {
			t.Errorf("%q: a one-line literal is the template text, got %#v %v", header, msg, notes)
		}
		// A node that is a literal over two lines is an expression, not a template.
		prog, errs := Build(header + templateFlow + "log info node 'a\nb' 'x';\nend;\n")
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		node := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt).Body[0].(*ast.LogStmt).Node
		if _, ok := node.(*ast.SourceExpr); !ok {
			t.Errorf("%q: the log node is an expression, got %#v", header, node)
		}
	}
}

// The rewrite fmt --upgrade applies under mdl 0 keeps the `{1}` parameter
// the literal was, spelled so that mdl 1 reads it the same way.
func TestTemplateLineBreak_FixKeepsTheParameter(t *testing.T) {
	for stmt, want := range map[string]string{
		"log info 'a\nb';":                                              "log info '{1}' with ({1} = 'a\nb');",
		"show message 'a\nb' type Error;":                               "show message '{1}' type Error with ({1} = 'a\nb');",
		"show message 'a\nb' blocking;":                                 "show message '{1}' with ({1} = 'a\nb') blocking;",
		"validation feedback $o/Name message 'a\nb' on error continue;": "validation feedback $o/Name message '{1}' with ({1} = 'a\nb') on error continue;",
	} {
		src := templateFlow + stmt + "\nend;\n"
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("%q: %v", stmt, errs[0])
		}
		var fixed string
		for _, n := range prog.LanguageNotes {
			if n.Code == templateLineBreak.Code && n.Fix != nil {
				fixed = applyEditsForTest(src, n.Fix.Edits)
			}
		}
		if !strings.Contains(fixed, want) {
			t.Errorf("%q: want %q in\n%s", stmt, want, fixed)
		}
	}
}

// applyEditsForTest applies non-overlapping rune-offset edits to src.
func applyEditsForTest(src string, edits []ast.TextEdit) string {
	r := []rune(src)
	sorted := append([]ast.TextEdit(nil), edits...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Start > sorted[j-1].Start; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	for _, e := range sorted {
		r = append(r[:e.Start], append([]rune(e.Text), r[e.Stop:]...)...)
	}
	return string(r)
}
