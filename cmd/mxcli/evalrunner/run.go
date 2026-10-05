// SPDX-License-Identifier: Apache-2.0

package evalrunner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AgentOptions configures one headless agent run of an eval test.
type AgentOptions struct {
	// RunDir receives everything the run produces: the project (unless
	// ProjectPath is set), the transcript and the agent's stream output.
	RunDir string
	// ProjectPath is an existing .mpr to run against. When empty a fresh
	// project is created in RunDir/app with `mxcli new`, which also runs
	// `mxcli init` — so the agent gets the CLAUDE.md and skills a user gets.
	ProjectPath   string
	MendixVersion string
	// MxCliPath is the mxcli used to create the project; `mxcli new` copies
	// it into the project, so it is also the mxcli the agent runs.
	MxCliPath  string
	ClaudePath string // default "claude"
	Model      string
	Timeout    time.Duration
	// ExtraArgs are passed to claude after the defaults.
	ExtraArgs []string
	// ClaudeHome is where Claude Code keeps transcripts (default ~/.claude).
	ClaudeHome string
	Progress   io.Writer
}

// AgentRun is what one headless run produced.
type AgentRun struct {
	ProjectPath string
	SessionID   string
	// TranscriptPath is the session transcript copied into RunDir; its
	// subagent transcripts sit beside it, as Claude Code lays them out.
	TranscriptPath string
	StreamPath     string
	Duration       time.Duration
	TimedOut       bool
	// ResultText is the final `result` record's text — the agent's closing
	// message, or the reason the run failed (e.g. an authentication error).
	ResultText string
	IsError    bool
}

// ProjectAppName is the name eval runs give a fresh project.
const ProjectAppName = "EvalApp"

// RunAgent creates (or reuses) a project, runs `claude -p` on the test's
// prompt in it, and copies the session transcript into RunDir.
func RunAgent(test *EvalTest, opts AgentOptions) (*AgentRun, error) {
	if opts.ClaudePath == "" {
		opts.ClaudePath = "claude"
	}
	if opts.Progress == nil {
		opts.Progress = io.Discard
	}
	if err := os.MkdirAll(opts.RunDir, 0o755); err != nil {
		return nil, err
	}
	run := &AgentRun{ProjectPath: opts.ProjectPath}

	if run.ProjectPath == "" {
		if opts.MendixVersion == "" {
			return nil, errors.New("a Mendix version is needed to create a fresh project")
		}
		appDir := filepath.Join(opts.RunDir, "app")
		fmt.Fprintf(opts.Progress, "Creating %s (Mendix %s) in %s...\n", ProjectAppName, opts.MendixVersion, appDir)
		cmd := exec.Command(opts.MxCliPath, "new", ProjectAppName, "--version", opts.MendixVersion, "--output-dir", appDir)
		log, err := os.Create(filepath.Join(opts.RunDir, "mxcli-new.log"))
		if err != nil {
			return nil, err
		}
		cmd.Stdout, cmd.Stderr = log, log
		err = cmd.Run()
		log.Close()
		if err != nil {
			return nil, fmt.Errorf("mxcli new failed (see %s): %w", log.Name(), err)
		}
		run.ProjectPath = filepath.Join(appDir, ProjectAppName+".mpr")
	}
	projectDir := filepath.Dir(run.ProjectPath)

	run.SessionID = newUUID()
	args := ClaudeArgs(test.Prompt, run.SessionID, opts.Model, opts.ExtraArgs)
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = test.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	run.StreamPath = filepath.Join(opts.RunDir, "claude-stream.jsonl")
	stream, err := os.Create(run.StreamPath)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	stderr, err := os.Create(filepath.Join(opts.RunDir, "claude-stderr.txt"))
	if err != nil {
		return nil, err
	}
	defer stderr.Close()

	cmd := exec.CommandContext(ctx, opts.ClaudePath, args...)
	cmd.Dir = projectDir
	cmd.Env = AgentEnv(os.Environ())
	cmd.Stdout = stream
	cmd.Stderr = stderr
	cmd.WaitDelay = 10 * time.Second

	fmt.Fprintf(opts.Progress, "Running %s -p in %s (session %s, timeout %s)...\n", opts.ClaudePath, projectDir, run.SessionID, timeout)
	start := time.Now()
	runErr := cmd.Run()
	run.Duration = time.Since(start)
	run.TimedOut = ctx.Err() == context.DeadlineExceeded
	stream.Close()

	run.ResultText, run.IsError = readStreamResult(run.StreamPath)

	if src := findTranscript(opts.ClaudeHome, run.SessionID); src != "" {
		run.TranscriptPath = filepath.Join(opts.RunDir, "transcript.jsonl")
		if err := copyTranscript(src, run.TranscriptPath); err != nil {
			return run, err
		}
	} else if fi, err := os.Stat(run.StreamPath); err == nil && fi.Size() > 0 {
		// No transcript on disk (a different CLAUDE_CONFIG_DIR, say). The
		// stream-json output carries the same assistant/user messages, without
		// timestamps, so the report still has calls, results and tokens.
		run.TranscriptPath = run.StreamPath
	}

	switch {
	case run.TimedOut:
		return run, fmt.Errorf("agent run timed out after %s", timeout)
	case run.IsError:
		return run, fmt.Errorf("agent run failed: %s", run.ResultText)
	case runErr != nil:
		return run, fmt.Errorf("claude exited: %w", runErr)
	}
	return run, nil
}

// ClaudeArgs is the headless invocation: print mode, a known session id so
// the transcript can be found afterwards, stream-json so a failed run still
// leaves a record, and permissions bypassed because nobody is there to grant
// them. Run it only on a scratch project.
func ClaudeArgs(prompt, sessionID, model string, extra []string) []string {
	args := []string{"-p", prompt,
		"--session-id", sessionID,
		"--output-format", "stream-json", "--verbose",
		"--dangerously-skip-permissions",
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, extra...)
}

// hostSessionEnv matches the variables a hosting Claude Code session sets
// for its own children. A benchmark started from inside an agent session must
// not inherit them: they make the child behave as part of the host session
// (auth refresh through the host, its session id) instead of as a user's
// terminal.
var hostSessionEnv = regexp.MustCompile(`^(CLAUDECODE|CLAUDE_CODE_[A-Z_]+)=`)

// AgentEnv is the environment the agent runs in.
func AgentEnv(env []string) []string {
	var out []string
	for _, e := range env {
		if hostSessionEnv.MatchString(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// readStreamResult returns the final `result` record of a stream-json run.
func readStreamResult(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	var text string
	var isErr bool
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var r struct {
			Type    string `json:"type"`
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
		}
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Type == "result" {
			text, isErr = r.Result, r.IsError
		}
	}
	return text, isErr
}

// findTranscript locates <claude home>/projects/*/<session>.jsonl.
func findTranscript(home, sessionID string) string {
	if home == "" {
		if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
			home = d
		} else if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".claude")
		}
	}
	m, _ := filepath.Glob(filepath.Join(home, "projects", "*", sessionID+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// copyTranscript copies a transcript and its subagent transcripts
// (<session>/subagents/*.jsonl) so that dst keeps the same layout.
func copyTranscript(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return err
	}
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(src, ".jsonl"), "subagents", "*.jsonl"))
	if len(subs) == 0 {
		return nil
	}
	dir := filepath.Join(strings.TrimSuffix(dst, ".jsonl"), "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, s := range subs {
		if err := copyFile(s, filepath.Join(dir, filepath.Base(s))); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
