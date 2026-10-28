// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#1255 (mendixlabs/mxcli#610): `create association … to System.User`
// passed `check --references`, and exec first failed with "child entity not
// found: System.User", then — once that was fixed — exited 1 after writing it:
//
//	Error: failed to reconcile access rules of module System: load domain model
//	00000000-0000-0000-0000-000000000002: open …/00000000-0000-0000-0000-000000000002.mxunit:
//	no such file or directory
//
// The unit test of reconcileModuleAccess pins the skip; this one runs the whole
// statement through exec on the Studio Pro-authored PedApp fixture, with an
// access rule in the FROM module so the reconcile has work to do, and asserts
// check and exec agree. Measured on 11.12.5: the resulting project is
// `mx check` clean, `owner both` included.
func TestCreateAssociationToSystemEntity_ExecAgreesWithCheck(t *testing.T) {
	for _, target := range []string{"System.User", "System.Language"} {
		t.Run(target, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			src := fmt.Sprintf(`mdl 1;
create module B610;
create module role B610.Reader;
create persistent entity B610.Cars (Name: String(50));
grant read * on entity B610.Cars to B610.Reader;
create association B610.Cars_Owner from B610.Cars to %s owner both;
`, target)
			assertAgree(t, exec, dir, src, "")

			out.Reset()
			if err := agreeExec(t, exec, "describe association B610.Cars_Owner;"); err != nil {
				t.Fatalf("describe: %v", err)
			}
			if want := "from B610.Cars to " + target; !strings.Contains(out.String(), want) {
				t.Errorf("describe must round-trip %q, got:\n%s", want, out.String())
			}
		})
	}
}
