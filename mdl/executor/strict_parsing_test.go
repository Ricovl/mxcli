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
			e.registry.handlers[reflect.TypeOf(&ast.ShowStmt{})] = func(*ExecContext, ast.Statement) error {
				ran++
				return nil
			}
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
