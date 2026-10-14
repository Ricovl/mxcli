// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// `search --fail-on-match` makes the command a gate: a pipeline can refuse a
// change when a forbidden word turns up anywhere in the project's text. The exit
// status carries the answer (1 = something matched, 0 = nothing did), while the
// output stays what it always was, including a pure JSON array under --json.
//
// These run the real main() in a child process (see runMainStreams), so the exit
// code is the one a shell would see.
func TestSearchFailOnMatch_ExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	cases := []struct {
		name     string
		query    string
		wantCode int
		wantHits bool
	}{
		{"a hit exits 1", "User", 1, true},
		{"no hit exits 0", "zzqqxxnomatch", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"-p", mpr, "--json", "search", tc.query, "--fail-on-match"}
			r := runMainStreams(t, dir, args...)
			if r.exitCode != tc.wantCode {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.exitCode, tc.wantCode, r.stdout, r.stderr)
			}
			assertPureJSON(t, args, r)
			var rows []map[string]any
			if err := json.Unmarshal([]byte(r.stdout), &rows); err != nil {
				t.Fatalf("stdout is not a JSON array: %v\n%s", err, r.stdout)
			}
			if (len(rows) > 0) != tc.wantHits {
				t.Errorf("rows = %d, want hits = %v\n%s", len(rows), tc.wantHits, r.stdout)
			}
		})
	}
}

// Without the flag a hit is still exit 0: the flag is opt-in, and every script
// that already calls `search` keeps working.
func TestSearchWithoutFailOnMatch_HitExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	r := runMainStreams(t, filepath.Dir(mpr), "-p", mpr, "--json", "search", "User")
	if r.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", r.exitCode, r.stderr)
	}
}

// A query the index cannot run is neither a hit nor a clean bill. Under the gate
// it exits 2, so 1 keeps meaning "something matched" and a failing search is
// never mistaken for a passing one.
func TestSearchFailOnMatch_UnrunnableQueryExitsTwo(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	r := runMainStreams(t, filepath.Dir(mpr), "-p", mpr, "--json", "search", `a"b`, "--fail-on-match")
	if r.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout:\n%s\nstderr:\n%s", r.exitCode, r.stdout, r.stderr)
	}
}
