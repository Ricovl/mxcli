// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
)

const defaultSpellings = `mdl 1;
create or modify persistent entity MyFirstModule.Repro_DiffDefaults (
  Success: Boolean,
  Body: String
);
`

func render(r *Report) string {
	var b bytes.Buffer
	r.Write(&b, executor.DiffOptions{}, false)
	return b.String()
}

// ako/mxcli#907: once exec has run a script, a second exec writes nothing, and
// diff must say so. It reported the entity as modified because it compared the
// script's implicit spellings (`Boolean`, `String`) with the stored defaults
// described explicitly (`Boolean default false`, `String(unlimited)`).
func TestDiffOfAnAppliedScriptIsEmpty(t *testing.T) {
	mpr := pedAppCopy(t)
	prog := parse(t, defaultSpellings)

	// Control: on the project without the entity, diff and exec both add it.
	rep, diffSet, execSet, out, err := diffThenExec(t, mpr, "", prog, false)
	if err != nil {
		t.Fatalf("exec 1: %v\n%s", err, out)
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec 1:\n  %s", strings.Join(d, "\n  "))
	}
	if got := render(rep); !strings.Contains(got, "+++ Entity.MyFirstModule.Repro_DiffDefaults (new)") ||
		!strings.Contains(got, "Summary: 1 new, 0 modified, 0 removed") {
		t.Errorf("diff before exec 1 does not show the new entity:\n%s", got)
	}

	// The property: exec 2 writes nothing, and diff says nothing changes.
	rep, diffSet, execSet, out, err = diffThenExec(t, mpr, "", prog, false)
	if err != nil {
		t.Fatalf("exec 2: %v\n%s", err, out)
	}
	if len(execSet) != 0 {
		t.Fatalf("exec 2 wrote, so this is no test of diff:\n  %s", strings.Join(execSet, "\n  "))
	}
	if len(diffSet) != 0 {
		t.Errorf("diff reports writes exec does not make:\n  %s\n%s", strings.Join(diffSet, "\n  "), render(rep))
	}
	if got := render(rep); !strings.Contains(got, "exec would write nothing") || strings.Contains(got, "Success") {
		t.Errorf("diff of an applied script:\n%s", got)
	}
}

// ako/mxcli#807: a plain create of an element that exists is refused by exec,
// and was reported by diff as unchanged.
func TestDiffOfAPlainCreateOfAnExistingElementIsRefused(t *testing.T) {
	mpr := pedAppCopy(t)
	if _, err := execScript(t, mpr, "", parse(t, defaultSpellings), false); err != nil {
		t.Fatal(err)
	}
	plain := parse(t, "mdl 1;\ncreate persistent entity MyFirstModule.Repro_DiffDefaults (\n  Success: Boolean\n);\n")
	rep, diffSet, execSet, _, execErr := diffThenExec(t, mpr, "", plain, false)
	if execErr == nil || !strings.Contains(execErr.Error(), "already exists") {
		t.Fatalf("exec of the plain create: got %v, want already exists", execErr)
	}
	if rep.ExecErr == nil || rep.ExecErr.Error() != execErr.Error() {
		t.Errorf("diff's error = %v, exec's = %v", rep.ExecErr, execErr)
	}
	if got := render(rep); !strings.Contains(got, "Refused: exec would stop at this error, having written nothing: entity already exists") {
		t.Errorf("diff of the plain create:\n%s", got)
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec:\n  %s", strings.Join(d, "\n  "))
	}
}

// ako/mxcli#856: diff ran none of a script's earlier statements, so a flow
// that calls one the script creates first could not be diffed.
func TestDiffRunsEarlierStatements(t *testing.T) {
	mpr := pedAppCopy(t)
	prog := parse(t, `mdl 1;
create microflow MyFirstModule.Repro_Callee () returns Boolean
begin
  return true;
end;
create or modify microflow MyFirstModule.Repro_Caller () returns Boolean
begin
  $R = call microflow MyFirstModule.Repro_Callee();
  return $R;
end;
`)
	rep, diffSet, execSet, out, err := diffThenExec(t, mpr, "", prog, false)
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	if rep.ExecErr != nil {
		t.Errorf("diff: %v", rep.ExecErr)
	}
	got := render(rep)
	for _, want := range []string{"Microflow.MyFirstModule.Repro_Callee (new)", "Microflow.MyFirstModule.Repro_Caller (new)",
		"call microflow MyFirstModule.Repro_Callee()", "Summary: 2 new, 0 modified, 0 removed"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff does not show %q:\n%s", want, got)
		}
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec:\n  %s", strings.Join(d, "\n  "))
	}
}

// ako/mxcli#907: pages were not compared at all. A page whose layout is
// repointed is one written unit, rendered as the layout line that changes.
func TestDiffShowsAPageLayoutChange(t *testing.T) {
	mpr := pedAppCopy(t)
	// Control: a script that reads only writes nothing.
	rep0, err := Run(mpr, parse(t, "show modules;"), Options{NewBackend: fileEngine})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep0.Units) != 0 {
		t.Fatalf("a read-only script writes %d unit(s)", len(rep0.Units))
	}

	page := describe(t, mpr, "page MyFirstModule.Home_Web")
	if !strings.Contains(page, "Layout: Atlas_Core.Atlas_TopBar") {
		t.Fatalf("the fixture's home page no longer uses Atlas_TopBar:\n%s", page)
	}
	moved := strings.Replace(page, "Layout: Atlas_Core.Atlas_TopBar", "Layout: Atlas_Core.Atlas_Default", 1)
	rep, diffSet, execSet, out, err := diffThenExec(t, mpr, "", parse(t, moved), false)
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec:\n  %s", strings.Join(d, "\n  "))
	}
	got := render(rep)
	if !strings.Contains(got, "-  Layout: Atlas_Core.Atlas_TopBar,") || !strings.Contains(got, "+  Layout: Atlas_Core.Atlas_Default,") {
		t.Errorf("diff does not show the layout change:\n%s", got)
	}
}

func describe(t *testing.T, mpr, target string) string {
	t.Helper()
	var out bytes.Buffer
	x := executor.New(&out)
	x.SetBackendFactory(fileEngine)
	defer x.Close()
	for _, stmt := range parse(t, "connect local '"+mpr+"'; describe "+target+";").Statements {
		if err := x.Execute(stmt); err != nil {
			t.Fatalf("describe %s: %v", target, err)
		}
	}
	text := out.String()
	if i := strings.Index(text, "mdl 1;"); i >= 0 {
		text = text[i:]
	}
	return text
}

// A statement that only moves a document to another folder rewrites no unit
// bytes: the move is a row of the .mpr's Unit table. diff must still report it.
func TestDiffReportsAMoveToAnotherFolder(t *testing.T) {
	mpr := pedAppCopy(t)
	create := parse(t, "mdl 1;\ncreate microflow MyFirstModule.Repro_Mover () returns Boolean\nbegin\n  return true;\nend;\n")
	if _, err := execScript(t, mpr, "", create, false); err != nil {
		t.Fatal(err)
	}
	move := parse(t, "mdl 1;\nmove microflow MyFirstModule.Repro_Mover to folder 'Elsewhere';\n")
	rep, diffSet, execSet, out, err := diffThenExec(t, mpr, "", move, false)
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
		t.Errorf("diff disagrees with exec:\n  %s", strings.Join(d, "\n  "))
	}
	if got := render(rep); !strings.Contains(got, "Microflow MyFirstModule.Repro_Mover") || !strings.Contains(got, "MyFirstModule/Elsewhere") {
		t.Errorf("diff does not report the move:\n%s", got)
	}
	// Control: the same move again writes nothing, and diff agrees.
	rep, diffSet, execSet, _, _ = diffThenExec(t, mpr, "", move, false)
	if len(execSet) != 0 || len(diffSet) != 0 {
		t.Errorf("a repeated move: exec wrote %v, diff reported %v\n%s", execSet, diffSet, render(rep))
	}
}
