// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// --v1 is to the MDL-V1-* warnings what --deprecations is to MDL-DEPRnnn: a
// headerless (mdl 0) script that would not parse under `mdl 1;` warns by
// default, and fails the run under --v1=error (ako/mxcli-sudoku FINDINGS #64).
// Without it a project could pass --deprecations=error and still not be ready
// for the header, with no gate saying so.

// slashScript is headerless and ends a statement with the SQL*Plus `/` line:
// MDL-V1-SLASH under mdl 0, a syntax error under mdl 1.
const slashScript = "create module V1Gate;\n/\n"

// headedScript is the same statement written for mdl 1.
const headedScript = "mdl 1;\ncreate module V1Gate;\n"

func checkWithV1(t *testing.T, value, src string) (int, string) {
	t.Helper()
	if value != "" {
		if err := checkCmd.Flags().Set("v1", value); err != nil {
			t.Fatalf("set --v1=%s: %v", value, err)
		}
		defer func() {
			_ = checkCmd.Flags().Set("v1", "warn")
			checkCmd.Flags().Lookup("v1").Changed = false
		}()
	}
	file := writeScript(t, t.TempDir(), "s.mdl", src)
	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{file}) })
	return code, out
}

func TestCheckV1WarnsByDefault(t *testing.T) {
	code, out := checkWithV1(t, "", slashScript)
	if code != 0 {
		t.Errorf("a headerless script failed by default (exit %d); MDL-V1-* must only warn:\n%s", code, out)
	}
	if !strings.Contains(out, "MDL-V1-SLASH") {
		t.Errorf("MDL-V1-SLASH not reported:\n%s", out)
	}
}

func TestCheckV1ErrorFailsTheRun(t *testing.T) {
	code, out := checkWithV1(t, "error", slashScript)
	if code == 0 {
		t.Errorf("--v1=error passed a headerless script with a `/` terminator:\n%s", out)
	}
	if !strings.Contains(out, "MDL-V1-SLASH") {
		t.Errorf("the failing run must name the code:\n%s", out)
	}
	if !strings.Contains(out, "1 error") {
		t.Errorf("the summary must count the promoted warning as an error:\n%s", out)
	}
}

// The control: a script already headed `mdl 1;` reports no MDL-V1-* warning,
// so --v1=error does not fail everything.
func TestCheckV1ErrorPassesAHeadedScript(t *testing.T) {
	code, out := checkWithV1(t, "error", headedScript)
	if code != 0 || strings.Contains(out, "MDL-V1-") {
		t.Errorf("an `mdl 1;` script failed --v1=error (exit %d):\n%s", code, out)
	}
}

// --deprecations=error does not gate MDL-V1-*: the two flags gate different
// things, which is why both exist.
func TestCheckDeprecationsErrorDoesNotGateV1(t *testing.T) {
	if err := checkCmd.Flags().Set("deprecations", "error"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = checkCmd.Flags().Set("deprecations", "warn")
		checkCmd.Flags().Lookup("deprecations").Changed = false
	}()
	code, out := checkWithV1(t, "", slashScript)
	if code != 0 {
		t.Errorf("--deprecations=error failed on an MDL-V1-* warning (exit %d):\n%s", code, out)
	}
}

func TestParseV1PolicyRejectsUnknownValue(t *testing.T) {
	for in, want := range map[string]deprecation.Policy{"": deprecation.Warn, "warn": deprecation.Warn, "ERROR": deprecation.Error} {
		got, err := parseV1Policy(in)
		if err != nil || got != want {
			t.Errorf("parseV1Policy(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	_, err := parseV1Policy("eror")
	if err == nil {
		t.Fatal("a typo must be refused, not read as warn: a CI gate meant to fail would pass")
	}
	if !strings.Contains(err.Error(), "--v1") {
		t.Errorf("the error must name the flag: %v", err)
	}
}
