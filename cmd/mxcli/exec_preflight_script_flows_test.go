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

// A `create or modify` whose new body calls a flow the statement before it
// creates: exec's preflight said nothing, so exec wrote the new sub-flow and
// then refused the modify (MDL-V1-REBUILD, "$Reporter is still used by"),
// leaving the script half-applied. The preflight must refuse it before
// anything is written.
func TestExecPreflightRefusesModifyCallingFlowTheScriptCreates(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(src, "PedApp.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PedApp.mpr"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, "mprcontents"), os.DirFS(filepath.Join(src, "mprcontents"))); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })

	parse := func(script string) *ast.Program {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		return prog
	}
	if err := exec.ExecuteProgram(parse(`mdl 1;
create microflow MyFirstModule.VerdictAct ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
  log info node 'X' 'done';
end;`)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	run := func(script string) (string, string) {
		t.Helper()
		var out bytes.Buffer
		refusal := execPreflight(exec, parse(script), mpr, "s.mdl", false, false, false, deprecation.Warn, &out, false)
		return refusal, out.String()
	}

	refusal, out := run(`mdl 1;
create microflow MyFirstModule.VerdictNewSub ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
end;
create or modify microflow MyFirstModule.VerdictAct ($F: FeedbackModule.Feedback)
begin
  call microflow MyFirstModule.VerdictNewSub (F = $F);
  log info node 'X' 'done';
end;`)
	if refusal == "" || !strings.Contains(refusal, "Nothing was written") {
		t.Fatalf("want the pre-flight to refuse the script, got %q\n%s", refusal, out)
	}
	if !strings.Contains(out, "is still used by") {
		t.Errorf("pre-flight output must carry exec's refusal:\n%s", out)
	}
	if strings.Contains(out, "not predicted") {
		t.Errorf("the change is predicted, yet reported unpredicted:\n%s", out)
	}

	// Control: a modify that only changes a log message is spliced; the
	// pre-flight lets it run and reports nothing unpredicted.
	refusal, out = run(`mdl 1;
create or modify microflow MyFirstModule.VerdictAct ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
  log info node 'X' 'finished';
end;`)
	if refusal != "" || strings.Contains(out, "not predicted") {
		t.Errorf("control: want the splice accepted silently, got %q\n%s", refusal, out)
	}
}
