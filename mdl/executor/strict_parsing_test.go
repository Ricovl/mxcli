// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The R11 strictness changes (#732), end to end through check and exec. Under
// mdl 0 each script keeps running, and `check` names the construct with its
// registered code as a warning; under mdl 1 the same script does not get past
// the parser, so exec never starts.
var strictParsingCases = []struct {
	name, src, code string
	refusal         string // what the mdl 1 error says
}{
	{"missing semicolon", "show modules\nshow entities;", "MDL-V1-SEMI", "no terminating `;`"},
	{"slash terminator", "show modules;\n/\nshow entities;", "MDL-V1-SLASH", "`/` is not a statement terminator"},
	{"unknown property key", "show modules;\ncreate published rest service M.S (Path: 'rest/s', Verison: '1.0') { };", "MDL-V1-PROP", "did you mean 'Version'"},
	{"mis-shaped property value", "show modules;\ncreate rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation Post { Method: post, Path: '/u', Response: json from $X } };", "MDL-V1-PROPVALUE", "takes none or json as $var"},
	// A change of meaning, not a rejection: see TestStringEscapeUnderEachVersion.
	// R7 (ako/mxcli#755): a session command belongs at the REPL or on the
	// command line; under mdl 0 the script still sets the format.
	{"session command", "show modules;\nset format = json;", "MDL-V1-SESSION", "`set format` is a session command"},
	{"backslash escape", "show modules;\ncreate persistent entity M.N (T: String(20) default 'C:\\temp');", "MDL-V1-ESCAPE", ""},
}

// countStatements registers handlers that count what exec runs.
func countStatements(e *Executor, ran *int, defaults *[]any) {
	e.registry.handlers[reflect.TypeOf(&ast.ShowStmt{})] = func(*ExecContext, ast.Statement) error {
		*ran++
		return nil
	}
	for _, s := range []ast.Statement{&ast.CreatePublishedRestServiceStmt{}, &ast.CreateRestClientStmt{}, &ast.SetStmt{}} {
		e.registry.handlers[reflect.TypeOf(s)] = func(*ExecContext, ast.Statement) error {
			*ran++
			return nil
		}
	}
	e.registry.handlers[reflect.TypeOf(&ast.CreateEntityStmt{})] = func(_ *ExecContext, s ast.Statement) error {
		*ran++
		*defaults = append(*defaults, s.(*ast.CreateEntityStmt).Attributes[0].DefaultValue)
		return nil
	}
}

func TestStrictParsingWarnsUnderMdl0AndRunsTheScript(t *testing.T) {
	for _, tc := range strictParsingCases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("mdl 0 must keep parsing it: %v", errs)
			}
			var warned bool
			for _, v := range ValidateProgram(prog, "") {
				if v.RuleID == tc.code {
					warned = true
					if v.Severity != linter.SeverityWarning {
						t.Errorf("%s must be a warning under mdl 0, got %v", tc.code, v.Severity)
					}
				}
			}
			if !warned {
				t.Errorf("check did not report %s", tc.code)
			}

			e := New(io.Discard)
			var ran int
			var defaults []any
			countStatements(e, &ran, &defaults)
			if err := e.ExecuteProgram(prog); err != nil {
				t.Fatal(err)
			}
			if ran != 2 {
				t.Errorf("mdl 0: ran %d statements, want 2", ran)
			}
		})
	}
}

func TestStrictParsingRefusedUnderMdl1(t *testing.T) {
	for _, tc := range strictParsingCases {
		if tc.refusal == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			_, errs := visitor.Build("mdl 1;\n" + tc.src)
			var refused bool
			for _, err := range errs {
				refused = refused || strings.Contains(err.Error(), tc.refusal)
			}
			if !refused {
				t.Fatalf("mdl 1 must refuse it with %q, got %v\n%s", tc.refusal, errs, tc.src)
			}
		})
	}
}

// Under mdl 0 exec stores the backslash escape's old value (a tab); under
// mdl 1 the backslash is the character written.
func TestStringEscapeUnderEachVersion(t *testing.T) {
	for header, want := range map[string]string{"": "C:\temp", "mdl 1;\n": `C:\temp`} {
		prog, errs := visitor.Build(header + "create persistent entity M.N (T: String(20) default 'C:\\temp');")
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		e := New(io.Discard)
		var ran int
		var defaults []any
		countStatements(e, &ran, &defaults)
		if err := e.ExecuteProgram(prog); err != nil {
			t.Fatal(err)
		}
		if len(defaults) != 1 || defaults[0] != want {
			t.Errorf("%q: exec saw default %q, want %q", header, defaults, want)
		}
	}
}
