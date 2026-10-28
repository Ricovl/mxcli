// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
)

// A local module that links to a marketplace module's entity owns that link: a
// cross-module association is stored in the FROM entity's module. It is still the
// update's casualty — DROP MODULE deletes the marketplace entity, and DeleteEntity
// cascades into every other domain model and removes the cross-module
// associations that reach the entity by name. `--save-edits` enumerated only the
// replaced module, so the one piece of local work the update was certain to
// destroy was the one it never saved (mendixlabs/mxcli#1502).
const crossModuleAssocMDL = `
create persistent entity MyFirstModule.Config ( Note: String(100) );
create association MyFirstModule.Config_Account from MyFirstModule.Config to Administration.Account;
`

const crossModuleAssoc = "MyFirstModule.Config_Account"

// describeAssoc returns the DESCRIBE output for an association, and whether it exists.
func describeAssoc(t *testing.T, mprPath, qualified string) (string, bool) {
	t.Helper()
	var sink bytes.Buffer
	exec := executor.New(&sink)
	exec.SetBackendFactory(testBackend)
	defer exec.Close()
	if err := exec.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	qn, err := splitQualified(qualified)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeAssociation, Name: qn}); err != nil {
		return err.Error(), false
	}
	return sink.String(), true
}

// The mechanism, and the control for the test below: an update of Administration
// removes a local module's association into it. Without this the save test could
// pass against a fixture where nothing was ever at risk.
func TestPerformUpdate_RemovesCrossModuleAssociationsIntoTheModule(t *testing.T) {
	target := copyFixture(t)
	execMDL(t, target, crossModuleAssocMDL)
	if _, ok := describeAssoc(t, target, crossModuleAssoc); !ok {
		t.Fatalf("control: %s was not created", crossModuleAssoc)
	}

	if _, err := PerformUpdate(target, copyFixture(t), emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend); err != nil {
		t.Fatalf("PerformUpdate: %v", err)
	}
	if out, ok := describeAssoc(t, target, crossModuleAssoc); ok {
		t.Fatalf("expected the update to remove %s (it reaches a dropped entity); it survived:\n%s\n"+
			"if the update now preserves it, the save below is no longer the only rescue — revisit #1502",
			crossModuleAssoc, out)
	}
}

// The reported symptom: --save-edits saves nothing for the association, so after
// the update it is gone with no file to replay.
func TestSaveEdits_SavesCrossModuleAssociationsIntoTheModule(t *testing.T) {
	target := copyFixture(t)
	execMDL(t, target, crossModuleAssocMDL)

	installed, err := SnapshotModule(target, "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot installed: %v", err)
	}
	published, err := SnapshotModule(copyFixture(t), "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot package: %v", err)
	}
	drift := Compare(installed, published)

	if got := drift.DependentNames(); len(got) != 1 || got[0] != "ASSOCIATION "+crossModuleAssoc {
		t.Fatalf("dependents = %v; want exactly ASSOCIATION %s", got, crossModuleAssoc)
	}
	// The module itself is untouched: the association lives in MyFirstModule.
	// It must not be passed off as an edit to Administration.
	if drift.LocallyModified() {
		t.Errorf("Administration is unedited; the association belongs to MyFirstModule")
	}

	// `marketplace diff` reports them too, without counting them as edits.
	if res := NewDiffResult("Administration", "", "", drift); len(res.Dependents) != 1 || res.LocallyModified {
		t.Errorf("diff result: dependents %v, locallyModified %v", res.Dependents, res.LocallyModified)
	}

	dir := t.TempDir()
	written, unsaved, err := SaveEdits(dir, drift)
	if err != nil {
		t.Fatalf("SaveEdits: %v", err)
	}
	if len(unsaved) != 0 {
		t.Errorf("unsaved = %v", unsaved)
	}
	want := filepath.Join(dir, "association-MyFirstModule_Config_Account.mdl")
	if len(written) != 1 || written[0] != want {
		t.Fatalf("written = %v; want [%s] — this is mendixlabs/mxcli#1502", written, want)
	}
	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "association MyFirstModule.Config_Account") {
		t.Fatalf("saved file does not recreate the association:\n%s", body)
	}

	// The file is only a rescue if it replays: update, then exec it.
	if _, err := PerformUpdate(target, copyFixture(t), emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend); err != nil {
		t.Fatalf("PerformUpdate: %v", err)
	}
	if _, ok := describeAssoc(t, target, crossModuleAssoc); ok {
		t.Fatal("control: the update was expected to remove the association")
	}
	execMDL(t, target, string(body))
	out, ok := describeAssoc(t, target, crossModuleAssoc)
	if !ok {
		t.Fatalf("replaying %s did not restore the association: %s", want, out)
	}
	if !strings.Contains(out, "Administration.Account") {
		t.Errorf("restored association does not reach Administration.Account:\n%s", out)
	}
}
