// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// init_companion.go ships the mxcli companion — a Claude Code mod that shows
// the project tree and MDL source in a pane — into a project
// (docs/11-proposals/proposal-mxcli-companion-mod.md, slice 1).
//
// Claude Code loads a plugin folder under the project's .claude/skills/<name>/
// in place, once the workspace is trusted, with no install step. So the mod is
// written there, beside the skills, and refreshed with them on every session
// start. Its source is mods/companion/ in the mxcli repo, embedded through
// cmd/mxcli/mods/ by `make sync-mods`; it is kept out of .claude/skills/mendix/
// so the skills sync neither ships it to other tools' folders nor deletes it.

// companionFolder is the mod's folder under .claude/skills/. It must not be the
// name of a skill.
const companionFolder = "mxcli-companion"

// companionKeep are paths inside the mod folder the sync never removes: the
// engine lays the API types and a tsconfig beside a mod it has loaded.
var companionKeep = []string{".claude-plugin/types/", "tsconfig.json"}

// syncCompanion writes the embedded mod into .claude/skills/mxcli-companion/
// when the project is set up for Claude Code (.claude/ exists, the same signal
// skillDests uses), returning the paths it wrote or removed, relative to the
// mod folder. It writes only what differs, so an unchanged project reports
// nothing — this runs on every session start.
func syncCompanion(projectDir string) ([]string, error) {
	if st, err := os.Stat(filepath.Join(projectDir, ".claude")); err != nil || !st.IsDir() {
		return nil, nil
	}
	dest := filepath.Join(projectDir, ".claude", "skills", companionFolder)
	const root = "mods/companion"

	var changed []string
	shipped := map[string]bool{}
	err := fs.WalkDir(modsFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		shipped[rel] = true
		want, err := modsFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", p, err)
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if have, readErr := os.ReadFile(target); readErr == nil && bytes.Equal(have, want) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, want, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		changed = append(changed, rel)
		return nil
	})
	if err != nil {
		return changed, err
	}
	if len(shipped) == 0 {
		return nil, fmt.Errorf("no embedded companion mod: cmd/mxcli/mods is empty or stale; " +
			"it is generated from mods/ — run `make sync-mods` (or `make build`) first")
	}

	// Retire files an older mod shipped and this one does not.
	err = filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dest, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shipped[rel] {
			return nil
		}
		for _, keep := range companionKeep {
			if rel == keep || (strings.HasSuffix(keep, "/") && strings.HasPrefix(rel, keep)) {
				return nil
			}
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		changed = append(changed, rel)
		return nil
	})
	sort.Strings(changed)
	return changed, err
}
