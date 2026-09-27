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
