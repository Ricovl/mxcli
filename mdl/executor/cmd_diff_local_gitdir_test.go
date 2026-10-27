// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1602 (ako/mxcli#1512), side note: "`diff-local` runs `git
// diff` in the process working directory, not in the project's repo. Run from
// outside the repo with `-p <app>/App.mpr`, it fails with `Error: failed to
// find changed files: failed to git diff: exit status 1`".
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

// gitProjectRepo commits a constant unit under <root>/App/mprcontents, then
// changes its default value in the working tree. It returns the repo root and
// the project directory.
func gitProjectRepo(t *testing.T) (root, proj string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root = t.TempDir()
	proj = filepath.Join(root, "App")
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
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write("x")
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	write("secret123")
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
			ctx, out := set1602Context()
			dir := tc.dir(root, proj)
			ctx.Backend.(*mock.MockBackend).ContentsDirFunc = func() string { return dir }

			if err := diffLocal(ctx, "HEAD", DiffOptions{}); err != nil {
				t.Fatalf("diff-local from %s: %v (#1602)", tc.name, err)
			}
			if got := out.String(); !strings.Contains(got, "DefaultValue (string, 1 → 9 chars)") {
				t.Errorf("diff-local from %s did not diff the changed unit:\n%s", tc.name, got)
			}
		})
	}
}
