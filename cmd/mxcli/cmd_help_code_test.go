// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetCmdFlags(syntaxCmd)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

// `mxcli help <code>` prints the registry entry the warning pointed at:
// old form, new form, rewrite, refused from (ako/mxcli#714 decision 3).
func TestHelpCode_Deprecation(t *testing.T) {
	out, err := runRoot(t, "help", "mdl-depr001")
	if err != nil {
		t.Fatalf("mxcli help mdl-depr001: %v\n%s", err, out)
	}
	e, _ := deprecation.Lookup(deprecation.CreateOrReplace)
	for _, want := range []string{"MDL-DEPR001", "Old form:", e.Old, "New form:", e.Canonical, "Rewrite:", "yes:", "Refused from:", "mdl 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
}

func TestHelpCode_LanguageChange(t *testing.T) {
	out, err := runRoot(t, "help", "MDL-V1-LIMIT1")
	if err != nil {
		t.Fatalf("mxcli help MDL-V1-LIMIT1: %v\n%s", err, out)
	}
	for _, want := range []string{"MDL-V1-LIMIT1", "Before mdl 1:", "From mdl 1:", "Rewrite:", "fmt --upgrade --header"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	// An exec-time change is answered too, though fmt cannot rewrite it.
	out, err = runRoot(t, "help", "MDL-V1-REBUILD")
	if err != nil || !strings.Contains(out, "check -p") {
		t.Errorf("mxcli help MDL-V1-REBUILD: err=%v\n%s", err, out)
	}
}

func TestHelpCode_UnknownCodeFails(t *testing.T) {
	out, err := runRoot(t, "help", "MDL-DEPR999")
	if err == nil || !strings.Contains(err.Error(), "unknown code MDL-DEPR999") {
		t.Fatalf("want an unknown-code error, got err=%v\n%s", err, out)
	}
}

// The control: `help <command>` still prints the command's help, as cobra's
// built-in did.
func TestHelpCode_CommandHelpUnchanged(t *testing.T) {
	out, err := runRoot(t, "help", "fmt")
	if err != nil {
		t.Fatalf("mxcli help fmt: %v", err)
	}
	if !strings.Contains(out, "Format an MDL file") && !strings.Contains(out, "--upgrade") {
		t.Errorf("help fmt did not print fmt's help:\n%s", firstLines(out, 5))
	}
}

// `mxcli syntax <topic> --deprecated` lists the topic's old spellings, and
// only those; without a topic it lists every one.
func TestSyntaxDeprecated(t *testing.T) {
	out, err := runRoot(t, "syntax", "page", "action", "--deprecated")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, deprecation.PageActionWord) || !strings.Contains(out, "show_page") {
		t.Errorf("page action --deprecated lacks MDL-DEPR020:\n%s", out)
	}
	if strings.Contains(out, deprecation.ReferenceSetUnderscore) {
		t.Errorf("page action --deprecated lists a domain-model spelling:\n%s", out)
	}

	// A broader topic includes its subtopics' entries.
	out, _ = runRoot(t, "syntax", "page", "--deprecated")
	if !strings.Contains(out, deprecation.PageActionWord) || !strings.Contains(out, "MDL-DEPR101") {
		t.Errorf("page --deprecated lacks page.action / page.alter entries:\n%s", out)
	}

	out, _ = runRoot(t, "syntax", "--deprecated", "--json")
	var rows []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("--deprecated --json: %v\n%s", err, firstLines(out, 5))
	}
	if len(rows) != len(deprecation.All()) {
		t.Errorf("--deprecated with no topic listed %d entries, want all %d", len(rows), len(deprecation.All()))
	}

	// Control: without the flag, the topic prints its syntax, not the list.
	out, _ = runRoot(t, "syntax", "page", "action")
	if strings.Contains(out, "Deprecated spellings for") {
		t.Errorf("syntax without --deprecated printed the deprecated list")
	}
}
