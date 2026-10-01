// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A headerless (mdl 0) script may CONNECT, and exec follows it. diff runs the
// script for real, so a connect it followed would write to the project being
// diffed — while reporting that exec writes nothing, because the copy it
// compares saw no write. A connect to the project itself is followed on the
// copy instead.
func TestDiffFollowsAConnectToTheProjectOntoTheCopy(t *testing.T) {
	mpr := pedAppCopy(t)
	prog := parse(t, "connect local '"+mpr+"';\n"+
		"create persistent entity MyFirstModule.DiffConnectEscape (Name: String(50));\n")

	// diffThenExec fails the test if diff writes to the project.
	rep, diffSet, execSet, out, err := diffThenExec(t, mpr, "", prog, false)
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	if len(execSet) == 0 {
		t.Fatal("exec wrote nothing, so this is no test of diff")
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec:\n  %s\n%s", strings.Join(d, "\n  "), render(rep))
	}
}

// A connect to any other project is refused: diff has no copy of it.
func TestDiffRefusesAConnectToAnotherProject(t *testing.T) {
	mpr := pedAppCopy(t)
	other := pedAppCopy(t)
	before := snap(t, other)
	prog := parse(t, "connect local '"+other+"';\n"+
		"create persistent entity MyFirstModule.DiffConnectEscape (Name: String(50));\n")
	_, err := Run(mpr, prog, Options{NewBackend: fileEngine})
	if err == nil || !strings.Contains(err.Error(), "outside the scratch copy") {
		t.Errorf("diff of a connect to another project: err = %v, want a refusal", err)
	}
	if u, f := before.Compare(snap(t, other)); len(u)+len(f) > 0 {
		t.Fatalf("diff wrote to the other project:\n  %s", strings.Join(writeSet(u, f), "\n  "))
	}
}

// The same holds for a connect inside a script the script executes.
func TestDiffRefusesAConnectInANestedScript(t *testing.T) {
	mpr := pedAppCopy(t)
	other := pedAppCopy(t)
	before := snap(t, other)
	dir := t.TempDir()
	nested := "connect local '" + other + "';\n" +
		"create persistent entity MyFirstModule.DiffConnectEscape (Name: String(50));\n"
	if err := os.WriteFile(filepath.Join(dir, "nested.mdl"), []byte(nested), 0o644); err != nil {
		t.Fatal(err)
	}
	prog := parse(t, "execute script 'nested.mdl';\n")
	_, err := Run(mpr, prog, Options{NewBackend: fileEngine, ScriptDir: dir})
	if err == nil || !strings.Contains(err.Error(), "outside the scratch copy") {
		t.Errorf("diff of a nested connect: err = %v, want a refusal", err)
	}
	if u, f := before.Compare(snap(t, other)); len(u)+len(f) > 0 {
		t.Fatalf("diff wrote to the other project:\n  %s", strings.Join(writeSet(u, f), "\n  "))
	}
}

// SQL queries and IMPORT act on databases, not on the project: diff cannot
// run them on a copy, and refuses rather than running them for real.
func TestDiffRefusesStatementsThatActOnADatabase(t *testing.T) {
	for name, stmt := range map[string]ast.Statement{
		"sql query": &ast.SQLQueryStmt{Alias: "db", Query: "delete from customers"},
		"import":    &ast.ImportStmt{SourceAlias: "db", Query: "select 1", TargetEntity: "MyFirstModule.Customer"},
	} {
		t.Run(name, func(t *testing.T) {
			mpr := pedAppCopy(t)
			prog := &ast.Program{Statements: []ast.Statement{stmt}}
			_, err := Run(mpr, prog, Options{NewBackend: fileEngine})
			if err == nil || !strings.Contains(err.Error(), "outside the scratch copy") {
				t.Errorf("err = %v, want a refusal", err)
			}
		})
	}
}

// The scratch folder may sit inside the project folder (TMPDIR=./tmp in the
// project); the copy must not copy itself into itself.
func TestDiffWithTheScratchFolderInsideTheProject(t *testing.T) {
	mpr := pedAppCopy(t)
	tmp := filepath.Join(filepath.Dir(mpr), "tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	prog := parse(t, defaultSpellings)
	rep, err := Run(mpr, prog, Options{NewBackend: fileEngine})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if got := render(rep); !strings.Contains(got, "Summary: 1 new, 0 modified, 0 removed") {
		t.Errorf("diff:\n%s", got)
	}
}
