// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// mxMaxPath is the path length mx refuses to write a file at or beyond. It is
// Windows' MAX_PATH, but mx enforces it itself (Mendix's
// FileSystem.OpenNewFileForWriting throws PathTooLongException), so it holds on
// Linux and macOS too. Measured with mx 11.12.1 on Linux: a 259-character file
// path is written, a 260-character one is refused.
const mxMaxPath = 260

// NewWorkDir creates the scratch directory a command builds its reference
// projects in, under the shortest root the user owns.
//
// Length is the whole point. mx writes every file of the blank template and of
// the imported package beneath this directory, and the deepest of those are a
// bundled node_modules tree: 181 characters for the 11.12.1 blank app, about
// 202 for NanoflowCommons 7.x. Under the default Windows %TEMP%
// (C:\Users\<user>\AppData\Local\Temp) and the old
// \mxdiff<10 digits>\target-ref beneath it, that left no room for a user name
// at all (mendixlabs/mxcli#1486).
//
// The caller removes the directory. Name its contents in a letter or two: every
// character spent here is one less for the package's own paths.
func NewWorkDir(prefix string) (string, error) {
	home, _ := os.UserHomeDir()
	root := workRoot(runtime.GOOS, os.TempDir(), home)
	if err := os.MkdirAll(root, 0o700); err == nil {
		if dir, err := os.MkdirTemp(root, prefix); err == nil {
			return dir, nil
		}
	}
	return os.MkdirTemp("", prefix)
}

// workRoot picks the shortest of the roots a work directory may go under.
//
// Every candidate is private to the user, which rules out the obvious short
// one: a directory at a Windows drive root inherits Modify for all
// authenticated users from C:\, so another account could swap the downloaded
// package between its download and its import. So on Windows the alternative to
// %TEMP% is ~\.mxcli\t, beside the caches mxcli already keeps there; on POSIX
// it is /tmp, where os.MkdirTemp makes a 0700 directory (macOS's per-user
// $TMPDIR is about 49 characters on its own). An explicitly shorter TMP or
// TMPDIR still wins, so pointing it somewhere short remains the way out.
func workRoot(goos, tempDir, home string) string {
	candidates := []string{tempDir}
	switch {
	case goos == "windows" && home != "":
		candidates = append(candidates, strings.TrimRight(home, `\`)+`\.mxcli\t`)
	case goos != "windows":
		candidates = append(candidates, "/tmp")
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if len(c) < len(best) {
			best = c
		}
	}
	return best
}

// mxFailure wraps a failed mx invocation, explaining a path-length refusal.
// mx's own message names neither the limit nor that the directory was mxcli's
// choice, so it reads as a broken install.
func mxFailure(step string, err error, out []byte, workDir string) error {
	msg := strings.TrimSpace(string(out))
	if !strings.Contains(msg, "path is too long") && !strings.Contains(msg, "PathTooLongException") {
		return fmt.Errorf("mx %s failed: %w\n%s", step, err, msg)
	}
	return fmt.Errorf("mx %s failed: %w\n%s\n"+
		"hint: mx refuses to write any file whose path is %d characters or longer, on every platform.\n"+
		"  It was writing under %s (%d characters), which leaves %d for the package's own paths.\n"+
		"  Point TMP (Windows) or TMPDIR at a shorter directory you own and re-run.",
		step, err, msg, mxMaxPath, workDir, len(workDir), mxMaxPath-1-len(workDir)-1)
}
