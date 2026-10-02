// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/migration"
	"github.com/spf13/cobra"
)

// helpCmd replaces cobra's built-in `help`: `mxcli help <command>` works as
// before, and `mxcli help <code>` prints the registry entry for a deprecated
// spelling (MDL-DEPRnnn) or a language change (MDL-V1-*) — the old form, the
// new form, whether `fmt --upgrade` rewrites it and the version that refuses
// it. Every warning carrying such a code ends with "(mxcli help <code>)"
// (ako/mxcli#714 decision 3).
var helpCmd = &cobra.Command{
	Use:   "help [command | code]",
	Short: "Help about any command, or about a warning code",
	Long: `Help provides help for any command in the application, or prints the
entry for a warning code: a deprecated spelling (MDL-DEPRnnn) or a change of
meaning between language versions (MDL-V1-*).

  mxcli help exec
  mxcli help MDL-DEPR001
  mxcli help MDL-V1-LIMIT1

Every code is also tabulated on the docs page "Language versions and migration".`,
	ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []string
		cmd, _, e := c.Root().Find(args)
		if e != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if cmd == nil {
			cmd = c.Root()
		}
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() || sub.Name() == "help" {
				completions = append(completions, sub.Name())
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) == 1 {
			if text, ok := checkCodeHelp[strings.ToUpper(args[0])]; ok {
				fmt.Fprint(c.OutOrStdout(), text)
				return nil
			}
		}
		if len(args) == 1 && migration.IsCode(args[0]) {
			text, ok := migration.Help(args[0])
			if !ok {
				c.SilenceUsage = true
				return fmt.Errorf("unknown code %s: no deprecated spelling or language change has it "+
					"(the docs page \"Language versions and migration\" lists every code)", args[0])
			}
			fmt.Fprint(c.OutOrStdout(), text)
			return nil
		}
		// cobra's own help, unchanged.
		cmd, _, e := c.Root().Find(args)
		if cmd == nil || e != nil {
			c.Printf("Unknown help topic %#q\n", args)
			return c.Root().Usage()
		}
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		return cmd.Help()
	},
}

// checkCodeHelp answers `mxcli help <code>` for the `check` rules that are
// neither a deprecated spelling nor a language change. The full entry, with the
// measured name spaces, is the docs page "Error Messages Reference".
var checkCodeHelp = map[string]string{
	"MDL-DUPDEF": `MDL-DUPDEF: element defined twice in one script

  Two plain ` + "`create`" + ` statements make the same element (same kind, same
  qualified name) with no drop or rename between them. exec would create the
  first and refuse the second as "already exists", after everything before it
  had been written.

  Fix: keep one; use ` + "`create or modify`" + ` to change an element the script
  already made, or drop it before re-creating it.

  check --references (-p app.mpr) also reports a plain create of an element the
  project already has, for every create exec refuses that way.

  Docs: Appendixes > Error Messages Reference > MDL-DUPDEF
`,
	"MDL-DUPNAME": `MDL-DUPNAME: name already taken in the module

  The statement gives an element a name Mendix will not let it share:
    microflows, nanoflows, rules                     CE0122
    pages, snippets, layouts                         CE0122
    entities, associations, enumerations             CE0065
  Every other kind is a name space of its own. Folders do not separate names,
  and names compare case-insensitively: M.act_login next to microflow
  M.ACT_Login is a duplicate (CE0122).

  Reported for a create, a rename ... to, and a move ... to Module. exec
  refuses it before writing anything, under every language version.

  Fix: pick another name or rename the other element first; to change the
  existing element, spell its name as it is stored.

  Docs: Appendixes > Error Messages Reference > MDL-DUPNAME
`,
}

func init() {
	rootCmd.SetHelpCommand(helpCmd)
}
