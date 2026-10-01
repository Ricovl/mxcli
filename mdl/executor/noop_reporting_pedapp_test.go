// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#890 (rehearsal 2, R-rep): re-running a settled script wrote nothing
// but still announced writes — "Granted access …", "Set project security level
// …", "Added module roles …", "Updated … settings", "Moved … to new location" —
// so the output could not serve as an idempotency gate ("a second run reports
// Unchanged", #859). A statement that writes nothing reports Unchanged.
//
// Each statement is run on its own on the Studio Pro-authored PedApp fixture:
// once to apply it (the control: that run is a write and says so), then again,
// when it must write nothing and say Unchanged.
func TestNoopRerun_ReportsUnchanged(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := afRun(t, exec, `create persistent entity MyFirstModule.Rerun890 (Title: String(50));
create microflow MyFirstModule.Rerun890_Flow () begin end;
create module role MyFirstModule.Rerun890Role;`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	// PedApp stores security level Off, demo users on and strict mode off, and
	// the module role is new — so each first run is a write.
	cases := []struct{ stmt, wrote string }{
		{"grant MyFirstModule.User on MyFirstModule.Rerun890 (create, delete, read *, write *);", "Granted access on MyFirstModule.Rerun890"},
		{"alter project security level prototype;", "Set project security level to Prototype"},
		{"alter project security demo users off;", "Demo users disabled"},
		{"alter project security strict mode on;", "Strict mode enabled"},
		{"alter user role User add module roles (MyFirstModule.Rerun890Role);", "Added module roles MyFirstModule.Rerun890Role to user role User"},
		{"alter settings runtime (DecimalScale: 6);", "Updated model settings"},
		{"alter settings configuration 'Default' (HttpPortNumber: 8081);", "Updated configuration 'Default'"},
		{"create or modify configuration 'Default' (HttpPortNumber: 8082);", "Updated configuration 'Default'"},
		{"move microflow MyFirstModule.Rerun890_Flow to folder 'Organised';", "Moved microflow MyFirstModule.Rerun890_Flow"},
	}
	for _, c := range cases {
		out.Reset()
		if err := afRun(t, exec, c.stmt); err != nil {
			t.Fatalf("%s: run 1: %v\n%s", c.stmt, err, out.String())
		}
		if !strings.Contains(out.String(), c.wrote) {
			t.Errorf("%s: run 1 is a write and must say so (%q):\n%s", c.stmt, c.wrote, out.String())
		}

		before := projectFiles(t, dir)
		out.Reset()
		if err := afRun(t, exec, c.stmt); err != nil {
			t.Fatalf("%s: run 2: %v\n%s", c.stmt, err, out.String())
		}
		if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
			t.Errorf("%s: run 2 wrote %v", c.stmt, changed)
		}
		got := out.String()
		if strings.Contains(got, c.wrote) || !strings.Contains(got, "Unchanged ") {
			t.Errorf("%s: run 2 wrote nothing and must report Unchanged, got:\n%s", c.stmt, got)
		}
	}
}

// openPedAppCopy is openPedAppFixture that also says where the copy is.
func openPedAppCopy(t *testing.T) (*Executor, *bytes.Buffer, string) {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyPedAppFile(filepath.Join(src, "PedApp.mpr"), filepath.Join(dir, "PedApp.mpr")); err != nil {
		t.Fatal(err)
	}
	if err := copyPedAppTree(filepath.Join(src, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out, dir
}

// projectFiles hashes every file of the project by content.
func projectFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func diffProjectFiles(a, b map[string]string) []string {
	var out []string
	for p, v := range b {
		if a[p] != v {
			out = append(out, filepath.Base(p))
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			out = append(out, "removed "+filepath.Base(p))
		}
	}
	sort.Strings(out)
	return out
}

// The rehearsal's shape, as a program run: there a run of access-rule
// statements is written once, at its end (#872), so a grant cannot know at the
// time whether it wrote. Its report is held until the run is written.
func TestNoopRerun_ProgramReportsUnchanged(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const script = `mdl 1;
create or modify persistent entity MyFirstModule.Repro_Grant (Title: String(50));
grant MyFirstModule.User on MyFirstModule.Repro_Grant (create, delete, read *, write *);
alter project security level prototype;
create or modify module role MyFirstModule.Repro_Role;
alter user role User add module roles (MyFirstModule.Repro_Role);
revoke MyFirstModule.User on MyFirstModule.Repro_Grant;
grant MyFirstModule.User on MyFirstModule.Repro_Grant (create, delete, read *, write *);
`
	run := func() string {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		out.Reset()
		if err := exec.ExecuteProgram(prog); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return out.String()
	}
	writes := []string{"Granted access", "Revoked access", "Set project security level", "Added module roles"}

	// Control: the first run writes, and says so — except for the closing
	// reset (#872), which re-grants the rule the run already holds: nothing
	// is written for it, and its statements say so even on the first run.
	first := run()
	for _, w := range writes {
		if w == "Revoked access" {
			continue
		}
		if !strings.Contains(first, w) {
			t.Errorf("run 1 is a write and must report %q:\n%s", w, first)
		}
	}
	if strings.Contains(first, "Revoked access") || strings.Count(first, "Granted access") != 1 {
		t.Errorf("run 1: the reset that nets to nothing must not report a write:\n%s", first)
	}
	before := projectFiles(t, dir)
	second := run()
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("run 2 wrote %v", changed)
	}
	for _, w := range writes {
		if strings.Contains(second, w) {
			t.Errorf("run 2 wrote nothing but reported %q:\n%s", w, second)
		}
	}
	if !strings.Contains(second, "already in sync") {
		t.Errorf("run 2 must report its statements as unchanged:\n%s", second)
	}
}
