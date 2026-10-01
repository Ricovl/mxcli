// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd is `mxcli version`, the bare-subcommand spelling every
// neighbouring tool (go, docker, gh, mx) accepts. It prints exactly what
// `--version` prints — the build version and build time compiled in through
// -X main.Version / -X main.BuildTime — so a bug report quoting either one
// identifies the same build (ako/mxcli#534).
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the mxcli version and build time",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		root := cmd.Root()
		// Same text as cobra's default --version template.
		fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", root.Name(), root.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
