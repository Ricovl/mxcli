// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// The exit status is the contract of `mxcli playwright check`: a caller
// branches on it instead of reading the output (or a screenshot).
func TestCheckExitCode(t *testing.T) {
	if got := checkExitCode(docker.PageCheckResult{Pages: 3}); got != 0 {
		t.Errorf("all pages passed: exit %d, want 0", got)
	}
	if got := checkExitCode(docker.PageCheckResult{Pages: 3, Failed: 1}); got != 1 {
		t.Errorf("one page failed: exit %d, want 1", got)
	}
}

// check is registered under `mxcli playwright` with the flags the skills use.
func TestPlaywrightCheckFlags(t *testing.T) {
	cmd, _, err := playwrightCmd.Find([]string{"check"})
	if err != nil || cmd != playwrightCheckCmd {
		t.Fatalf("playwright check not registered: %v", err)
	}
	for _, f := range []string{"user", "password", "role", "assert-text", "assert-count", "screenshot", "base-url", "fresh-login", "wait"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}
