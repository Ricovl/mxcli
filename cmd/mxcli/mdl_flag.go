// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/spf13/cobra"
)

// mdlFlagCommands are the commands that take `--mdl <n>`, the MDL language
// version they emit or read interactively (ako/mxcli#840, freeze decision 6).
// Listed in one place so the help text, the docs and the test agree:
//
//   - mxcli (the REPL) and mxcli -c: the language of input without a header;
//     describe output follows it.
//   - describe: the language of the description.
//   - context: the language of the MDL sources it assembles.
//   - diff-local: the language the MDL diff is written in.
//
// A script FILE is not on the list: exec, check and fmt read a headerless
// file as mdl 0 and a headed one by its header (ADR-0011), whatever the flag.
var mdlFlagCommands = []*cobra.Command{rootCmd, describeCmd, contextCmd, diffLocalCmd}

func init() {
	for _, c := range mdlFlagCommands {
		c.Flags().String("mdl", fmt.Sprint(int(langver.Interactive)),
			"MDL language version to emit and read: 1 (default) or 0, the alpha language")
	}
}

// mdlFlag reads `--mdl`, exiting on a version this mxcli does not know.
func mdlFlag(cmd *cobra.Command) langver.Version {
	s, err := cmd.Flags().GetString("mdl")
	if err != nil {
		return langver.Interactive
	}
	v, err := langver.ParseFlag(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return v
}
