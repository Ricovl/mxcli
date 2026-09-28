// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): `Visible: [expr]` / `Editable: [expr]` become the bare
// expression the brackets stored — a bare attribute rooted in $currentObject.
func TestUpgrade_WidgetConditionsBare(t *testing.T) {
	page := func(props string) string {
		return "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n  dataview dv (DataSource: $E) {\n    textbox t (Attribute: Name, " +
			props + ") -- keep\n  }\n};"
	}
	for _, tc := range []struct{ name, src, want string }{
		{"visible, attribute rooted", page("Visible: [Active]"), page("Visible: $currentObject/Active")},
		{"editable", page("Editable: [$currentObject/Status = 'Open']"), page("Editable: $currentObject/Status = 'Open'")},
		{"both", page("Visible: [Active], Editable: [not(Locked)]"), page("Visible: $currentObject/Active, Editable: not($currentObject/Locked)")},
		{"alter page set", "alter page M.P { set (Visible: [Active]) on t };", "alter page M.P { set (Visible: $currentObject/Active) on t };"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mustUpgrade(t, tc.src, Options{})
			if res.Source != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", res.Source, tc.want)
			}
			if len(res.Unrewritten) != 0 {
				t.Errorf("Unrewritten = %+v", res.Unrewritten)
			}
		})
	}
}

// A condition the rewrite cannot carry — here one holding an mdl 0 backslash
// escape, whose value the string-escape rewrite owns — is reported, not
// rewritten.
func TestUpgrade_WidgetConditionNotBareIsReported(t *testing.T) {
	src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { textbox t (Attribute: Name, Visible: [$currentObject/Name = 'a\tb']) } };`
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Errorf("rewritten to %q", res.Source)
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.BracketedWidgetCondition {
		t.Errorf("Unrewritten = %+v, want one %s", res.Unrewritten, deprecation.BracketedWidgetCondition)
	}
}

// A constant condition has no bare spelling — bare, `true` is the plain value,
// stored differently — so its brackets are not reported at all.
func TestUpgrade_ConstantWidgetConditionIsNotAnAlias(t *testing.T) {
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { textbox t (Attribute: Name, Editable: [true]) } };"
	res := mustUpgrade(t, src, Options{})
	if res.Source != src || len(res.Unrewritten) != 0 {
		t.Errorf("rewritten to %q, Unrewritten %+v", res.Source, res.Unrewritten)
	}
}
