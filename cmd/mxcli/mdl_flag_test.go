// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Every command listed as emitting MDL takes --mdl, defaulting to 1 (ako/mxcli#840).
func TestMdlFlagIsOnEveryEmittingCommand(t *testing.T) {
	for _, c := range mdlFlagCommands {
		f := c.Flags().Lookup("mdl")
		if f == nil {
			t.Errorf("%s has no --mdl flag", c.CommandPath())
			continue
		}
		if f.DefValue != "1" {
			t.Errorf("%s --mdl defaults to %q, want 1", c.CommandPath(), f.DefValue)
		}
	}
	// A script file is read by its own header, never by a flag (ADR-0011).
	for _, c := range []string{"exec", "check", "fmt"} {
		cmd, _, err := rootCmd.Find([]string{c})
		if err != nil || cmd.Flags().Lookup("mdl") != nil {
			t.Errorf("%s takes --mdl (err %v); a script file is read by its header", c, err)
		}
	}
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// `mxcli describe` writes mdl 1 headed by `mdl 1;`, and --mdl 0 the alpha
// language without a header (ako/mxcli#840, freeze decision 5). Two outputs
// concatenated are one valid script.
func TestDescribeSubcommandLanguage(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	mdl1 := runMainStreams(t, dir, "-p", mpr, "describe", "entity", "System.User")
	if mdl1.exitCode != 0 || firstLine(mdl1.stdout) != "mdl 1;" {
		t.Fatalf("describe: exit %d, want output headed by `mdl 1;`:\n%s\n%s", mdl1.exitCode, mdl1.stdout, mdl1.stderr)
	}
	if _, errs := visitor.Build(mdl1.stdout + "\n" + mdl1.stdout); len(errs) > 0 {
		t.Errorf("two describe outputs concatenated do not parse: %v", errs)
	}

	mdl0 := runMainStreams(t, dir, "-p", mpr, "describe", "--mdl", "0", "entity", "System.User")
	if mdl0.exitCode != 0 || strings.Contains(mdl0.stdout, "mdl 1;") || !strings.Contains(mdl0.stdout, "System.User") {
		t.Fatalf("describe --mdl 0: exit %d, want the description without a header:\n%s\n%s", mdl0.exitCode, mdl0.stdout, mdl0.stderr)
	}
	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(mdl1.stdout), "mdl 1;")) != strings.TrimSpace(mdl0.stdout) {
		t.Logf("note: the entity's mdl 0 and mdl 1 descriptions differ beyond the header")
	}

	if bad := runMainStreams(t, dir, "-p", mpr, "describe", "--mdl", "2", "entity", "System.User"); bad.exitCode == 0 ||
		!strings.Contains(bad.stderr, "mdl 0 through mdl 1") {
		t.Errorf("describe --mdl 2: exit %d, stderr %q", bad.exitCode, bad.stderr)
	}
}

// `mxcli -c` reads input without a header as mdl 1 since the freeze (decision
// 6); --mdl 0, or an `mdl 0;` the commands state, reads it as mdl 0. The probe
// is `show entity X`, which mdl 1 refuses (MDL-V1-SHOWSUMMARY) and mdl 0 runs.
func TestOneLinerLanguage(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	r := runMainStreams(t, dir, "-p", mpr, "-c", "show entity System.User")
	if r.exitCode == 0 || !strings.Contains(r.stderr, "is not in mdl 1") {
		t.Fatalf("-c without a header was not read as mdl 1: exit %d\nstdout:\n%s\nstderr:\n%s", r.exitCode, r.stdout, r.stderr)
	}
	for _, args := range [][]string{
		{"-p", mpr, "--mdl", "0", "-c", "show entity System.User"},
		{"-p", mpr, "-c", "mdl 0; show entity System.User"},
	} {
		if r := runMainStreams(t, dir, args...); r.exitCode != 0 {
			t.Errorf("mxcli %v: exit %d, want the mdl 0 summary\nstderr:\n%s", args[2:], r.exitCode, r.stderr)
		}
	}

	// describe follows the one-liner's language.
	if r := runMainStreams(t, dir, "-p", mpr, "-c", "describe entity System.User"); firstLine(r.stdout) != "mdl 1;" {
		t.Errorf("-c describe is not headed by `mdl 1;`:\n%s\n%s", r.stdout, r.stderr)
	}
	for _, args := range [][]string{
		{"-p", mpr, "--mdl", "0", "-c", "describe entity System.User"},
		{"-p", mpr, "-c", "mdl 0; describe entity System.User"},
	} {
		if r := runMainStreams(t, dir, args...); r.exitCode != 0 || strings.Contains(r.stdout, "mdl 1;") {
			t.Errorf("mxcli %v: exit %d, want an mdl 0 description:\n%s\n%s", args[2:], r.exitCode, r.stdout, r.stderr)
		}
	}

	// A header switching the language part-way through one -c is refused.
	if r := runMainStreams(t, dir, "-p", mpr, "-c", "list modules; mdl 0; list modules"); r.exitCode == 0 ||
		!strings.Contains(r.stderr, "conflicts") {
		t.Errorf("a conflicting header in -c: exit %d, stderr %q", r.exitCode, r.stderr)
	}
}

func TestHoverDropsTheLanguageHeader(t *testing.T) {
	if got := withoutLanguageHeader("mdl 1;\ncreate entity M.E ();"); got != "create entity M.E ();" {
		t.Errorf("got %q", got)
	}
	if got := withoutLanguageHeader("create entity M.E ();"); got != "create entity M.E ();" {
		t.Errorf("a description without a header changed: %q", got)
	}
}
