// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/cmd/mxcli/evalrunner"
	"github.com/spf13/cobra"
)

var evalRunCmd = &cobra.Command{
	Use:   "run <file>",
	Short: "Run Claude Code headless on an eval test, then check the project and report the session",
	Long: `Run one eval test end to end, unattended:

  1. create a fresh project with 'mxcli new' (which runs 'mxcli init', so the
     agent gets the CLAUDE.md and skills a user gets) — or use -p
  2. run 'claude -p <prompt>' in it, permissions bypassed, with a known
     session id
  3. copy the session transcript (and its subagent transcripts) into the run
     directory
  4. run the test's automated checks against the project
  5. write 'diag session-report' for the transcript: model calls, tokens,
     tool calls by category, costliest results, error -> retry chains

Everything lands in <output>/<test-id>-<timestamp>/: transcript.jsonl,
claude-stream.jsonl, session-report.txt/.json, and the check reports.

The agent runs with --dangerously-skip-permissions. Run it on a scratch
project only. Variables a hosting Claude Code session sets for its children
(CLAUDECODE, CLAUDE_CODE_*) are removed, so a run started from inside an agent
session behaves like one started from a terminal; the agent needs its own
credentials (a logged-in 'claude', or ANTHROPIC_API_KEY).

An iteration section in the test is not run; its checks are skipped.

Examples:
  mxcli eval run docs/14-eval/eval-bench-001.md --version 11.15.0
  mxcli eval run docs/14-eval/eval-bench-001.md --version 11.15.0 --timeout 45m -o ~/bench
  mxcli eval run docs/14-eval/eval-1.md -p scratch/app.mpr --skip-mx-check
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		version, _ := cmd.Flags().GetString("version")
		outputDir, _ := cmd.Flags().GetString("output")
		claudePath, _ := cmd.Flags().GetString("claude")
		model, _ := cmd.Flags().GetString("model")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		claudeArgs, _ := cmd.Flags().GetStringArray("claude-arg")
		skipMxCheck, _ := cmd.Flags().GetBool("skip-mx-check")
		skipTests, _ := cmd.Flags().GetBool("skip-tests")
		mxcliPath, _ := cmd.Flags().GetString("mxcli-path")
		top, _ := cmd.Flags().GetInt("top")

		if projectPath == "" && version == "" {
			fmt.Fprintln(os.Stderr, "Error: --version is required to create a fresh project (or pass -p)")
			os.Exit(1)
		}
		if mxcliPath == "" {
			if self, err := os.Executable(); err == nil {
				mxcliPath = self
			} else {
				mxcliPath = "mxcli"
			}
		}
		test, err := evalrunner.ParseEvalFile(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		runDir, err := filepath.Abs(filepath.Join(outputDir, fmt.Sprintf("%s-%s", test.ID, time.Now().Format("2006-01-02T15-04-05"))))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		run, agentErr := evalrunner.RunAgent(test, evalrunner.AgentOptions{
			RunDir:        runDir,
			ProjectPath:   projectPath,
			MendixVersion: version,
			MxCliPath:     mxcliPath,
			ClaudePath:    claudePath,
			Model:         model,
			Timeout:       timeout,
			ExtraArgs:     claudeArgs,
			Progress:      os.Stdout,
		})
		if run == nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", agentErr)
			os.Exit(1)
		}
		failed := false
		if agentErr != nil {
			fmt.Fprintf(os.Stderr, "Agent: %v\n", agentErr)
			failed = true
		} else {
			fmt.Fprintf(os.Stdout, "Agent finished in %s\n", run.Duration.Round(time.Second))
		}

		// The report first: it is the point of a benchmark run, and it is
		// worth having even when the agent failed or the checks do not pass.
		if run.TranscriptPath != "" {
			if err := writeRunSessionReport(run.TranscriptPath, runDir, top, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: session report: %v\n", err)
			}
		}

		if agentErr == nil || run.TimedOut {
			checkTest := *test
			checkTest.Iteration = nil
			checkOpts := evalrunner.CheckOptions{
				ProjectPath: run.ProjectPath,
				MxCliPath:   mxcliPath,
				SkipMxCheck: skipMxCheck,
				SkipTests:   skipTests,
			}
			if version != "" {
				if mx := docker.CachedMxPath(version); mx != "" {
					if _, err := os.Stat(mx); err == nil {
						checkOpts.MxPath = mx
					}
				}
			}
			result := runEvalTest(&checkTest, checkOpts, false)
			summary := &evalrunner.RunSummary{Timestamp: time.Now(), Results: []evalrunner.EvalResult{*result}}
			if err := evalrunner.WriteJSONReport(result, runDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			if err := evalrunner.WriteMarkdownReport(summary, runDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			if result.OverallScore < 1.0 {
				failed = true
			}
		}
		fmt.Fprintf(os.Stdout, "Run directory: %s\n", runDir)
		if failed {
			os.Exit(1)
		}
	},
}

// writeRunSessionReport writes session-report.txt and .json beside the
// transcript and prints the text form.
func writeRunSessionReport(transcript, dir string, top int, w io.Writer) error {
	var text, js bytes.Buffer
	if err := writeSessionReports(&text, []string{transcript}, top, true, false); err != nil {
		return err
	}
	if err := writeSessionReports(&js, []string{transcript}, top, true, true); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "session-report.txt"), text.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "session-report.json"), js.Bytes(), 0o644); err != nil {
		return err
	}
	_, err := w.Write(text.Bytes())
	return err
}

func init() {
	f := evalRunCmd.Flags()
	f.String("version", "", "Mendix version for the fresh project (e.g. 11.15.0)")
	f.StringP("output", "o", "eval-runs", "Directory that receives the run directory")
	f.String("claude", "claude", "The claude CLI to run")
	f.String("model", "", "Model for the agent (default: claude's own default)")
	f.Duration("timeout", 0, "Stop the agent after this long (default: the test's timeout)")
	f.StringArray("claude-arg", nil, "Extra argument for claude (repeatable)")
	f.Bool("skip-mx-check", false, "Skip mx_check_passes checks")
	f.Bool("skip-tests", false, "Skip tests_pass checks")
	f.String("mxcli-path", "", "mxcli to create the project with and copy into it (default: self)")
	f.Int("top", 10, "Costliest results to list in the session report")
	evalCmd.AddCommand(evalRunCmd)
}
