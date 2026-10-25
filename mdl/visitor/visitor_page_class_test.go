// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Issue #714 — CREATE PAGE header carries a page-level CSS Class and Style.
func TestCreatePage_ClassAndStyle(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (
		Title: 'x',
		Layout: Atlas_Core.Atlas_Default,
		Class: 'my-page bg-primary',
		Style: 'padding: 10px'
	) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.Class != "my-page bg-primary" {
		t.Errorf("Class = %q, want 'my-page bg-primary'", stmt.Class)
	}
	if stmt.Style != "padding: 10px" {
		t.Errorf("Style = %q, want 'padding: 10px'", stmt.Style)
	}
}

// Class/Style are optional — omitting them leaves the fields empty.
func TestCreatePage_ClassOmitted(t *testing.T) {
	prog, errs := Build(`CREATE PAGE M.P (Title: 'x') { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if stmt.Class != "" || stmt.Style != "" {
		t.Errorf("expected empty Class/Style, got %q/%q", stmt.Class, stmt.Style)
	}
}

// ako/mxcli#1678 — the page's own design properties, written in the header with
// the same list a widget takes.
func TestCreatePage_DesignProperties(t *testing.T) {
	prog, errs := Build(`create page M.P (
		Title: 'x',
		Layout: Atlas_Core.NativePhone_Default,
		DesignProperties: ('Spacing top': 'Inner large', 'Hide header bottom border': on)
	) { dynamictext t (Content: 'x') };`)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	got := prog.Statements[0].(*ast.CreatePageStmtV3).DesignProperties
	if len(got) != 2 || got[0].Key != "Spacing top" || got[0].Value != "Inner large" ||
		got[1].Key != "Hide header bottom border" || got[1].Value != "on" {
		t.Errorf("DesignProperties = %+v", got)
	}
}
