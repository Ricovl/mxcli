// SPDX-License-Identifier: Apache-2.0

// `mxcli diff-local --ref main...feature-branch` is documented ("changes since
// common ancestor") and printed, on a git copy of testdata/pedapp:
//
//	Warning: failed to read .grant version of mprcontents/84/77/….mxunit: exit status 128
//
//	Summary: 0 new, 0 modified, 0 deleted
//
// parseRefRange split on the first "..", so "a...b" became base "a" and target
// ".b". For `git diff a...b` the old side is the merge base of a and b, not a.
package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseRefRange(t *testing.T) {
	for _, tc := range []struct {
		ref                string
		base, target       string
		isRange, mergeBase bool
	}{
		{"HEAD", "", "HEAD", false, false},
		{"main", "", "main", false, false},
		{"HEAD~1..HEAD", "HEAD~1", "HEAD", true, false},
		{"main...feature", "main", "feature", true, true},
		// An empty side is HEAD, as it is to git.
		{"main..", "main", "HEAD", true, false},
		{"..feature", "HEAD", "feature", true, false},
		{"main...", "main", "HEAD", true, true},
	} {
		base, target, isRange, mergeBase := parseRefRange(tc.ref)
		if base != tc.base || target != tc.target || isRange != tc.isRange || mergeBase != tc.mergeBase {
			t.Errorf("parseRefRange(%q) = (%q, %q, %v, %v), want (%q, %q, %v, %v)", tc.ref,
				base, target, isRange, mergeBase, tc.base, tc.target, tc.isRange, tc.mergeBase)
		}
	}
}

// divergedRepo commits a constant, branches `feature` off that commit and
// changes the constant there, then changes it differently on main. Only the
// merge base holds the value the feature branch started from.
func divergedRepo(t *testing.T) (proj string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	proj = t.TempDir()
	unit := filepath.Join(proj, "mprcontents", "5a", "5a", fld1602Unit+".mxunit")
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(def string) {
		data, err := bson.Marshal(map[string]any{"$Type": "Constants$Constant", "Name": "ApiKey", "DefaultValue": def})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(unit, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = proj
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write("x")
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "feature")
	write("feat")
	git("commit", "-q", "-am", "feature")
	git("checkout", "-q", "main")
	write("mainval")
	git("commit", "-q", "-am", "main moves on")
	return proj
}

func TestDiffLocal_ThreeDotRangeDiffsFromTheMergeBase(t *testing.T) {
	proj := divergedRepo(t)
	ctx, out := set1602Context()
	ctx.Backend.(*mock.MockBackend).ContentsDirFunc = func() string { return filepath.Join(proj, "mprcontents") }

	if err := diffLocal(ctx, "main...feature", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "Warning") || !strings.Contains(got, "1 modified") {
		t.Fatalf("diff-local --ref main...feature did not diff the feature branch's change:\n%s", got)
	}
	// "x" (1 char) at the merge base, "feat" (4) on feature. Reading the old
	// side at main would give "mainval" (7).
	if !strings.Contains(got, "DefaultValue (string, 1 → 4 chars)") {
		t.Errorf("the old side is not the merge base of main and feature:\n%s", got)
	}

	// Control: two dots compare the tips, main's value against feature's.
	ctx, out = set1602Context()
	ctx.Backend.(*mock.MockBackend).ContentsDirFunc = func() string { return filepath.Join(proj, "mprcontents") }
	if err := diffLocal(ctx, "main..feature", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "DefaultValue (string, 7 → 4 chars)") {
		t.Errorf("control: main..feature does not compare main's tip with feature's:\n%s", got)
	}
}

// A unit whose file moves to another name is a different unit to Mendix — the
// file name is the unit ID — but git's rename detection reported it as
// "R100\told\tnew", which diff-local read as a modification of the OLD path:
// the new revision has no such file, so the unit was skipped with a warning and
// counted nowhere.
func TestDiffLocal_RenamedUnitFileIsADeleteAndAnAdd(t *testing.T) {
	proj := divergedRepo(t)
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = proj
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	const moved = "6b6b6b6b-1111-2222-3333-444444444444"
	if err := os.MkdirAll(filepath.Join(proj, "mprcontents", "6b", "6b"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("mv", filepath.Join("mprcontents", "5a", "5a", fld1602Unit+".mxunit"),
		filepath.Join("mprcontents", "6b", "6b", moved+".mxunit"))
	git("commit", "-q", "-m", "unit gets a new ID")

	ctx, out := set1602Context()
	ctx.Backend.(*mock.MockBackend).ContentsDirFunc = func() string { return filepath.Join(proj, "mprcontents") }
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "Warning") || !strings.Contains(got, "Summary: 1 new, 0 modified, 1 deleted") {
		t.Errorf("a unit file moved to a new ID is not reported as one new and one deleted unit:\n%s", got)
	}
}
