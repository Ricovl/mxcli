// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1602 (ako/mxcli#1512), side note: "`diff-local` runs `git
// diff` in the process working directory, not in the project's repo. Run from
// outside the repo with `-p <app>/App.mpr`, it fails with `Error: failed to
// find changed files: failed to git diff: exit status 1`".
package executor

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const gitUnit = "5a5a5a5a-1111-2222-3333-444444444444"

// gitDiffContext is a connected v2 project whose mprcontents is contentsDir.
// An enumeration renders its values, so a test can see which revision each
// side was read at.
func gitDiffContext(contentsDir string) (*ExecContext, *bytes.Buffer) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		VersionFunc:     func() types.MPRVersion { return 2 },
		ContentsDirFunc: func() string { return contentsDir },
	}
	out := &bytes.Buffer{}
	return &ExecContext{Context: context.Background(), Backend: mb, Output: out, Cache: &executorCache{}}, out
}

// writeEnumUnit writes an enumeration unit with one value to proj's mprcontents.
func writeEnumUnit(t *testing.T, proj, value string) {
	t.Helper()
	unit := filepath.Join(proj, "mprcontents", "5a", "5a", gitUnit+".mxunit")
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := bson.Marshal(map[string]any{
		"$Type":  "Enumerations$Enumeration",
		"Name":   "Status",
		"Values": []any{int32(3), map[string]any{"$Type": "Enumerations$EnumerationValue", "Name": value}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitIn runs git in dir with a throwaway identity, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitProjectRepo commits an enumeration under <root>/App/mprcontents, then
// changes its value in the working tree. It returns the repo root and the
// project directory.
func gitProjectRepo(t *testing.T) (root, proj string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root = t.TempDir()
	proj = filepath.Join(root, "App")
	writeEnumUnit(t, proj, "Before")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	writeEnumUnit(t, proj, "After")
	return root, proj
}

func TestDiffLocal_RunsGitInTheProjectRepo(t *testing.T) {
	for _, tc := range []struct {
		name string
		cwd  func(root, proj string) string
		dir  func(root, proj string) string // what ContentsDir reports
	}{
		{"cwd outside any repo", func(_, _ string) string { return t.TempDir() },
			func(_, proj string) string { return filepath.Join(proj, "mprcontents") }},
		{"cwd in the project, below the repo root", func(_, proj string) string { return proj },
			func(_, _ string) string { return "mprcontents" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, proj := gitProjectRepo(t)
			t.Chdir(tc.cwd(root, proj))
			ctx, out := gitDiffContext(tc.dir(root, proj))

			if err := diffLocal(ctx, "HEAD", DiffOptions{}); err != nil {
				t.Fatalf("diff-local from %s: %v (#1602)", tc.name, err)
			}
			got := out.String()
			if !strings.Contains(got, "-  Before 'Before'") || !strings.Contains(got, "+  After 'After'") {
				t.Errorf("diff-local from %s did not diff the changed unit:\n%s", tc.name, got)
			}
		})
	}
}
