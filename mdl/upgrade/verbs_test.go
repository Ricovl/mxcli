// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R6 (ako/mxcli#755): the old verbs are rewritten in place, in the letter case
// the script uses; the `show` summaries with no describe equivalent are left
// alone and reported.
func TestUpgrade_R6Verbs(t *testing.T) {
	src := "SHOW PROJECT SECURITY;\nshow security matrix in M;\nshow structure depth 2;\nshow context of M.F;\n" +
		"show page M.P;\nshow entity M.E;\n" +
		"alter user role Clerk remove module roles (M.User);\n" +
		"alter settings workflows remove group 'Approvers';\n" +
		"alter entity M.E add column Note: String(200), drop column Old;\n" +
		"create microflow M.F () begin $R = REST CALL get 'https://x.org' returns string; end;\n" +
		"describe widget combobox;\n" +
		"define fragment Hdr as { dynamictext t (Content: 'x') };\n"
	want := "DESCRIBE APP SECURITY;\ndescribe security matrix in M;\ndescribe structure depth 2;\ndescribe context of M.F;\n" +
		"describe page M.P;\nshow entity M.E;\n" +
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
		deprecation.DefineFragment: 1,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.ShowSingleThing || res.Unrewritten[0].Line != 6 {
		t.Errorf("Unrewritten = %+v, want the `show entity` on line 6", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
