// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/spf13/cobra"
)

// The --deprecations flag decides what `check` and `exec` do with a deprecated
// MDL spelling (an MDL-DEPRnnn warning, registry in mdl/deprecation). The
// default warns; `error` fails the run, so CI over docs, skills and examples
// can hold them to the canonical form.
const deprecationsFlagUsage = "What to do with a deprecated MDL spelling (MDL-DEPRnnn): " +
	"warn, or error to fail the run (for CI over docs, skills and examples)"

func init() {
	checkCmd.Flags().String("deprecations", "warn", deprecationsFlagUsage)
	execCmd.Flags().String("deprecations", "warn", deprecationsFlagUsage)
}

// deprecationPolicy reads --deprecations, exiting with a usage error on a value
// it does not know: silently treating a typo as `warn` would let a CI gate pass
// that was meant to fail.
func deprecationPolicy(cmd *cobra.Command) deprecation.Policy {
	raw, _ := cmd.Flags().GetString("deprecations")
	policy, err := deprecation.ParsePolicy(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}
	return policy
}
