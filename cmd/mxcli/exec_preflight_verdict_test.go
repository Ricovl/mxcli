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

// exec stopped at the first flow change it refuses ("cannot be spliced") with
// the statements before it already written and the ones after it not, while
// `check -p` had predicted the refusal. The pre-flight now runs the same
// verdict, so the script is refused before anything is written.
func TestExecPreflightRefusesAFlowChangeExecWouldRefuse(t *testing.T) {
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

	const flow = `mdl 1;
create microflow MyFirstModule.LoopFlow ($Items: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    commit $It;
  end loop;
end;`
	setup, errs := visitor.Build(flow)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := exec.ExecuteProgram(setup); err != nil {
		t.Fatalf("setup: %v", err)
	}

	changed := strings.Replace(strings.Replace(flow, "create microflow", "create or modify microflow", 1),
		"commit $It;", "commit $It without events;", 1)
	script := "mdl 1;\ncreate microflow MyFirstModule.Before () begin end;\n" +
		strings.TrimPrefix(changed, "mdl 1;\n") + "\ncreate microflow MyFirstModule.After () begin end;\n"
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}

	var out bytes.Buffer
	refusal := execPreflight(exec, prog, mpr, "s.mdl", false, false, false, deprecation.Warn, &out, false)
	if refusal == "" || !strings.Contains(refusal, "Nothing was written") {
		t.Fatalf("want the pre-flight to refuse the script, got %q\n%s", refusal, out.String())
	}
	if !strings.Contains(out.String(), "replace loop") {
		t.Errorf("the refusal's reason is not printed:\n%s", out.String())
	}

	// Control: --continue-on-error asks for every statement that can run to
	// run, so the verdict is printed and the script is not refused.
	out.Reset()
	if refusal := execPreflight(exec, prog, mpr, "s.mdl", false, false, true, deprecation.Warn, &out, false); refusal != "" {
		t.Fatalf("--continue-on-error must not be refused up front: %s", refusal)
	}
}
