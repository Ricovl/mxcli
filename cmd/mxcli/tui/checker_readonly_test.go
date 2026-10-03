// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// stubMxScript is an `mx` that does to the project it checks what the real one
// does — it writes theme-cache/ and deployment/sass/ — and writes a JSON result
// with one error to the -j file.
const stubMxScript = `#!/bin/sh
[ "$1" = check ] || exit 2
dir=$(dirname "$2")
mkdir -p "$dir/theme-cache/web" "$dir/deployment/sass"
echo recompiled > "$dir/theme-cache/web/theme.compiled.css"
echo x > "$dir/deployment/sass/main.scss"
shift 2
while [ $# -gt 0 ]; do
  if [ "$1" = -j ]; then
    echo '{"errors":[{"code":"CE0001","message":"stub error","module-name":"M","document-name":"Page '"'"'P'"'"'"}]}' > "$2"
    shift
  fi
  shift
done
exit 1
`

// treeHashes records each file's hash and mtime under root.
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

// TestRunMxCheck_DoesNotModifyProject: the TUI checker runs mx check after every
// change; it must not write theme-cache/ or deployment/ into the project
// (ako/mxcli#961). Measured with the real mx 11.14 on a v1 and a v2 project:
// a plain `mx check` leaves the model alone but adds theme-cache/web/ and
// deployment/sass/ in both formats, so a stub that does the same is enough here.
func TestRunMxCheck_DoesNotModifyProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub mx is a shell script")
	}
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "mx"), []byte(stubMxScript), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TMPDIR", t.TempDir())

			proj := t.TempDir()
			files := []string{"App.mpr", "theme/web/main.scss", "widgets/W.mpk"}
			if format == "v2" {
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

			var result MxCheckResultMsg
			batch := runMxCheck(mpr)().(tea.BatchMsg)
			for _, c := range batch {
				if m, ok := c().(MxCheckResultMsg); ok {
					result = m
				}
			}
			if result.Err != nil {
				t.Fatalf("check: %v", result.Err)
			}
			if len(result.Errors) != 1 || result.Errors[0].Code != "CE0001" {
				t.Errorf("errors = %+v, want the stub's CE0001", result.Errors)
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
				t.Errorf("the TUI checker modified the project: %s", strings.Join(diff, ", "))
			}
		})
	}
}
