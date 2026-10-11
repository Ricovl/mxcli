// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

// ChipCoV8 #14: `test --local` had no way to move off 8081/8091/6544.
func TestTestPortFlags_ReachRunOptions(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	registerTestPortFlags(cmd)
	if err := cmd.ParseFlags([]string{"--app-port", "9081", "--admin-port=9091", "--serve-port", "7544"}); err != nil {
		t.Fatal(err)
	}
	var opts testrunner.RunOptions
	applyTestPortFlags(cmd, &opts)
	if opts.AppPort != 9081 || opts.AdminPort != 9091 || opts.ServePort != 7544 {
		t.Errorf("ports = %d/%d/%d, want 9081/9091/7544", opts.AppPort, opts.AdminPort, opts.ServePort)
	}

	// Control: unset flags leave the options at zero, which the runner resolves
	// to its defaults.
	cmd = &cobra.Command{Use: "test"}
	registerTestPortFlags(cmd)
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	opts = testrunner.RunOptions{}
	applyTestPortFlags(cmd, &opts)
	if opts.AppPort != 0 || opts.AdminPort != 0 || opts.ServePort != 0 {
		t.Errorf("unset flags gave ports %d/%d/%d, want 0/0/0", opts.AppPort, opts.AdminPort, opts.ServePort)
	}
}

func TestTestCommandRegistersPortFlags(t *testing.T) {
	for _, name := range []string{"app-port", "admin-port", "serve-port"} {
		if testRunCmd.Flags().Lookup(name) == nil {
			t.Errorf("mxcli test has no --%s flag", name)
		}
	}
}
