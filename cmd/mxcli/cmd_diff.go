// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/scriptdiff"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <script.mdl>",
	Short: "Show what executing an MDL script would change in the project",
	Long: `Show what "mxcli exec" would change in a Mendix project, without changing it.

The script is executed by exec itself, against a scratch copy of the project,
and the copy is then compared with the project unit by unit. What diff reports
is therefore exactly what exec would write: the documents it adds, rewrites,
moves or removes (an entity or association counts as its own document), and
the files next to the model it writes. Each change is shown as the DESCRIBE of
the document before and after. A unit exec rewrites whose description does
not change says which properties change instead.

Like exec, diff runs the pre-flight checks first (skip them with --no-check),
runs the statements in order under the script's language header, and stops at
the first error (--continue-on-error runs every statement). A script exec
would refuse, or a statement it would stop at, is reported as Refused.

The project itself is only read. The scratch copy leaves out .git, deployment,
releases and node_modules, and is deleted afterwards.

Output Formats:
  unified  - Traditional unified diff format (default)
  side     - Side-by-side comparison
  struct   - Structural changes summary

Examples:
  # Unified diff (default)
  mxcli diff -p app.mpr changes.mdl

  # Side-by-side diff
  mxcli diff -p app.mpr changes.mdl --format side

  # Structural diff
  mxcli diff -p app.mpr changes.mdl --format struct

  # With color output
  mxcli diff -p app.mpr changes.mdl --color
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		format, _ := cmd.Flags().GetString("format")
		useColor, _ := cmd.Flags().GetBool("color")
		width, _ := cmd.Flags().GetInt("width")
		skipCheck, _ := cmd.Flags().GetBool("no-check")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		showExecOutput, _ := cmd.Flags().GetBool("exec-output")
		depPolicy := deprecationPolicy(cmd)
		refuseJSONFlag("diff", "--format unified|side|struct")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		content, err := readMDLSource(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors found:\n")
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
			os.Exit(1)
		}
		if line, bad := unparsableInput(string(content), len(prog.Statements)); bad {
			fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
			os.Exit(1)
		}

		opts := scriptdiff.Options{
			// Always the file engine, whatever --mcp says: the script is
			// executed for real, and only the scratch copy may receive it.
			NewBackend:      func() backend.FullBackend { return modelsdkbackend.New() },
			ContinueOnError: continueOnError,
			Preflight: func(scratch *executor.Executor, w io.Writer) string {
				return execPreflight(scratch, prog, projectPath, skipCheck, depPolicy, w, useColor)
			},
		}
		if filePath != "-" {
			if abs, absErr := filepath.Abs(filePath); absErr == nil {
				opts.ScriptDir = filepath.Dir(abs)
			}
		}

		report, err := scriptdiff.Run(projectPath, prog, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		report.Write(os.Stdout, executor.DiffOptions{
			Format:   executor.DiffFormat(format),
			UseColor: useColor,
			Width:    width,
		}, showExecOutput)
	},
}

var diffLocalCmd = &cobra.Command{
	Use:   "diff-local",
	Short: "Compare local changes against git",
	Long: `Compare local (uncommitted) changes in mxunit files against a git reference.

This command finds modified mxunit files in the mprcontents/ folder and shows
the differences as MDL. Only works with MPR v2 format (Mendix 10.18+).

The --ref flag accepts any git ref or range (e.g., HEAD, main, main..feature-branch).

Examples:
  # Show uncommitted changes vs HEAD
  mxcli diff-local -p app.mpr

  # Compare against a specific commit
  mxcli diff-local -p app.mpr --ref HEAD~1

  # Compare against a branch
  mxcli diff-local -p app.mpr --ref main

  # Compare two arbitrary revisions (git range syntax)
  mxcli diff-local -p app.mpr --ref main..feature-branch

  # Three-dot range (changes since common ancestor)
  mxcli diff-local -p app.mpr --ref main...feature-branch

  # With structural format
  mxcli diff-local -p app.mpr --format struct --color
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		ref, _ := cmd.Flags().GetString("ref")
		format, _ := cmd.Flags().GetString("format")
		useColor, _ := cmd.Flags().GetBool("color")
		width, _ := cmd.Flags().GetInt("width")
		refuseJSONFlag("diff-local", "--format unified|side|struct")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Default ref to HEAD
		if ref == "" {
			ref = "HEAD"
		}

		// Create executor and connect
		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}

		// Run diff-local
		opts := executor.DiffOptions{
			Format:   executor.DiffFormat(format),
			UseColor: useColor,
			Width:    width,
		}

		if err := exec.DiffLocal(ref, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}
