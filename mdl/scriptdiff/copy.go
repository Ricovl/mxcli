// SPDX-License-Identifier: Apache-2.0

package scriptdiff

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// skippedDirs are directories of a project folder the scratch copy leaves out:
// version-control metadata, build output and package caches. Exec reads none of
// them and writes none of them, and together they are most of a project folder's
// size once it has been built or deployed.
var skippedDirs = map[string]bool{
	".git":           true,
	".svn":           true,
	"deployment":     true,
	"releases":       true,
	"node_modules":   true,
	".mendix-cache":  true,
	".mendix-cache2": true,
}

// copyProject copies the project folder src into dst, which must not exist.
//
// Everything is a real copy rather than a link: exec writes the .mpr, the unit
// files and, for some statements, files next to the project (java and
// javascript sources, theme files), and a hard link or a symlinked directory
// would carry such a write back into the project being diffed. A symlink in the
// project is copied as a symlink, which is the one exception, and the same one
// `cp -a` makes.
func copyProject(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch mode := info.Mode(); {
		case mode&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			if rel != "." && skippedDirs[info.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		case mode.IsRegular():
			return copyFile(p, target, mode.Perm())
		default:
			return nil // sockets, devices: nothing exec reads
		}
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return out.Close()
}
