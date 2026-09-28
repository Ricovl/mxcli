// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"
)

// ADR-0011: an alias parses to the IDENTICAL operation as its canonical form —
// the only difference allowed is the deprecation the visitor records for the
// old spelling. Without this, an alias could silently build a different
// change (a value parsed through another rule, a target dropped) and the
// warning would tell the user the two are interchangeable when they are not.
// `fmt --upgrade` relies on the same equivalence (ADR-0011, Negative).
func TestGenericAlter_AliasesBuildTheIdenticalOperation(t *testing.T) {
	pairs := []struct{ name, old, canonical string }{
		{"set property on widget", `set Caption = 'Save' on btnSave`, `set (Caption: 'Save') on btnSave`},
		{"set list with =", `set (Caption = 'x', ButtonStyle = Success) on btn2`, `set (Caption: 'x', ButtonStyle: Success) on btn2`},
		{"set unparenthesised colon", `set Caption: 'Save' on btnSave`, `set (Caption: 'Save') on btnSave`},
		{"page-level set", `set Title = 'Edit'`, `set (Title: 'Edit')`},
		{"quoted key", `set 'showLabel' = false on w1`, `set ('showLabel': false) on w1`},
		{"action", `set Action = call microflow M.ACT on btnGo`, `set (Action: call microflow M.ACT) on btnGo`},
		{"named action slot", `set 'createFileAction' = call microflow M.F on up1`, `set ('createFileAction': call microflow M.F) on up1`},
		{"datasource", `set DataSource = $Param on dv1`, `set (DataSource: $Param) on dv1`},
		{"visible", `set Visible = [Name != ''] on txt1`, `set (Visible: [Name != '']) on txt1`},
		{"expression", `set DynamicClasses = if $x/F then 'a' else '' on c1`, `set (DynamicClasses: if $x/F then 'a' else '') on c1`},
		{"column target", `set Caption = 'Total' on dg.Total`, `set (Caption: 'Total') on dg.Total`},
		{"drop widget", `drop widget a, dg.Total`, `drop a, dg.Total`},
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			oldProg := mustBuild(t, "alter page M.P { "+p.old+"; };")
			canonProg := mustBuild(t, "alter page M.P { "+p.canonical+"; };")
			old := buildAlterPage(t, "alter page M.P { "+p.old+"; };").Operations
			canon := buildAlterPage(t, "alter page M.P { "+p.canonical+"; };").Operations
			if len(old) != 1 || len(canon) != 1 {
				t.Fatalf("want one operation each, got %d and %d", len(old), len(canon))
			}
			if len(oldProg.Deprecations) != 1 {
				t.Fatalf("old spelling %q recorded %v, want one deprecation", p.old, deprecationCodes(oldProg))
			}
			if got := deprecationCodes(canonProg); len(got) != 0 {
				t.Fatalf("canonical spelling %q recorded %v", p.canonical, got)
			}
			if !reflect.DeepEqual(old[0], canon[0]) {
				t.Errorf("alias builds a different operation:\n old:       %#v\n canonical: %#v", old[0], canon[0])
			}
		})
	}
}
