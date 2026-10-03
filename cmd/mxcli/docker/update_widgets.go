// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// updateWidgetsPathArg returns an absolute form of the .mpr path for the
// `mx update-widgets` invocation. MxToolset's AddProjectDirAsAllowedPath computes
// Path.GetDirectoryName(mprFilePath) to whitelist the project directory; given a
// bare filename (e.g. "app.mpr", as passed by `mxcli docker build -p app.mpr` run
// from the project dir) that returns "" → null and the tool throws
// System.ArgumentNullException, silently skipping the widget migration. That in
// turn leaves CE0463 "widget definition changed" errors unresolved at check time.
// An absolute path always has a directory component. `mx check` is unaffected, so
// only the update-widgets arg is normalized. Falls back to the input if Abs fails.
func updateWidgetsPathArg(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// updateWidgetsCmd runs the mx invocation. It is a package variable so tests can
// substitute a stub that simulates the v2 -> v1 conversion without needing mx.
var updateWidgetsCmd = func(mxPath, pathArg string, w, stderr io.Writer) error {
	cmd := exec.Command(mxPath, "update-widgets", pathArg)
	cmd.Stdout = w
	cmd.Stderr = stderr
	PrepareMxCommand(cmd)
	return cmd.Run()
}

// snapshotStorageFormat backs up the MPRv2 storage files (.mpr index + mprcontents/)
// to a temp directory and returns a restore function that puts them back, undoing
// any v2 -> v1 conversion performed by an intervening `mx update-widgets`. The
// restore function removes the temp directory and is safe to defer; it best-effort
// restores and never panics. mprPath and contentsDir come from a backend on a
// project already known to be MPRv2.
func snapshotStorageFormat(mprPath, contentsDir string) (dir string, restore func(), err error) {
	tmp, err := os.MkdirTemp("", "mxcli-mpr-snapshot-*")
	if err != nil {
		return "", nil, err
	}

	mprBackup := filepath.Join(tmp, filepath.Base(mprPath))
	if err := copyFile(mprPath, mprBackup); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}

	contentsBackup := filepath.Join(tmp, "mprcontents")
	if err := copyDir(contentsDir, contentsBackup); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}

	restore = func() {
		defer os.RemoveAll(tmp)
		// Restore the v2 index file.
		_ = copyFile(mprBackup, mprPath)
		// update-widgets deletes mprcontents/; drop whatever is there now (nothing,
		// after a conversion) and restore the backed-up tree.
		_ = os.RemoveAll(contentsDir)
		_ = copyDir(contentsBackup, contentsDir)
	}
	return tmp, restore, nil
}
