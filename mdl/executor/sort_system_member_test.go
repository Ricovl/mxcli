// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// `sort by CreatedDate` — the spelling describe prints for AutoCreatedDate —
// is CE1613 on mxbuild 11.14.0; `sort by createdDate` and
// `sort by M.E.createdDate` build clean. The case of the system member is the
// whole difference, and check passed all four.
func TestCheckSortSystemMemberSpelling(t *testing.T) {
	const setup = `mdl 1;
create module ModD;
create persistent entity ModD.Doc extends System.FileDocument (Title: String(100));`
	const flow = `mdl 1;
create microflow ModD.SUB_Sort ()
begin
  retrieve $A from %s sort by %s asc;
end;`
	cases := []struct {
		entity, sort, want string
	}{
		{"ModD.Doc", "CreatedDate", "spelled `createdDate`"},
		{"ModD.Doc", "ModD.Doc.CreatedDate", "Write `sort by ModD.Doc.createdDate`"},
		{"ModD.Doc", "ChangedDate", "spelled `changedDate`"},
		// Controls.
		{"ModD.Doc", "createdDate", ""},
		{"ModD.Doc", "ModD.Doc.createdDate", ""},
	}
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	for _, c := range cases {
		src := strings.Replace(strings.Replace(flow, "%s", c.entity, 1), "%s", c.sort, 1)
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		if c.want == "" {
			if got != "" {
				t.Errorf("%s sort by %s: builds clean; check reported:\n%s", c.entity, c.sort, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) || !strings.Contains(got, "CE1613") {
			t.Errorf("%s sort by %s: check must report %q, reported:\n%s", c.entity, c.sort, c.want, got)
		}
	}

	// An entity the script declares with `CreatedDate: AutoCreatedDate`: the
	// declaration names the member, not an attribute.
	src := `mdl 1;
create persistent entity ModD.Fresh (Name: String(10), CreatedDate: AutoCreatedDate);
create microflow ModD.SUB_Fresh ()
begin
  retrieve $A from ModD.Fresh sort by CreatedDate asc;
end;`
	if got := strings.Join(agreeCheck(t, exec, dir, src), "\n"); !strings.Contains(got, "spelled `createdDate`") {
		t.Errorf("script-declared AutoCreatedDate: check must report the spelling, reported:\n%s", got)
	}
}
