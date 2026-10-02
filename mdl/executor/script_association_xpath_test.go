// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An association the SCRIPT creates, between entities the PROJECT already has.
// Measured on mxbuild 11.14.0: both statements below build clean, and check
// --references refused both — the retrieve as "neither an attribute nor an
// association" (the XPath walk resolved associations from the project only),
// the create as "has no member Child_Parent" (authoredMembers ignored
// CREATE ASSOCIATION). The control is the same flow once the association is
// in the project, which already passed.
func TestCheckScriptAssociation_OnProjectEntities(t *testing.T) {
	const setup = `mdl 1;
create module ModA;
create persistent entity ModA.Parent (Name: String(100));
create persistent entity ModA.Child (Name: String(100));`
	const flow = `create microflow ModA.SUB_Find ($P: ModA.Parent)
begin
  retrieve $Cs from ModA.Child where [ModA.Child_Parent = $P];
  retrieve $Ps from ModA.Parent where [ModA.Child_Parent/ModA.Child/Name = 'x'];
  $C = create ModA.Child (Child_Parent = $P);
end;`
	const assoc = "create association ModA.Child_Parent from ModA.Child to ModA.Parent;\n"

	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if got := agreeCheck(t, exec, dir, "mdl 1;\n"+assoc+flow); len(got) != 0 {
		t.Errorf("association created by the script: mxbuild builds this clean; check reported:\n%s", strings.Join(got, "\n"))
	}
	// A typo is still caught: the walk did not go quiet.
	bad := strings.Replace(flow, "[ModA.Child_Parent = $P]", "[ModA.Child_Parnet = $P]", 1)
	if got := strings.Join(agreeCheck(t, exec, dir, "mdl 1;\n"+assoc+bad), "\n"); !strings.Contains(got, "ModA.Child_Parnet") {
		t.Errorf("a misspelled association must still be reported, got:\n%s", got)
	}
	// Control: the association stored in the project.
	if err := agreeExec(t, exec, "mdl 1;\n"+assoc); err != nil {
		t.Fatalf("store association: %v", err)
	}
	assertAgree(t, exec, dir, "mdl 1;\n"+flow, "")
}
