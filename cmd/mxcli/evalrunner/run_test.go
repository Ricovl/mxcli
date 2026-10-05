// SPDX-License-Identifier: Apache-2.0

package evalrunner

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stubClaude is a stand-in for the claude CLI: it records its argv and
// working directory, writes a transcript (and a subagent transcript) where
// Claude Code would, and prints a stream-json result record.
const stubClaude = `#!/bin/sh
printf '%s\n' "$@" > "$STUB_OUT/argv"
pwd > "$STUB_OUT/cwd"
env > "$STUB_OUT/env"
sid=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--session-id" ]; then sid="$a"; fi
  prev="$a"
done
d="$CLAUDE_CONFIG_DIR/projects/-proj"
mkdir -p "$d/$sid/subagents"
echo '{"type":"assistant","sessionId":"'$sid'","message":{"id":"m1","usage":{"cache_read_input_tokens":10},"content":[]}}' > "$d/$sid.jsonl"
echo '{"type":"assistant","isSidechain":true,"agentId":"x","message":{"id":"s1","content":[]}}' > "$d/$sid/subagents/agent-x.jsonl"
echo '{"type":"result","is_error":'$STUB_IS_ERROR',"result":"'"$STUB_RESULT"'"}'
`

func setupStub(t *testing.T, isErr bool, result string) (claude, out, home, mpr string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	claude = filepath.Join(dir, "claude")
	if err := os.WriteFile(claude, []byte(stubClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	out = filepath.Join(dir, "out")
	home = filepath.Join(dir, "claudehome")
	_ = os.MkdirAll(out, 0o755)
	proj := filepath.Join(dir, "proj")
	_ = os.MkdirAll(proj, 0o755)
	mpr = filepath.Join(proj, "App.mpr")
	_ = os.WriteFile(mpr, nil, 0o644)
	t.Setenv("STUB_OUT", out)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	t.Setenv("STUB_IS_ERROR", map[bool]string{true: "true", false: "false"}[isErr])
	t.Setenv("STUB_RESULT", result)
	return
}

func TestRunAgentCopiesTranscript(t *testing.T) {
	claude, out, _, mpr := setupStub(t, false, "done")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "host-session") // must not reach the agent
	runDir := filepath.Join(t.TempDir(), "run")
	test := &EvalTest{ID: "T", Prompt: "build it", Timeout: time.Minute}

	run, err := RunAgent(test, AgentOptions{RunDir: runDir, ProjectPath: mpr, ClaudePath: claude, Model: "opus"})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if run.ResultText != "done" || run.IsError {
		t.Fatalf("result = %q %v", run.ResultText, run.IsError)
	}
	if run.TranscriptPath != filepath.Join(runDir, "transcript.jsonl") {
		t.Fatalf("transcript = %q", run.TranscriptPath)
	}
	if _, err := os.Stat(filepath.Join(runDir, "transcript", "subagents", "agent-x.jsonl")); err != nil {
		t.Fatalf("subagent transcript not copied: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(out, "argv"))
	want := ClaudeArgs("build it", run.SessionID, "opus", nil)
	if got := strings.Split(strings.TrimSpace(string(argv)), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	cwd, _ := os.ReadFile(filepath.Join(out, "cwd"))
	if wantDir, _ := filepath.EvalSymlinks(filepath.Dir(mpr)); strings.TrimSpace(string(cwd)) != wantDir {
		t.Fatalf("cwd = %q, want %q", cwd, wantDir)
	}
	env, _ := os.ReadFile(filepath.Join(out, "env"))
	if strings.Contains(string(env), "CLAUDE_CODE_SESSION_ID") {
		t.Fatal("the host session's CLAUDE_CODE_* variables reached the agent")
	}
}

// An authentication failure is a result record with is_error, not a crash:
// RunAgent must surface the reason rather than report an empty session.
func TestRunAgentReportsResultError(t *testing.T) {
	claude, _, _, mpr := setupStub(t, true, "Failed to authenticate")
	test := &EvalTest{ID: "T", Prompt: "p", Timeout: time.Minute}
	run, err := RunAgent(test, AgentOptions{RunDir: t.TempDir(), ProjectPath: mpr, ClaudePath: claude})
	if err == nil || !strings.Contains(err.Error(), "Failed to authenticate") {
		t.Fatalf("err = %v", err)
	}
	if run == nil || run.TranscriptPath == "" {
		t.Fatalf("a failed run still keeps its transcript: %+v", run)
	}
}

func TestAgentEnv(t *testing.T) {
	in := []string{"PATH=/bin", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=x", "ANTHROPIC_API_KEY=k", "CLAUDE_CONFIG_DIR=/c"}
	want := []string{"PATH=/bin", "ANTHROPIC_API_KEY=k", "CLAUDE_CONFIG_DIR=/c"}
	if got := AgentEnv(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("AgentEnv = %q", got)
	}
}

func TestBenchmarkBriefParses(t *testing.T) {
	test, err := ParseEvalFile(filepath.Join("..", "..", "..", "docs", "14-eval", "eval-bench-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if test.ID != "BENCH-001" || test.Timeout != 45*time.Minute {
		t.Fatalf("id %s timeout %s", test.ID, test.Timeout)
	}
	types := map[string]bool{}
	for _, c := range test.Checks {
		types[c.Type] = true
	}
	for _, want := range []string{"association_exists", "module_role_exists", "file_exists", "tests_pass", "mx_check_passes"} {
		if !types[want] {
			t.Errorf("benchmark lacks a %s check", want)
		}
	}
}

func TestFileExistsCheck(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "tests"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "tests", "a.test.mdl"), nil, 0o644)
	opts := CheckOptions{ProjectPath: filepath.Join(dir, "App.mpr")}
	if r := checkFileExists(Check{Type: "file_exists", Args: "tests/*.test.mdl"}, opts); !r.Passed {
		t.Fatalf("%+v", r)
	}
	if r := checkFileExists(Check{Type: "file_exists", Args: "tests/*.md"}, opts); r.Passed {
		t.Fatalf("%+v", r)
	}
}
