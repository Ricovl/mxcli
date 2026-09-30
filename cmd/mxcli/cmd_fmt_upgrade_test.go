// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runFmt(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// cobra keeps flag values between runs in one process.
	for _, f := range []string{"write", "upgrade", "header"} {
		_ = fmtCmd.Flags().Set(f, "false")
		fmtCmd.Flags().Lookup(f).Changed = false
	}
	_ = rootCmd.PersistentFlags().Set("project", "")
	rootCmd.PersistentFlags().Lookup("project").Changed = false
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"fmt"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestFmtUpgrade_WritesOnlyTheRewrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "-- keep me\nCREATE OR REPLACE entity M.User (Name: String);\n   show entities;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "-w", path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	// Only the deprecated keywords change: no heuristic upper-casing (which
	// would turn M.User into M.USER), no re-indentation, and no header while
	// mdl 1 is a preview.
	want := "-- keep me\nCREATE OR MODIFY entity M.User (Name: String);\n   list entities;\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if _, err := runFmt(t, "--upgrade", "--header", "-w", path); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(got), "mdl 1;\n-- keep me\n") {
		t.Fatalf("--header did not add the header:\n%s", got)
	}

	// Idempotent: a second run leaves the file as it is.
	before := string(got)
	if _, err := runFmt(t, "--upgrade", "--header", "-w", path); err != nil {
		t.Fatal(err)
	}
	if got, _ = os.ReadFile(path); string(got) != before {
		t.Fatalf("second fmt --upgrade changed the file:\n%s", got)
	}
}

func TestFmtUpgrade_HeaderNeedsUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	if err := os.WriteFile(path, []byte("show entities;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--header", path); err == nil || !strings.Contains(err.Error(), "--upgrade") {
		t.Fatalf("--header without --upgrade: %v", err)
	}
}

// A construct with no mechanical rewrite is reported with its reason, the
// header is refused, and the file is left as it was.
func TestFmtUpgrade_ReportsWhatItCannotRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.mdl")
	src := "create microflow M.F ($L: List of M.E) begin\n  $n = count(filter($L, Name = 'x'));\nend\n/\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runFmt(t, "--upgrade", "--header", "-w", path)
	if err == nil || !strings.Contains(err.Error(), "MDL-V1-LIST") || !strings.Contains(err.Error(), "nested call") {
		t.Fatalf("want the nested list operation reported with its reason, got %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != src {
		t.Fatalf("the file was changed although the header was refused:\n%s", got)
	}
}

// A .test.mdl that check accepts is one fmt --upgrade can read (ako/mxcli#837):
// the bodies are upgraded, the @test / @expect doc comments are kept verbatim,
// and --header adds no header to a test file, saying why.
func TestFmtUpgrade_TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "csv-import.test.mdl")
	src := "-- tests\n\n/**\n * @test Head of the rows\n * @expect $first/Merchant = 'Albert Heijn'\n */\n" +
		"retrieve $rows from Ledger.ImportRow where Batch = 't' limit 1;\n$first = head($rows);\n/\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "-w", path); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(src, "head($rows)", "head $rows", 1)
	if got, _ := os.ReadFile(path); string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	out, err := runFmt(t, "--upgrade", "--header", "-w", path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != want {
		t.Fatalf("--header changed the test file:\n%s", got)
	}
	if !strings.Contains(out, "no language header") {
		t.Errorf("--header on a test file said nothing about the header:\n%s", out)
	}

	// Plain fmt does not format a test file: it says what does.
	if _, err := runFmt(t, path); err == nil || !strings.Contains(err.Error(), "--upgrade") {
		t.Fatalf("plain fmt on a test file: %v", err)
	}
}

// `find(…)` over a flow call's result: without a project fmt cannot tell the
// string function from the List operation and refuses the header, saying to
// pass the project; with -p it reads what the called flow returns there and
// rewrites (ako/mxcli#860). PedApp's flows are Studio Pro-authored.
func TestFmtUpgrade_FindOnACallResultReadsTheProject(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")

	path := filepath.Join(t.TempDir(), "s.mdl")
	const script = "create microflow MyFirstModule.F ($o: Administration.Account) begin\n" +
		"  declare $Pos Integer = 0;\n" +
		"  $Url = call microflow FeedbackModule.ConvertUUIDToURL(uuid = 'x');\n" +
		"  $Pos = find($Url, 'x');\n" +
		"  $Ctx = call nanoflow Atlas_Web_Content.DS_LoginContext();\n" +
		"  $Hit = find($Ctx, $currentObject = $o);\n" +
		"end;\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runFmt(t, "--upgrade", "--header", "-w", path)
	if err == nil || !strings.Contains(err.Error(), "pass the project") {
		t.Fatalf("without -p: want the header refused, asking for the project; got %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); string(got) != script {
		t.Fatalf("a refused upgrade wrote the file:\n%s", got)
	}

	if out, err := runFmt(t, "--upgrade", "--header", "-w", "-p", mpr, path); err != nil {
		t.Fatalf("with -p: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(path)
	for _, want := range []string{
		"mdl 1;\n",
		"\n  set $Pos = find($Url, 'x');\n", // ConvertUUIDToURL returns a String
		"\n  $Hit = find $Ctx where $currentObject = $o;\n", // DS_LoginContext does not
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
