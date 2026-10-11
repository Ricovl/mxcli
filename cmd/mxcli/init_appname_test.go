// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The project name in generated docs is the app's, from the .mpr — not the
// repository folder's, which in a cloud session is whatever the repo was
// called (a seed repo named "seed" titled its CLAUDE.md "Mendix Project: seed").
func TestProjectDisplayName(t *testing.T) {
	cases := []struct{ dir, mpr, want string }{
		{"/work/seed", "ChipCoV8.mpr", "ChipCoV8"},
		{"/work/seed", "", "seed"},
		{"/work/seed", ".mpr", "seed"},
		{"/work/App", "App.mpr", "App"},
	}
	for _, c := range cases {
		if got := projectDisplayName(c.dir, c.mpr); got != c.want {
			t.Errorf("projectDisplayName(%q, %q) = %q, want %q", c.dir, c.mpr, got, c.want)
		}
	}
}

func TestInit_UsesAppNameNotFolderName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "ChipCoV8.mpr"), "")
	runInit(t, []string{"claude"}, dir)

	claude := mustRead(t, filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(claude, "# Mendix Project: ChipCoV8\n") {
		t.Errorf("CLAUDE.md heading does not name the app:\n%s", claude[:min(len(claude), 400)])
	}
	if strings.Contains(claude, "Mendix Project: seed") {
		t.Error("CLAUDE.md is titled after the repository folder")
	}
	dc := mustRead(t, filepath.Join(dir, ".devcontainer", "devcontainer.json"))
	if !strings.Contains(dc, `"name": "ChipCoV8"`) {
		t.Errorf("devcontainer name is not the app name:\n%s", dc)
	}
}

// A later `init --sync-skills` must not retitle the doc after the folder.
func TestSyncProjectTooling_UsesAppName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "ChipCoV8.mpr"), "")
	path := filepath.Join(dir, "CLAUDE.md")
	mustWrite(t, path, wrapMxcliSection("# Mendix Project: old\n"))
	if _, err := syncProjectTooling(dir); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); !strings.Contains(got, "# Mendix Project: ChipCoV8\n") {
		t.Errorf("synced CLAUDE.md heading does not name the app:\n%s", got[:min(len(got), 400)])
	}
}
