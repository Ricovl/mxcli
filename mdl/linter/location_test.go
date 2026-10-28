// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Location.DocumentName is documented as the document's own name ("Customer"),
// and QualifiedName put the module in front of it. Most of the bundled Starlark
// rules pass `document_name=<x>.qualified_name` instead — QUAL004, CONV001-010,
// the SEC rules, ~30 in all — so every one of their findings printed as
// `at M.M.NAV_Home`, the module twice (ako/mxcli#1484's repro). The Go rules
// mostly strip the module first; missing_translations does not.
//
// Fixing the rules one by one would leave a project's stale copies of them
// (rules are written into the project by `mxcli init` and kept until it is
// re-run) and every user-written rule still printing the module twice. So the
// one place the qualified name is assembled tolerates both spellings.
func TestLocationQualifiedNameDoesNotRepeatTheModule(t *testing.T) {
	for _, tc := range []struct {
		name string
		loc  linter.Location
		want string
	}{
		{"short document name", linter.Location{Module: "M", DocumentType: "Microflow", DocumentName: "NAV_Home"}, "M.NAV_Home"},
		{"qualified document name", linter.Location{Module: "M", DocumentType: "Microflow", DocumentName: "M.NAV_Home"}, "M.NAV_Home"},
		// missing_translations names an element below a document.
		{"qualified name of a member", linter.Location{Module: "Sales", DocumentType: "attribute", DocumentName: "Sales.Order.Status"}, "Sales.Order.Status"},
		// CONV018 reports a module about itself.
		{"module-level finding", linter.Location{Module: "Sales", DocumentType: "Module", DocumentName: "Sales"}, "Sales"},
		{"project-level finding", linter.Location{DocumentType: "security", DocumentName: "ProjectSecurity"}, "ProjectSecurity"},

		// CONTROLS: a document may share its module's name, and a module name
		// may be a prefix of another module's — neither is a repeat.
		{"document named like its module", linter.Location{Module: "Sales", DocumentType: "Microflow", DocumentName: "Sales"}, "Sales.Sales"},
		{"module name is a prefix of the document's", linter.Location{Module: "M", DocumentType: "Microflow", DocumentName: "MX_Home"}, "M.MX_Home"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.loc.QualifiedName(); got != tc.want {
				t.Errorf("QualifiedName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestShippedRuleLocationPrintsTheModuleOnce is the reported line, end to end:
// the shipped QUAL004 rule over a catalog, through the text formatter.
func TestShippedRuleLocationPrintsTheModuleOnce(t *testing.T) {
	r, err := linter.LoadStarlarkRule(filepath.Join(flowRulesDir, "orphaned_elements.star"))
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	violations := r.Check(linter.NewLintContextFromDB(flowKindsFixtureDB(t)))

	var hit *linter.Violation
	for i := range violations {
		if strings.HasSuffix(violations[i].Location.DocumentName, "Big_MF") {
			hit = &violations[i]
		}
	}
	// CONTROL: the fixture's Big_MF is unreferenced, so QUAL004 reports it.
	if hit == nil {
		t.Fatalf("QUAL004 did not report Sales.Big_MF; got %d findings", len(violations))
	}

	var out bytes.Buffer
	if err := (&linter.TextFormatter{NoSummary: true}).Format([]linter.Violation{*hit}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Sales.Sales.") {
		t.Errorf("the location repeats the module:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "at Sales.Big_MF") {
		t.Errorf("want the location `at Sales.Big_MF`, got:\n%s", out.String())
	}
}
