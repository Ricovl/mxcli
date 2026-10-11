// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func companionDir(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "skills", companionFolder)
}

// The companion is a plugin Claude Code loads from .claude/skills/<name>/ —
// a manifest, the hooks list, and the module. Without all three it is not a mod.
func TestSyncCompanion_WritesTheModForClaudeProjects(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := syncCompanion(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.tsx", "hooks/tree.ts", "types/index.d.ts"} {
		if _, err := os.Stat(filepath.Join(companionDir(dir), filepath.FromSlash(f))); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
		if !slices.Contains(changed, f) {
			t.Errorf("%s written but not reported: %v", f, changed)
		}
	}
	// The mod's own tests are development files; a user project has no use for them.
	if _, err := os.Stat(filepath.Join(companionDir(dir), "tests")); err == nil {
		t.Error("the mod's tests were shipped into the project")
	}
	// A second run finds nothing to do: this runs on every session start.
	again, err := syncCompanion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("second sync changed %v, want nothing", again)
	}
}

// Presence of .claude/ is the signal, as for the .claude/skills mirror: a
// project set up only for another tool gets no Claude Code plugin.
func TestSyncCompanion_SkipsProjectsWithoutClaude(t *testing.T) {
	dir := t.TempDir()
	changed, err := syncCompanion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("changed %v in a project without .claude/", changed)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
		t.Error(".claude/ created in a project without it")
	}
}

// A refresh replaces a stale copy and retires files a newer mod no longer
// ships — inside the mod's own folder only. The types the engine lays beside a
// loaded mod, and everything else in .claude/skills/, are left alone.
func TestSyncCompanion_RefreshesOnlyItsOwnFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := companionDir(dir)
	mustWrite(t, filepath.Join(mod, "hooks", "register.tsx"), "// an older mod\n")
	mustWrite(t, filepath.Join(mod, "hooks", "retired.ts"), "// gone in this version\n")
	mustWrite(t, filepath.Join(mod, ".claude-plugin", "types", "claude-code", "index.d.ts"), "// laid by the engine\n")
	userSkill := filepath.Join(dir, ".claude", "skills", "my-skill", "SKILL.md")
	mustWrite(t, userSkill, "---\nname: my-skill\n---\n")

	changed, err := syncCompanion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changed, "hooks/register.tsx") {
		t.Errorf("stale register.tsx not refreshed: %v", changed)
	}
	if got, _ := os.ReadFile(filepath.Join(mod, "hooks", "register.tsx")); string(got) == "// an older mod\n" {
		t.Error("register.tsx still holds the old content")
	}
	if _, err := os.Stat(filepath.Join(mod, "hooks", "retired.ts")); err == nil {
		t.Error("a file the mod no longer ships was kept")
	}
	if _, err := os.Stat(filepath.Join(mod, ".claude-plugin", "types", "claude-code", "index.d.ts")); err != nil {
		t.Errorf("the engine's types were removed: %v", err)
	}
	if _, err := os.Stat(userSkill); err != nil {
		t.Errorf("a user's skill beside the mod was removed: %v", err)
	}
}
