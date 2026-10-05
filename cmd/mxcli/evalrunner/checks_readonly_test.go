// SPDX-License-Identifier: Apache-2.0

package evalrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stubMx does to the project it checks what the real `mx check` does — writes
// theme-cache/ and deployment/sass/ — and reports the path it was given.
const stubMx = `#!/bin/sh
[ "$1" = check ] || exit 2
dir=$(dirname "$2")
mkdir -p "$dir/theme-cache/web" "$dir/deployment/sass"
echo recompiled > "$dir/theme-cache/web/theme.compiled.css"
echo x > "$dir/deployment/sass/main.scss"
echo "Loading $2"
[ -z "$MX_STUB_FAIL" ] || exit 1
`

func treeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			m[rel+"/"] = "dir"
			return nil
		}
		b, _ := os.ReadFile(p)
		info, _ := d.Info()
		sum := sha256.Sum256(b)
		m[rel] = hex.EncodeToString(sum[:]) + " " + info.ModTime().UTC().Format(time.RFC3339Nano)
		return nil
	})
	return m
}

// TestCheckMxCheck_DoesNotModifyProject: the eval runner's mx-check check must
// not write theme-cache/ or deployment/ into the project it grades
// (ako/mxcli#961), and still reports pass/fail and the project's own path.
func TestCheckMxCheck_DoesNotModifyProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub mx is a shell script")
	}
	mx := filepath.Join(t.TempDir(), "mx")
	if err := os.WriteFile(mx, []byte(stubMx), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", t.TempDir())
	for _, tc := range []struct {
		format string
		fail   bool
	}{{"v1", false}, {"v2", false}, {"v1", true}, {"v2", true}} {
		t.Run(fmt.Sprintf("%s/fail=%v", tc.format, tc.fail), func(t *testing.T) {
			if tc.fail {
				t.Setenv("MX_STUB_FAIL", "1")
			}
			proj := t.TempDir()
			files := []string{"App.mpr", "theme/web/main.scss"}
			if tc.format == "v2" {
				files = append(files, "mprcontents/ab/cd/abcd.mxunit")
			}
			for _, f := range files {
				p := filepath.Join(proj, f)
				os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			mpr := filepath.Join(proj, "App.mpr")
			before := treeHashes(t, proj)

			res := checkMxCheck(Check{}, CheckOptions{ProjectPath: mpr, MxPath: mx})
			if res.Passed == tc.fail {
				t.Errorf("Passed = %v, want %v (%s)", res.Passed, !tc.fail, res.Detail)
			}
			if tc.fail && !strings.Contains(res.Detail, "Loading "+mpr) {
				t.Errorf("detail does not name the project: %q", res.Detail)
			}

			after := treeHashes(t, proj)
			var diff []string
			for k, v := range after {
				if before[k] != v {
					diff = append(diff, k)
				}
			}
			for k := range before {
				if _, ok := after[k]; !ok {
					diff = append(diff, "removed "+k)
				}
			}
			if len(diff) > 0 {
				t.Errorf("the eval runner's mx check modified the project: %s", strings.Join(diff, ", "))
			}
		})
	}
}
