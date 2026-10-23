// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1612 (ako/mxcli#1540): `create M.Entity (…, System.owner =
// $User)` was refused at exec with
//
//	member "System.owner" on entity M.Entity is not a known association (and a
//	one-qualifier name cannot be an attribute) — create the association first
//	(`create or modify association System.owner from ... to ...`)
//
// advice that cannot apply to a built-in association — and `check -p` passed
// the same script. Measured with mx check on 11.12.5, writing the member as
// the issue expected (Association "System.owner") is CE1050 "Unknown
// association 'System.owner'." in every case: an entity storing its owner, one
// that does not, a System.FileDocument specialization, a specialization of an
// owning root, System.changedBy alike, in a microflow and in a nanoflow. So
// there is nothing to write; the fix is a refusal that says so, in check and
// exec alike.
const systemMemberChangeHead = `mdl 1;
create module B1540;
create persistent entity B1540.Owned (Name: String(50), owner: AutoOwner, changedBy: AutoChangedBy);
create persistent entity B1540.Plain (Name: String(50));
create association B1540.Owned_SeededBy from B1540.Owned to System.User;
`

const systemMemberChangeFlow = `create %s B1540.F ($o: B1540.Owned, $p: B1540.Plain, $User: System.User) returns Boolean
begin
  %s
  return true;
end;
`

func TestSystemMemberChange_CheckAndExecAgree(t *testing.T) {
	cases := []struct {
		name, kind, body, want string
	}{
		{"create sets System.owner", "microflow", `$S = create B1540.Owned (Name = 'x', System.owner = $User);`, "CE1050"},
		{"change sets System.changedBy", "microflow", `change $o (System.changedBy = $User);`, "CE1050"},
		{"set on a member path", "microflow", `set $o/System.owner = $User;`, "CE1050"},
		{"entity that stores no owner", "microflow", `$S = create B1540.Plain (Name = 'x', System.owner = $User);`, "CE1050"},
		{"nanoflow", "nanoflow", `$S = create B1540.Owned (Name = 'x', System.owner = $User);`, "CE1050"},
		{"spelled System.Owner", "microflow", `change $o (System.Owner = $User);`, "System.owner is kept by the runtime"},
		// Controls: an association the project models, and a real System one.
		{"modelled association to System.User", "microflow", `$S = create B1540.Owned (Name = 'x', B1540.Owned_SeededBy = $User);`, ""},
		{"System.User_Language", "microflow", `retrieve $Lang from System.Language first;
  change $User (System.User_Language = $Lang);`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			src := systemMemberChangeHead + fmt.Sprintf(systemMemberChangeFlow, c.kind, c.body)
			assertAgree(t, exec, dir, src, c.want)
			if c.want == "" {
				return
			}
			// The advice the report quoted must be gone: no System association
			// can be created.
			got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
			if strings.Contains(got, "create the association first") {
				t.Errorf("check still advises creating a System association:\n%s", got)
			}
		})
	}
}

// Plain check needs no project to know this: no module but System declares
// these names, and the entity's flags do not change the verdict.
func TestSystemMemberChange_PlainCheck(t *testing.T) {
	prog, errs := visitor.Build(systemMemberChangeHead +
		fmt.Sprintf(systemMemberChangeFlow, "microflow", `$S = create B1540.Owned (Name = 'x', System.owner = $User);`))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var mf *ast.CreateMicroflowStmt
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateMicroflowStmt); ok {
			mf = m
		}
	}
	var got []string
	for _, v := range ValidateMicroflow(mf) {
		if v.RuleID == systemMemberChangeRule {
			got = append(got, v.Message+" | "+v.Suggestion)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one %s violation, got %v", systemMemberChangeRule, got)
	}
	for _, want := range []string{"System.owner", "CE1050", "create association B1540.Owned_User from B1540.Owned to System.User"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("violation does not mention %q: %s", want, got[0])
		}
	}
	if !execEnforcedMicroflowRules[systemMemberChangeRule] {
		t.Errorf("%s was measured (CE1050, 11.12.5) and must be refused by exec, not only reported by check", systemMemberChangeRule)
	}
}
