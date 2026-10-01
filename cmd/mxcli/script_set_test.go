// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// writeScript writes src to dir/name and returns the path.
func writeScript(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// captureStd runs fn with os.Stdout and os.Stderr captured, returning both.
func captureStd(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	defer func() { os.Stdout, os.Stderr = so, se }()
	fn()
	_ = w.Close()
	os.Stdout, os.Stderr = so, se
	return <-done
}

const (
	setStub  = "-- the placeholder\ncreate or modify microflow MyFirstModule.Export ()\nbegin\n  log info node 'X' 'stub';\nend;\n"
	setReal  = "-- the real flow\ncreate or modify microflow MyFirstModule.Export ()\nbegin\n  log info node 'X' 'real';\n  log info node 'X' 'done';\nend;\n"
	setOther = "create or modify microflow MyFirstModule.Other ()\nbegin\n  log info node 'X' 'other';\nend;\n"
)

func TestFindFlowRedeclarations(t *testing.T) {
	parse := func(path, src string) setScript {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		return setScript{Path: path, Source: src, Prog: prog}
	}
	twice := "create or modify nanoflow M.N () begin end;\n\nCREATE OR REPLACE NANOFLOW M.\"N\" () begin end;\n"
	got := findFlowRedeclarations([]setScript{
		parse("a.mdl", setStub),
		parse("b.mdl", "mdl 1;\n"+setReal),
		parse("c.mdl", setOther),
		parse("d.mdl", twice),
		parse("e.mdl", "create microflow M.Plain () begin end;\n"),
		parse("f.mdl", "create microflow M.Plain () begin end;\n"),
	})
	if len(got) != 2 {
		t.Fatalf("want MyFirstModule.Export and M.N, got %+v", got)
	}
	if r := got[0]; r.Flow != "MyFirstModule.Export" || len(r.Sites) != 2 ||
		r.Sites[0].String() != "a.mdl:2" || r.Sites[1].String() != "b.mdl:3" || !r.mixedVersions() {
		t.Errorf("the cross-file pair: %+v", r)
	}
	if r := got[1]; r.Flow != "M.N" || len(r.Sites) != 2 || r.Sites[0].String() != "d.mdl:1" ||
		r.Sites[1].String() != "d.mdl:3" || r.mixedVersions() || !r.Sites[0].Nanoflow {
		t.Errorf("the in-file pair: %+v", r)
	}
	// One file declaring a flow twice has one header: nothing to decide together.
	groups := fileGroups([]string{"a.mdl", "b.mdl", "c.mdl", "d.mdl"}, got)
	if len(groups) != 1 || strings.Join(groups[0], ",") != "a.mdl,b.mdl" {
		t.Errorf("file groups: %v", groups)
	}
}

// ako/mxcli#905: `check` over a script set warns when two of its `create or
// modify` statements declare one flow — a stub-then-real pair — naming both
// and saying to drop the stub. Controls: a set without a redeclaration, and
// one file alone (single-file behaviour is unchanged).
func TestCheck_ScriptSetWarnsOnStubThenReal(t *testing.T) {
	// cobra keeps flag values between runs in one process: no project, text.
	_ = rootCmd.PersistentFlags().Set("project", "")
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	dir := t.TempDir()
	stub := writeScript(t, dir, "1-stub.mdl", setStub)
	real := writeScript(t, dir, "2-real.mdl", setReal)
	other := writeScript(t, dir, "3-other.mdl", setOther)

	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{stub, real}) })
	if code != 0 {
		t.Fatalf("a warning failed the run (code %d):\n%s", code, out)
	}
	for _, want := range []string{StubThenRealRule, "MyFirstModule.Export", stub + ":2", real + ":2", "drop the stub", "#843"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in check's output:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Checking syntax: "+stub) || !strings.Contains(out, "Checking syntax: "+real) {
		t.Errorf("each file is still checked:\n%s", out)
	}

	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{stub, other}) })
	if code != 0 || strings.Contains(out, StubThenRealRule) {
		t.Errorf("control, no redeclaration: code %d\n%s", code, out)
	}
	both := writeScript(t, dir, "both.mdl", setStub+setReal)
	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{both}) })
	if code != 0 || strings.Contains(out, StubThenRealRule) {
		t.Errorf("control, one file: code %d\n%s", code, out)
	}
}

// ako/mxcli#905: `fmt --upgrade -p` over a stub-then-real script set decided
// the header file by file: the stub's file was declined it (exec would refuse
// its change to the stored real flow) and the real flow's file took it. Run in
// order, the mdl 0 stub then rebuilt the real flow and the mdl 1 real
// statement was refused, every run. Over the set, the header is decided for
// the pair together: here neither file takes it, and the pair is reported.
// Controls: the real file alone takes the header (single-file behaviour is
// unchanged), and so does it in a set without a redeclaration.
func TestFmtUpgrade_ScriptSetDecidesStubThenRealHeaderTogether(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	// The stored real flow: a loop body the stub's statement would change,
	// which the splice cannot do, so the stub is refused under mdl 1.
	const real = "create or modify microflow MyFirstModule.Grow ($Items: List of System.User)\nreturns String as $Out\nbegin\n  declare $Out String = '';\n" +
		"  loop $U in $Items\n  begin\n    set $Out = $Out + ',';\n    set $Out = $Out + $U/Name;\n  end loop;\n  return $Out;\nend;\n"
	const stub = "create or modify microflow MyFirstModule.Grow ($Items: List of System.User)\nreturns String as $Out\nbegin\n  declare $Out String = '';\n  loop $U in $Items\n  begin\n    set $Out = $Out + ',';\n  end loop;\n  return $Out;\nend;\n"
	exe := executor.New(io.Discard)
	exe.SetBackendFactory(newBackendFactory())
	prog, errs := visitor.Build(fmt.Sprintf("connect local '%s';\n%s", visitor.QuoteString(mpr), real))
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("store the real flow: %v", err)
	}
	_ = exe.Close()

	// Control: the real file alone takes the header.
	alone := writeScript(t, t.TempDir(), "real.mdl", real)
	if out, err := runFmt(t, "--upgrade", "-w", "-p", mpr, alone); err != nil {
		t.Fatalf("the real file alone: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(alone); !strings.HasPrefix(string(got), "mdl 1;\n") {
		t.Fatalf("control: the real file alone did not take the header:\n%s", got)
	}

	// Control: in a set without a redeclaration it takes it too.
	cdir := t.TempDir()
	creal := writeScript(t, cdir, "2-real.mdl", real)
	cother := writeScript(t, cdir, "3-other.mdl", setOther)
	out, err := runFmt(t, "--upgrade", "-w", "-p", mpr, creal, cother)
	if err != nil {
		t.Fatalf("control set: %v\n%s", err, out)
	}
	if strings.Contains(out, StubThenRealRule) {
		t.Errorf("control set reported a pair:\n%s", out)
	}
	for _, p := range []string{creal, cother} {
		if got, _ := os.ReadFile(p); !strings.HasPrefix(string(got), "mdl 1;\n") {
			t.Errorf("control set: %s did not take the header:\n%s", p, got)
		}
	}

	sdir := t.TempDir()
	sstub := writeScript(t, sdir, "1-stub.mdl", stub)
	sreal := writeScript(t, sdir, "2-real.mdl", real)
	out, err = runFmt(t, "--upgrade", "-w", "-p", mpr, sstub, sreal)
	if err != nil {
		t.Fatalf("the stub-then-real set: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(sstub); string(got) != stub {
		t.Errorf("the stub took the header exec would refuse it under:\n%s", got)
	}
	if got, _ := os.ReadFile(sreal); string(got) != real {
		t.Errorf("the pair was split: the real file took the header its stub cannot take:\n%s", got)
	}
	for _, want := range []string{StubThenRealRule, "MyFirstModule.Grow", sstub + ":1", sreal + ":1", "decided for the files together", "drop the stub"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in fmt's report:\n%s", want, out)
		}
	}

	// --force-header overrides the refusal for both: the pair stays together.
	out, err = runFmt(t, "--upgrade", "--force-header", "-w", "-p", mpr, sstub, sreal)
	if err != nil {
		t.Fatalf("--force-header: %v\n%s", err, out)
	}
	for _, p := range []string{sstub, sreal} {
		if got, _ := os.ReadFile(p); !strings.HasPrefix(string(got), "mdl 1;\n") {
			t.Errorf("--force-header: %s did not take the header:\n%s", p, got)
		}
	}
}
