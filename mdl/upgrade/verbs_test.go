// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R6 (ako/mxcli#755): the old verbs are rewritten in place, in the letter case
// the script uses. `show navigation` and `show settings` become `list` (their
// tables are listings); `show entity X` is not a deprecated spelling of
// anything, so the alias upgrade leaves it alone (it is gated, see
// TestUpgrade_ShowSummaryBlocksTheHeader).
func TestUpgrade_R6Verbs(t *testing.T) {
	src := "SHOW PROJECT SECURITY;\nshow security matrix in M;\nshow structure depth 2;\nshow context of M.F;\n" +
		"show page M.P;\nshow entity M.E;\nshow navigation menu;\nSHOW SETTINGS;\n" +
		"alter user role Clerk remove module roles (M.User);\n" +
		"alter settings workflows remove group 'Approvers';\n" +
		"alter entity M.E add column Note: String(200), drop column Old;\n" +
		"create microflow M.F () begin $R = REST CALL get 'https://x.org' returns string; end;\n" +
		"describe widget combobox;\n" +
		"define fragment Hdr as { dynamictext t (Content: 'x') };\n"
	want := "DESCRIBE APP SECURITY;\ndescribe security matrix in M;\ndescribe structure depth 2;\ndescribe context of M.F;\n" +
		"describe page M.P;\nshow entity M.E;\nlist navigation menu;\nLIST SETTINGS;\n" +
		"alter user role Clerk drop module roles (M.User);\n" +
		"alter settings workflows drop group 'Approvers';\n" +
		"alter entity M.E add attribute Note: String(200), drop attribute Old;\n" +
		"create microflow M.F () begin $R = CALL REST SERVICE get 'https://x.org' returns string; end;\n" +
		"describe widget type combobox;\n" +
		"create fragment Hdr as { dynamictext t (Content: 'x') };\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{
		deprecation.ShowSingleThing: 5, deprecation.UserRoleRemove: 1, deprecation.SettingsRemove: 1,
		deprecation.ColumnForAttribute: 2, deprecation.RestCall: 1, deprecation.DescribeWidgetType: 1,
		deprecation.DefineFragment: 1, deprecation.Show: 2,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if len(res.Unrewritten) != 0 {
		t.Errorf("Unrewritten = %+v, want none", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}

// `show entity X` has no mdl 1 statement, so the header is refused over it
// with the reason, and nothing is written.
func TestUpgrade_ShowSummaryBlocksTheHeader(t *testing.T) {
	_, err := Upgrade("show entity M.E;\nlist entities;\n", Options{AddHeader: true})
	var hb *HeaderBlockedError
	if !errors.As(err, &hb) || len(hb.Constructs) != 1 || hb.Constructs[0].Code != "MDL-V1-SHOWSUMMARY" ||
		hb.Constructs[0].Line != 1 {
		t.Fatalf("err = %v, want the header refused over MDL-V1-SHOWSUMMARY on line 1", err)
	}
}
