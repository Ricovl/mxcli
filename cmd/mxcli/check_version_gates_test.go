// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The issue's own script (mendixlabs/mxcli#1434): a module and flows on a
// Mendix 9.24 project. The decision exercises the two properties the reported
// refusal named — Microflow.StableId (10.2+) and SequenceFlow.CaseValues
// (10.15+) — which the flow writers no longer emit below those versions (#1410).
const issue1434Flows = `create module R1434;
create entity R1434.Item ( "Count": integer );
create microflow R1434.MF_Empty () begin return; end;
create microflow R1434.MF_Decide ($Item: R1434.Item) returns boolean
begin
  declare $Result boolean = false;
  if $Item/"Count" > 10 then
    set $Result = true;
  end if;
  return $Result;
end;
`

// The same script plus a statement the project's version cannot store: exec's
// handler refuses `create view entity` below 10.21 (ako/mxcli#1811).
const issue1434Gated = issue1434Flows + `create view entity R1434.ItemView (Name: String(200)) as select cast('a' as String) as Name;
`

// copyV9Project copies the Mendix 9.24 v1 fixture the modelsdk/mpr tests use.
func copyV9Project(t *testing.T) (dir, mpr string) {
	t.Helper()
	src := filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project", "App.mpr")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("Mendix 9 fixture not found: %v", err)
	}
	dir = t.TempDir()
	mpr = filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, mpr
}

// mendixlabs/mxcli#1434: `mxcli check -p` printed "Check passed!" for a script
// that exec, on the same project, refused for the project's Mendix version —
// after the statements before the refused one had run. check -p now applies
// the gates exec's handlers apply (executor.CheckVersionGates), from the same
// statement-to-feature table.
//
// Controls: the gate-free script still passes, and check leaves the project
// as it was.
func TestCheck_RefusesWhatTheProjectVersionCannotStore(t *testing.T) {
	dir, mpr := copyV9Project(t)
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	before := treeDigest(t, dir)
	scripts := t.TempDir()

	var code int
	gated := writeScript(t, scripts, "gated.mdl", issue1434Gated)
	out := captureStd(t, func() { code = runCheckFile(checkCmd, gated) })
	if code == 0 || strings.Contains(out, "Check passed!") {
		t.Errorf("check passed a statement the 9.24 project cannot store (exit %d):\n%s", code, out)
	}
	for _, want := range []string{"statement 5: create view entity requires Mendix 10.21.0+", "MDL-VERSION01"} {
		if !strings.Contains(out, want) {
			t.Errorf("check output must name %q:\n%s", want, out)
		}
	}

	// Control: the flows alone are storable on 9.24 and pass.
	flows := writeScript(t, scripts, "flows.mdl", issue1434Flows)
	out = captureStd(t, func() { code = runCheckFile(checkCmd, flows) })
	if code != 0 || strings.Contains(out, "MDL-VERSION01") {
		t.Errorf("control: a script the 9.24 project can store failed its check (exit %d):\n%s", code, out)
	}

	if n := countDiff(before, treeDigest(t, dir)); n > 0 {
		t.Errorf("check wrote to the project: %d file(s) changed", n)
	}
}

// The other half of #1434: exec agrees with check on the same project. The
// gated script is refused by the pre-flight before anything is written (it used
// to run statements 1-4 and fail on the fifth); the flows alone — the
// statements the reported StableId / CaseValues refusal came from — pass the
// pre-flight and are written.
func TestExecPreflight_AgreesWithCheckOnVersionGates(t *testing.T) {
	_, mpr := copyV9Project(t)
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })

	build := func(script string) *ast.Program {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		return prog
	}
	preflight := func(prog *ast.Program) (string, string) {
		var out bytes.Buffer
		refusal := execPreflight(exec, prog, mpr, "s.mdl", false, false, false, deprecation.Warn, &out, false)
		return refusal, out.String()
	}

	refusal, out := preflight(build(issue1434Gated))
	if refusal == "" || !strings.Contains(refusal, "Nothing was written") {
		t.Errorf("want the pre-flight to refuse the version-gated script, got %q\n%s", refusal, out)
	}
	if !strings.Contains(out, "create view entity requires Mendix 10.21.0+") {
		t.Errorf("pre-flight output must name the gate:\n%s", out)
	}

	prog := build(issue1434Flows)
	if refusal, out := preflight(prog); refusal != "" {
		t.Fatalf("control: the flows were refused by the pre-flight: %s\n%s", refusal, out)
	}
	if err := exec.ExecuteProgram(prog); err != nil {
		t.Errorf("exec refused the flows check passes on Mendix 9.24: %v", err)
	}
}
