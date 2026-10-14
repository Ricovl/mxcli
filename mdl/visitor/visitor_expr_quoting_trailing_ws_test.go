// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
)

// mendixlabs/mxcli#1481. A quoted attribute segment followed by whitespace
// before its `,` / `)` / `;` was stored with its quotes ($Other/"C"), and only
// mx check rejected it — CE0117 "Error(s) in expression." The whitespace makes
// the visitor keep the slot's source text for layout (appendSourceExpressionSuffix),
// and that raw text skipped the quote-stripping every other source path does.
// The commonest trigger is a multi-line change/create with `)` on its own line,
// which fails only on the last member.
func TestTrailingWhitespaceSlotStripsIdentifierQuotes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"change, ) on own line", "change $Item (\n    \"A\" = $Other/\"A\",\n    \"B\" = $Other/\"B\",\n    \"C\" = $Other/\"C\"\n  );"},
		{"change, if-then-else value, ) on own line", "change $Item (\n    \"A\" = $Other/\"A\",\n    \"C\" = if $Other/\"B\" = empty then $Other/\"A\" else $Other/\"C\"\n  );"},
		{"create, ) on own line", "$New = create Repro.Item (\n    \"A\" = $Other/\"A\",\n    \"C\" = $Other/\"C\"\n  );"},
		{"change, one member, ) on own line", "change $Item (\n    \"C\" = $Other/\"C\"\n  );"},
		{"change on one line, space before , and )", "change $Item (\"A\" = $Other/\"A\" , \"C\" = $Other/\"C\" );"},
		{"declare, ; on next line", "declare $V String = $Other/\"C\"\n  ;"},
		{"set, ; on next line", "set $Item/\"C\" = $Other/\"C\"\n  ;"},
		{"call microflow, ) on own line", "call microflow Repro.Sub (\n    Value = $Other/\"C\"\n  );"},
		// Controls: no whitespace before the delimiter — these always passed.
		{"control: change, ) on last line", "change $Item (\n    \"A\" = $Other/\"A\",\n    \"C\" = $Other/\"C\");"},
		{"control: declare, ; on same line", "declare $V String = $Other/\"C\";"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create or modify microflow Repro.\"Mf\" (\n  $Item: Repro.\"Item\",\n  $Other: Repro.\"Item\"\n)\nbegin\n  " +
				tc.body + "\n  return;\nend;"
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			values := storedSlotValues(mf.Body)
			if len(values) == 0 {
				t.Fatal("no expression slots found")
			}
			for _, v := range values {
				if strings.ContainsAny(v, "\"`") {
					t.Errorf("stored expression %q keeps identifier quotes (CE0117)", v)
				}
			}
		})
	}
}

// A double quote inside a single-quoted string literal is the value, not an
// identifier quote, and must survive the trailing-whitespace path too.
func TestTrailingWhitespaceSlotKeepsQuoteInsideStringLiteral(t *testing.T) {
	prog, errs := Build("create microflow Repro.Mf ($Item: Repro.Item)\nbegin\n  change $Item (\n    \"C\" = 'say \"hi\"' + $Item/\"C\"\n  );\n  return;\nend;")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	values := storedSlotValues(prog.Statements[0].(*ast.CreateMicroflowStmt).Body)
	if len(values) != 1 {
		t.Fatalf("values = %q", values)
	}
	if want := `'say "hi"' + $Item/C`; values[0] != want {
		t.Errorf("stored = %q, want %q", values[0], want)
	}
}

// storedSlotValues renders the expression slots of the statements the
// trailing-whitespace path covers, as the builder stores them.
func storedSlotValues(body []ast.MicroflowStatement) []string {
	var out []string
	add := func(e ast.Expression) {
		if e != nil {
			out = append(out, mendixexpr.String(e))
		}
	}
	for _, s := range body {
		switch st := s.(type) {
		case *ast.ChangeObjectStmt:
			for _, c := range st.Changes {
				add(c.Value)
			}
		case *ast.CreateObjectStmt:
			for _, c := range st.Changes {
				add(c.Value)
			}
		case *ast.DeclareStmt:
			add(st.InitialValue)
		case *ast.MfSetStmt:
			add(st.Value)
		case *ast.CallMicroflowStmt:
			for _, a := range st.Arguments {
				add(a.Value)
			}
		}
	}
	return out
}
