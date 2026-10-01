// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestVersionSubcommand is ako/mxcli#534: `mxcli version` was an unknown
// command although --version worked and shouldSuppressWarning already
// expected a `version` argument. The subcommand must print exactly what
// --version prints, so a bug report quoting either one carries the same
// build identification.
func TestVersionSubcommand(t *testing.T) {
	sub := runRootForTest(t, []string{"version"})
	flag := runRootForTest(t, []string{"--version"})
	if strings.TrimSpace(sub) == "" {
		t.Fatal("`mxcli version` printed nothing")
	}
	if sub != flag {
		t.Errorf("`mxcli version` = %q, `mxcli --version` = %q; want identical", sub, flag)
	}
	if !strings.Contains(sub, rootCmd.Version) {
		t.Errorf("`mxcli version` = %q, does not carry the build version %q", sub, rootCmd.Version)
	}
}

func runRootForTest(t *testing.T, args []string) string {
	t.Helper()
	rootCmd.SetOut(nil)
	out, err := captureStdout(t, func() error {
		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("`mxcli %v` failed: %v\n%s", args, err, out)
	}
	return out
}
