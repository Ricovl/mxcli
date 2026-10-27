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

// divergedRepo commits an enumeration, branches `feature` off that commit and
// changes it there, then changes it differently on main. Only the merge base
// holds the value the feature branch started from.
func divergedRepo(t *testing.T) (proj string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	proj = t.TempDir()
	writeEnumUnit(t, proj, "Base")
	gitIn(t, proj, "init", "-q")
	gitIn(t, proj, "add", "-A")
	gitIn(t, proj, "commit", "-q", "-m", "base")
	gitIn(t, proj, "checkout", "-q", "-b", "feature")
	writeEnumUnit(t, proj, "Feat")
	gitIn(t, proj, "commit", "-q", "-am", "feature")
	gitIn(t, proj, "checkout", "-q", "main")
	writeEnumUnit(t, proj, "Mainval")
	gitIn(t, proj, "commit", "-q", "-am", "main moves on")
	return proj
}

func TestDiffLocal_ThreeDotRangeDiffsFromTheMergeBase(t *testing.T) {
	proj := divergedRepo(t)
	ctx, out := gitDiffContext(filepath.Join(proj, "mprcontents"))

	if err := diffLocal(ctx, "main...feature", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "Warning") || !strings.Contains(got, "1 modified") {
		t.Fatalf("diff-local --ref main...feature did not diff the feature branch's change:\n%s", got)
	}
	// The merge base holds "Base"; reading the old side at main would give
	// "Mainval".
	if !strings.Contains(got, "-  Base 'Base'") || !strings.Contains(got, "+  Feat 'Feat'") ||
		strings.Contains(got, "Mainval") || !strings.Contains(got, "(merge base of main and feature)") {
		t.Errorf("the old side is not the merge base of main and feature:\n%s", got)
	}

	// Control: two dots compare the tips, main's value against feature's.
	ctx, out = gitDiffContext(filepath.Join(proj, "mprcontents"))
	if err := diffLocal(ctx, "main..feature", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "-  Mainval 'Mainval'") || !strings.Contains(got, "+  Feat 'Feat'") {
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
	const moved = "6b6b6b6b-1111-2222-3333-444444444444"
	if err := os.MkdirAll(filepath.Join(proj, "mprcontents", "6b", "6b"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, proj, "mv", filepath.Join("mprcontents", "5a", "5a", gitUnit+".mxunit"),
		filepath.Join("mprcontents", "6b", "6b", moved+".mxunit"))
	gitIn(t, proj, "commit", "-q", "-m", "unit gets a new ID")

	ctx, out := gitDiffContext(filepath.Join(proj, "mprcontents"))
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "Warning") || !strings.Contains(got, "Summary: 1 new, 0 modified, 1 deleted") {
		t.Errorf("a unit file moved to a new ID is not reported as one new and one deleted unit:\n%s", got)
	}
}
