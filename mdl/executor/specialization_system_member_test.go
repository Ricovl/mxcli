// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// `owner: AutoOwner` (or any Auto* system member) on a specialization was
// dropped without a word — "Created" on the first run, "Unchanged" on the next,
// and nothing written, because Mendix keeps the flags on the root of the
// generalization chain and a specialization has nowhere to store them.
// Measured on mxbuild 11.14.0: `SM.Spec extends SM.Parent (owner: AutoOwner)`
// executed clean, and `[System.owner = '[%CurrentUser%]']` on SM.Spec then
// failed the build with CE0161.

// Inherited: System.FileDocument stores owner, so the declaration is
// redundant. Exec must say so rather than stay silent — and still succeed.
func TestSpecializationSystemMember_InheritedIsReported(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const src = `mdl 1;
create module ModS;
create persistent entity ModS.Doc extends System.FileDocument (Title: String(100), owner: AutoOwner);`
	assertAgree(t, exec, dir, src, "")
	if !strings.Contains(out.String(), "is inherited") || !strings.Contains(out.String(), "System.FileDocument already stores owner") {
		t.Errorf("exec must report the AutoOwner as inherited, printed:\n%s", out.String())
	}
}

// Not stored by the root: refused by check and exec alike, naming the root.
// The control is the same statement against a root that does store owner.
func TestSpecializationSystemMember_NotOnRootIsRefused(t *testing.T) {
	const setup = `mdl 1;
create module ModS;
create persistent entity ModS.Base (Name: String(100));
create persistent entity ModS.OwnedBase (Name: String(100), owner: AutoOwner);
create persistent entity ModS.Spec3 extends ModS.Base (Extra: String(10));`

	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir,
		"mdl 1;\ncreate persistent entity ModS.Spec extends ModS.Base (Extra: String(10), owner: AutoOwner);",
		"whose root ModS.Base does not store owner")
	assertAgree(t, exec, dir,
		"mdl 1;\nalter entity ModS.Spec3 add attribute owner: AutoOwner;",
		"whose root ModS.Base does not store owner")
	// Control.
	assertAgree(t, exec, dir,
		"mdl 1;\ncreate persistent entity ModS.Spec2 extends ModS.OwnedBase (Extra: String(10), owner: AutoOwner);", "")
}
