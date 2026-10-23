// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1139 asked for Studio Pro's "Disable activity" in MDL; it
// is `@excluded` on the statement. Only an ActionActivity has the Disabled
// property (Model SDK: ActionActivity.disabled, nowhere else), so on a
// decision, a loop or an end event the annotation parsed, passed check, and
// was dropped on write — the activity the author meant to switch off ran.
func excludedViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	for _, e := range errs {
		t.Fatalf("parse error: %v", e)
	}
	var out []linter.Violation
	var all []linter.Violation
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		all = ValidateMicroflow(s)
	case *ast.CreateNanoflowStmt:
		all = ValidateNanoflow(s)
	default:
		t.Fatalf("unexpected statement %T", s)
	}
	for _, v := range all {
		if v.RuleID == excludedNonActivityRule {
			out = append(out, v)
		}
	}
	return out
}

func TestExcludedOnNonActivityWarns_Issue1139(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"decision", "create microflow M.F ($b: Boolean) begin @excluded if $b then log info 'a'; end if; end;"},
		{"loop", "create microflow M.F ($L: list of M.E) begin @excluded loop $i in $L begin log info 'a'; end loop; end;"},
		{"while", "create microflow M.F ($b: Boolean) begin @excluded while $b begin log info 'a'; end while; end;"},
		{"return", "create microflow M.F () begin log info 'a'; @excluded return; end;"},
		{"raise error", "create microflow M.F () begin log info 'a'; @excluded raise error; end;"},
		{"nested in a branch", "create microflow M.F ($b: Boolean) begin if $b then @excluded if $b then log info 'a'; end if; end if; end;"},
		{"nanoflow decision", "create nanoflow M.N ($b: Boolean) begin @excluded if $b then log info 'a'; end if; end;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := excludedViolations(t, tc.src)
			if len(got) != 1 {
				t.Fatalf("want one %s, got %v", excludedNonActivityRule, got)
			}
			if got[0].Severity != linter.SeverityWarning {
				t.Errorf("severity = %v, want warning", got[0].Severity)
			}
		})
	}
}

// The supported use — an activity — must stay quiet.
func TestExcludedOnActivityDoesNotWarn_Issue1139(t *testing.T) {
	for _, src := range []string{
		"create microflow M.F () begin @excluded log info 'a'; end;",
		"create microflow M.F ($b: Boolean) begin if $b then @excluded log info 'a'; end if; end;",
		"create nanoflow M.N () begin @excluded declare $s String = 'x'; end;",
	} {
		if got := excludedViolations(t, src); len(got) != 0 {
			t.Errorf("%s\n  unexpected %v", src, got)
		}
	}
}
