// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1645 without a project: `mxcli check script.mdl` judges the clashes
// between members the script declares, and between one of them and a System
// member, against the same verdict exec gives (MDL-MEMBER01). What needs the
// project is silent here and left to `check -p`.
func TestMemberClash_NoProjectCheck(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"inherited attribute", `mdl 1;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Emp extends Other.Person (Name: String(50), Code: String(10));`,
			"HR.Emp inherits 'Name' from Other.Person (attribute Other.Person.Name)"},
		{"inherited association", `mdl 1;
create persistent entity Other.Person (Name: String(50));
create persistent entity Other.Dept (Name: String(50));
create association Other.Person_Dept from Other.Person to Other.Dept;
create persistent entity HR.Emp extends Other.Person (Code: String(10));
create association HR.Person_Dept from HR.Emp to Other.Dept;`,
			"HR.Emp inherits 'Person_Dept' from Other.Person (association Other.Person_Dept)"},
		{"System ancestor", `mdl 1;
create persistent entity HR.Acct extends System.User (Name: String(50));`,
			"HR.Acct inherits 'Name' from System.User (attribute System.User.Name)"},
		{"System ancestor's association", `mdl 1;
create persistent entity HR.Acct extends System.User (UserRoles: String(50));`,
			"HR.Acct inherits 'UserRoles' from System.User (association System.UserRoles)"},
		{"two levels below a System entity", `mdl 1;
create persistent entity HR.Pic extends System.Image (Caption: String(50));
create persistent entity HR.Thumb extends HR.Pic (Size: Long);`,
			"HR.Thumb inherits 'Size' from System.FileDocument"},
		// Controls.
		{"clean", `mdl 1;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Emp extends Other.Person (Code: String(10));
create persistent entity HR.Contractor extends Other.Person (Code: String(10));
create persistent entity HR.Cased extends Other.Person (name: String(10));`, ""},
		{"an entity the script does not declare", `mdl 1;
alter entity HR.Emp add attribute Name: String(50);`, ""},
		// Dropping an entity drops the associations at either end with it.
		{"an association dropped with its entity", `mdl 1;
create persistent entity M.A (X: String(10));
create persistent entity M.B (Y: String(10));
create association M.Link from M.A to M.B;
drop entity M.A;
create persistent entity M.A (Link: String(10));`, ""},
		{"a renamed module", `mdl 1;
create persistent entity Other.Person (Name: String(50));
rename module Other to Base;
create persistent entity HR.Emp extends Base.Person (Name: String(50));`,
			"HR.Emp inherits 'Name' from Base.Person"},
		{"dropped before it is re-added", `mdl 1;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Emp extends Other.Person (Code: String(10));
alter entity HR.Emp drop attribute Code;
alter entity Other.Person add attribute Code: String(10);`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(c.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs[0])
			}
			var got []string
			for _, v := range ValidateProgram(prog, "") {
				if v.RuleID == "MDL-MEMBER01" {
					got = append(got, v.Message)
				}
			}
			if c.want == "" {
				if len(got) != 0 {
					t.Errorf("control must pass, got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], c.want) || !strings.Contains(got[0], "CE0069") {
				t.Errorf("want one MDL-MEMBER01 naming %q and CE0069, got %v", c.want, got)
			}
		})
	}
}
