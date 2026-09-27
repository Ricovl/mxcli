// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func languageViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out []linter.Violation
	for _, v := range ValidateProgram(prog, "") {
		if strings.HasPrefix(v.RuleID, "MDL-LANG") || strings.HasPrefix(v.RuleID, "MDL-TEST") {
			out = append(out, v)
		}
	}
	return out
}

// Before beta, `mdl 1;` parses but warns that it may still change (ADR-0011).
func TestValidateLanguageVersion_PreviewWarns(t *testing.T) {
	vs := languageViolations(t, "mdl 1;\nshow entities;")
	if len(vs) != 1 {
		t.Fatalf("want one preview warning, got %v", vs)
	}
	v := vs[0]
	if v.RuleID != "MDL-LANG01" || v.Severity != linter.SeverityWarning ||
		!strings.Contains(v.Message, "preview: may still change") {
		t.Fatalf("got %+v", v)
	}
}

// Control: the same script without the header does not warn.
func TestValidateLanguageVersion_HeaderlessDoesNotWarn(t *testing.T) {
	if vs := languageViolations(t, "show entities;"); len(vs) != 0 {
		t.Fatalf("a headerless script with no gated construct warned: %v", vs)
	}
}

// A construct kept at its old meaning is reported as a warning with its own
// rule ID and line — not an error, since mdl 0 scripts must keep running.
func TestValidateLanguageVersion_ReportsGatedConstructs(t *testing.T) {
	prog := &ast.Program{
		LanguageVersion: langver.V0,
		LanguageNotes:   []ast.LanguageNote{{Line: 7, Code: "MDL-TEST-GATE", Message: "kept"}},
	}
	vs := ValidateLanguageVersion(prog)
	if len(vs) != 1 || vs[0].RuleID != "MDL-TEST-GATE" || vs[0].Severity != linter.SeverityWarning ||
		!strings.Contains(vs[0].Message, "line 7") {
		t.Fatalf("got %+v", vs)
	}
}
