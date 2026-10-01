// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <file|->",
	Short: "Execute an MDL script file",
	Long: `Execute an MDL script file containing MDL commands.

Before anything is written, the script is put through the same semantic checks
as "mxcli check". If any of them reports an error, nothing is executed: exec
applies statements one at a time and cannot roll back, so running a script with
a known error leaves the model partly updated. Warnings are printed and do not
stop the run. Use --no-check to apply a script anyway.

A deprecated MDL spelling (MDL-DEPRnnn, e.g. "create or replace" for "create or
modify") is a warning; --deprecations=error makes it an error.

By default execution stops at the first error. With --continue-on-error, every
statement is attempted; each failure is reported (prefixed with its statement
number) and execution continues, exiting non-zero if any statement failed. This
makes a partially-applied domain script re-runnable — the already-applied
statements (e.g. "attribute already exists") error individually while the not-
yet-applied ones still run — without a failure masking later work.

A write is refused while Studio Pro has the project open (its <project>.mpr.lock
is beside the .mpr): Studio Pro does not reload the model from disk, and its next
save would silently discard the change. Close the project in Studio Pro, or route
writes through it with --mcp. --force writes anyway (for a lock left behind by a
crash); MXCLI_ALLOW_STUDIO_PRO_OPEN=1 does the same for every command. Reads, and
re-running a script whose statements change nothing, are never refused.

Pass "-" as the file to read the script from standard input, so MDL can be
piped or written inline as a heredoc without a temporary file.

Example:
  mxcli exec setup.mdl
  mxcli exec -p app.mpr script.mdl
  mxcli exec -p app.mpr script.mdl --continue-on-error
  mxcli exec -p app.mpr script.mdl --no-check
  mxcli exec -p app.mpr - <<'EOF'
  SHOW STRUCTURE DEPTH 1;
  EOF
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		skipCheck, _ := cmd.Flags().GetBool("no-check")
		if force, _ := cmd.Flags().GetBool("force"); force {
			mmpr.AllowWritesWhileStudioProOpen = true
			if lock, _ := mmpr.StudioProLockFile(projectPath); lock != "" {
				fmt.Fprintf(os.Stderr, "Warning: Studio Pro appears to have this project open (%s); writing anyway (--force). "+
					"Studio Pro's next save will discard these changes unless the project is closed or reloaded first.\n", lock)
			}
		}
		depPolicy := deprecationPolicy(cmd)

		// Read the script (a path, or "-" for stdin)
		content, err := readMDLSource(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("exec")
		defer logger.Close()
		defer exec.Close()

		// A relative path inside the script (a toolbox icon, an image) names a
		// file next to the script, not next to the caller. Empty for stdin.
		if filePath != "-" {
			if abs, absErr := filepath.Abs(filePath); absErr == nil {
				exec.SetScriptDir(filepath.Dir(abs))
			}
		}

		// Auto-connect if project specified
		if projectPath != "" {
			connectCmd := fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath))
			prog, _ := visitor.Build(connectCmd)
			for _, stmt := range prog.Statements {
				if err := exec.Execute(stmt); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Parse and execute the file
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}
		// "Apply this file" that applies nothing is never what was meant, and a
		// silent no-op is the worst outcome for a replayable mdlsource/
		// (ako/mxcli#618).
		if line, bad := unparsableInput(string(content), len(prog.Statements)); bad {
			fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
			os.Exit(1)
		}

		if refusal := execPreflight(exec, prog, projectPath, skipCheck, depPolicy, os.Stderr, true); refusal != "" {
			fmt.Fprint(os.Stderr, refusal)
			os.Exit(1)
		}

		if continueOnError {
			res, err := exec.ExecuteProgramContinueOnError(prog, os.Stderr)
			if err != nil && !errors.Is(err, executor.ErrExit) {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "%d statements: %d succeeded, %d failed\n", res.Total, res.Succeeded, res.Failed)
			if res.Failed > 0 {
				os.Exit(1)
			}
			return
		}

		if err := exec.ExecuteProgram(prog); err != nil {
			if errors.Is(err, executor.ErrExit) {
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	execCmd.Flags().Bool("no-check", false,
		"Skip the pre-flight semantic checks and apply the script even if mxcli check would report errors")
	execCmd.Flags().Bool("force", false,
		"Write even though Studio Pro appears to have the project open (its .mpr.lock is present) — e.g. a lock left behind by a crash")
	execCmd.Flags().Bool("continue-on-error", false,
		"Run every statement, reporting each failure instead of halting at the first (exits non-zero if any failed) — makes a partially-applied script re-runnable")
}
