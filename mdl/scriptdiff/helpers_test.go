// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func fileEngine() backend.FullBackend { return modelsdkbackend.New() }

// pedAppCopy copies the committed PedApp fixture (Studio Pro-authored) into a
// temporary folder and returns its .mpr.
func pedAppCopy(t testing.TB) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "PedApp")
	if err := copyProject(src, dst); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dst, "PedApp.mpr")
}

func parse(t testing.TB, script string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], script)
	}
	return prog
}

// execScript runs a script the way `mxcli exec` does after its pre-flight: a
// fresh executor, connected to the project, running the program.
func execScript(t testing.TB, mpr, scriptDir string, prog *ast.Program, continueOnError bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	x := executor.New(&out)
	x.SetBackendFactory(fileEngine)
	if scriptDir != "" {
		x.SetScriptDir(scriptDir)
	}
	defer x.Close()
	if err := x.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	var err error
	if continueOnError {
		var fails bytes.Buffer
		_, err = x.ExecuteProgramContinueOnError(prog, &fails)
		out.WriteString(fails.String())
	} else {
		err = x.ExecuteProgram(prog)
	}
	if errors.Is(err, executor.ErrExit) {
		err = nil
	}
	_ = x.Execute(&ast.DisconnectStmt{})
	return out.String(), err
}

func snap(t testing.TB, mpr string) *Snapshot {
	t.Helper()
	s, err := TakeSnapshot(mpr)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// writeSet is a run's writes as comparable lines: "<kind> <unit id> <type>",
// and "file <kind> <path>".
func writeSet(units []UnitChange, files []FileChange) []string {
	var out []string
	for _, u := range units {
		k := string(u.Kind)
		if u.Kind == Modified && !u.Rewritten {
			k = "moved"
		}
		out = append(out, fmt.Sprintf("%s %s %s %s", k, u.ID, u.Type, u.Name))
	}
	for _, f := range files {
		out = append(out, fmt.Sprintf("file %s %s", f.Kind, f.Path))
	}
	sort.Strings(out)
	return out
}

// symmetricDiff lists the lines only in a (prefixed "diff only: ") and only in
// b ("exec only: ").
func symmetricDiff(a, b []string) []string {
	in := func(s []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range s {
			m[x] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	var out []string
	for _, x := range a {
		if !mb[x] {
			out = append(out, "diff only: "+x)
		}
	}
	for _, x := range b {
		if !ma[x] {
			out = append(out, "exec only: "+x)
		}
	}
	return out
}

// diffThenExec runs diff on the project, checks it wrote nothing, then runs
// exec on it, and returns what each said is written. New units get fresh IDs
// in each run, so an added unit is compared by type and name only.
func diffThenExec(t testing.TB, mpr, scriptDir string, prog *ast.Program, continueOnError bool) (rep *Report, diffSet, execSet []string, execOut string, execErr error) {
	t.Helper()
	before := snap(t, mpr)
	rep, err := Run(mpr, prog, Options{NewBackend: fileEngine, ScriptDir: scriptDir, ContinueOnError: continueOnError})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if u, f := before.Compare(snap(t, mpr)); len(u)+len(f) > 0 {
		t.Fatalf("diff wrote to the project it diffs:\n  %s", strings.Join(writeSet(u, f), "\n  "))
	}
	execOut, execErr = execScript(t, mpr, scriptDir, prog, continueOnError)
	u, f := before.Compare(snap(t, mpr))
	return rep, comparable(writeSet(rep.Units, rep.Files)), comparable(writeSet(u, f)), execOut, execErr
}

// comparable drops the unit ID of an added unit, which each run mints afresh.
func comparable(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, "added ") {
			parts := strings.SplitN(l, " ", 3)
			l = "added " + parts[2]
		}
		out[i] = l
	}
	sort.Strings(out)
	return out
}

// parseOK parses a script, returning its first error instead of failing.
func parseOK(script string) (*ast.Program, error) {
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return prog, nil
}
