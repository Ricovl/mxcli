// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1409 (ako/mxcli#1315): an entity's grant naming one of its
// associations was printed with the entity, ahead of the association, so the
// describe output was refused on replay:
//
//	Created entity: ModA.EntA
//	Error: entity ModA.EntA has no member(s) EntA_EntB; grant only names
//	members of the entity or of an entity it inherits from
//
// after `check` had said "Check passed!".

// describeOrderSetup is the reported module: an entity whose grant names the
// association it owns.
const describeOrderSetup = `mdl 1;
create module ModA;
create persistent entity ModA.EntA (Name: String(50));
create persistent entity ModA.EntB (Name: String(50));
create association ModA.EntA_EntB from ModA.EntA to ModA.EntB;
create module role ModA.Admin;
grant read (Name, EntA_EntB), write (Name) on entity ModA.EntA to ModA.Admin;`

// `describe module … with all` prints every grant after every association, so
// its output replays into a project that has only the module and its role.
func TestDescribeModule_GrantFollowsTheAssociationItNames(t *testing.T) {
	src, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, src, describeOrderSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := src.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: "ModA"}, WithAll: true}); err != nil {
		t.Fatalf("describe module: %v\n%s", err, out.String())
	}
	described := out.String()
	grant := strings.Index(described, "on entity ModA.EntA to ModA.Admin")
	assoc := strings.Index(described, "association ModA.EntA_EntB")
	if grant < 0 || assoc < 0 {
		t.Fatalf("describe module lost the grant or the association:\n%s", described)
	}
	if grant < assoc {
		t.Errorf("the grant naming EntA_EntB is printed before the association:\n%s", described)
	}

	// Replay into a second project holding only the module and the role.
	dst, dstOut, _ := openPedAppCopy(t)
	if err := agreeExec(t, dst, "mdl 1;\ncreate module ModA;\ncreate module role ModA.Admin;"); err != nil {
		t.Fatalf("target setup: %v\n%s", err, dstOut.String())
	}
	if err := agreeExec(t, dst, languageHeader.ReplaceAllString(described, "")); err != nil {
		t.Fatalf("describe module output does not replay: %v\n%s", err, described)
	}
}

// check says what exec says: a grant naming an association a later statement
// creates is refused with the reorder hint, on an entity the script declares as
// well as on a stored one. The control is the order describe now prints.
func TestCheckExecAgree_GrantNamesAssociationCreatedLater(t *testing.T) {
	const entities = `mdl 1;
create module ModA;
create module role ModA.Admin;
create persistent entity ModA.EntA (Name: String(50));
create persistent entity ModA.EntB (Name: String(50));
`
	const grant = "grant read (EntA_EntB, Name), write (Name) on entity ModA.EntA to ModA.Admin;\n"
	const assoc = "create association ModA.EntA_EntB from ModA.EntA to ModA.EntB;\n"

	t.Run("grant above the association (the reported shape)", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		checkSays := strings.Join(agreeCheck(t, exec, dir, entities+grant+assoc), "\n")
		for _, want := range []string{"has no member(s) EntA_EntB", "created later in this script"} {
			if !strings.Contains(checkSays, want) {
				t.Errorf("check must report %q, reported:\n%s", want, checkSays)
			}
		}
		if err := agreeExec(t, exec, entities+grant+assoc); err == nil || !strings.Contains(err.Error(), "has no member(s) EntA_EntB") {
			t.Errorf("exec must refuse the grant, got: %v", err)
		}
	})
	t.Run("control: grant below the association", func(t *testing.T) {
		exec, _, dir := openPedAppCopy(t)
		assertAgree(t, exec, dir, entities+assoc+grant, "")
	})
}
