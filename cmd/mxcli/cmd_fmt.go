// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/formatter"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [file.mdl | -]",
	Short: "Format an MDL file",
	Long: `Format an MDL script file with consistent styling:
  - Uppercase MDL keywords
  - Normalize indentation (2-space units)
  - Remove trailing whitespace
  - Normalize blank lines

Pass '-' (or omit the argument) to read from stdin.

Examples:
  # Format to stdout
  mxcli fmt script.mdl

  # Format in-place
  mxcli fmt script.mdl -w

  # Format from stdin (pipe)
  mxcli describe microflow Mod.MF | mxcli fmt
  mxcli describe microflow Mod.MF | mxcli fmt -

Upgrading (--upgrade):
  Rewrites every deprecated spelling registered in the deprecation registry to
  its canonical form (create or replace -> create or modify, show -> list, ...)
  and changes nothing else: comments, layout and keyword case are kept, and the
  heuristic formatting above is not applied. A deprecated use that has no
  mechanical rewrite is reported on stderr and left in place.

  --header also adds the language header (mdl 1;), after rewriting every
  construct whose meaning the header would change. A construct without such a
  rewrite blocks the header, and fmt fails rather than change the script's
  meaning. While mdl 1 is a preview the header is added only when asked.

  # Upgrade in place
  mxcli fmt --upgrade -w script.mdl
  mxcli fmt --upgrade --header -w script.mdl
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		writeInPlace, _ := cmd.Flags().GetBool("write")
		doUpgrade, _ := cmd.Flags().GetBool("upgrade")
		addHeader, _ := cmd.Flags().GetBool("header")
		if cmd.Flags().Changed("header") && !doUpgrade {
			return fmt.Errorf("--header needs --upgrade")
		}

		// Determine source: stdin when no arg or "-" is passed.
		fromStdin := len(args) == 0 || args[0] == "-"
		filePath := ""
		if !fromStdin {
			filePath = args[0]
		}

		if writeInPlace && fromStdin {
			return fmt.Errorf("-w cannot be used with stdin")
		}

		var data []byte
		var err error
		if fromStdin {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(filePath)
		}
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		label := filePath
		if fromStdin {
			label = "<stdin>"
		}

		// Reject unparseable input so automation scripts can detect failures.
		// Two failure modes:
		//   1. ANTLR reports explicit parse errors (structural violations).
		//   2. ANTLR silently skips unrecognised tokens — detected when no
		//      statements were produced from non-blank, non-comment content.
		prog, errs := visitor.Build(string(data))
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			return fmt.Errorf("syntax errors in %s:\n%s", label, strings.Join(msgs, "\n"))
		}
		if prog != nil && len(prog.Statements) == 0 && hasSubstantiveContent(string(data)) {
			return fmt.Errorf("no valid MDL statements found in %s", label)
		}

		var formatted string
		if doUpgrade {
			opts := upgrade.DefaultOptions()
			if cmd.Flags().Changed("header") {
				opts.AddHeader = addHeader
			}
			res, err := upgrade.Upgrade(string(data), opts)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			reportUpgrade(os.Stderr, label, res)
			formatted = res.Source
		} else {
			formatted = formatter.Format(string(data))
		}

		if writeInPlace {
			if doUpgrade && formatted == string(data) {
				return nil // nothing to upgrade: leave the file untouched
			}
			if err := os.WriteFile(filePath, []byte(formatted), 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
			verb := "Formatted"
			if doUpgrade {
				verb = "Upgraded"
			}
			fmt.Fprintf(os.Stderr, "%s %s\n", verb, filePath)
		} else {
			fmt.Print(formatted)
		}

		return nil
	},
}

func init() {
	fmtCmd.Flags().BoolP("write", "w", false, "Write result to source file instead of stdout")
	fmtCmd.Flags().Bool("upgrade", false, "Rewrite deprecated spellings to their canonical form, changing nothing else")
	fmtCmd.Flags().Bool("header", false, "With --upgrade: add the mdl 1 language header (opt-in while mdl 1 is a preview)")
}

// reportUpgrade prints what an upgrade did, and what it left, on w.
func reportUpgrade(w io.Writer, label string, res upgrade.Result) {
	codes := func(m map[string]int) string {
		var parts []string
		for c, n := range m {
			parts = append(parts, fmt.Sprintf("%s x%d", c, n))
		}
		sort.Strings(parts)
		return strings.Join(parts, ", ")
	}
	if len(res.Rewritten) > 0 {
		fmt.Fprintf(w, "%s: rewrote %s\n", label, codes(res.Rewritten))
	}
	if len(res.GatedRewritten) > 0 {
		fmt.Fprintf(w, "%s: rewrote for the header %s\n", label, codes(res.GatedRewritten))
	}
	if res.HeaderAdded {
		fmt.Fprintf(w, "%s: added the language header\n", label)
	}
	for _, d := range res.Unrewritten {
		msg := d.Code
		if e, ok := deprecation.Lookup(d.Code); ok {
			msg = fmt.Sprintf("%s (%s -> %s)", d.Code, e.Old, e.Canonical)
		}
		fmt.Fprintf(w, "%s:%d:%d: not upgraded, no mechanical rewrite: %s\n", label, d.Line, d.Column+1, msg)
	}
}

// hasSubstantiveContent reports whether s contains at least one non-blank,
// non-comment line — used to distinguish empty/comment-only files (which
// produce zero statements legitimately) from garbage input.
func hasSubstantiveContent(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "--") && !langver.IsHeaderLine(t) {
			return true
		}
	}
	return false
}
