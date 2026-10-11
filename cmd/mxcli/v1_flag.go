// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/spf13/cobra"
)

// The --v1 flag decides what `check` does with an MDL-V1-* warning: a construct
// a headerless (mdl 0) script keeps at its old meaning, or that `mdl 1;` would
// refuse. It mirrors --deprecations, which gates a different set of codes
// (MDL-DEPRnnn): a script can pass --deprecations=error and still not parse
// under mdl 1. Both at error is the full mdl 1 readiness gate.
const v1FlagUsage = "What to do with an MDL-V1-* warning (a headerless script not ready for `mdl 1;`): " +
	"warn, or error to fail the run (with --deprecations=error, the full mdl 1 readiness gate)"

func init() {
	checkCmd.Flags().String("v1", "warn", v1FlagUsage)
}

// parseV1Policy reads a --v1 value. Same values as --deprecations.
func parseV1Policy(raw string) (deprecation.Policy, error) {
	policy, err := deprecation.ParsePolicy(raw)
	if err != nil {
		return deprecation.Warn, fmt.Errorf("invalid --v1 value %q: want warn or error", strings.TrimSpace(raw))
	}
	return policy, nil
}

// v1Policy reads --v1, exiting with a usage error on a value it does not know:
// silently treating a typo as `warn` would let a CI gate pass that was meant
// to fail.
func v1Policy(cmd *cobra.Command) deprecation.Policy {
	raw, _ := cmd.Flags().GetString("v1")
	policy, err := parseV1Policy(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}
	return policy
}
