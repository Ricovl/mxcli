package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1444: a command run without -p picked up a stray .mpr in the
// working directory with only a bare "Using project: App.mpr", which does not
// say the project was guessed, where it is, or how to choose another. With
// several .mpr files it resolved nothing and said nothing about why.
func TestDiscoverProject_NamesTheGuessAndRefusesAmbiguity(t *testing.T) {
	touch := func(t *testing.T, dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("single .mpr is used and named by absolute path", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "Stray.mpr")
		path, note := discoverProject(dir)
		if path != "Stray.mpr" {
			t.Fatalf("path = %q, want the file name (flag value unchanged)", path)
		}
		abs := filepath.Join(dir, "Stray.mpr")
		if !strings.Contains(note, abs) {
			t.Errorf("notice %q does not name the absolute path %q", note, abs)
		}
		if !strings.Contains(note, "auto-detected") || !strings.Contains(note, "-p") {
			t.Errorf("notice %q does not say the project was auto-detected and how to override it", note)
		}
	})

	t.Run("several .mpr files are refused, listed, and -p is asked for", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "A.mpr")
		touch(t, dir, "B.mpr")
		path, note := discoverProject(dir)
		if path != "" {
			t.Fatalf("path = %q, want none when ambiguous", path)
		}
		for _, want := range []string{"A.mpr", "B.mpr", "-p"} {
			if !strings.Contains(note, want) {
				t.Errorf("note %q does not mention %q", note, want)
			}
		}
	})

	t.Run("no .mpr says nothing", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "notes.txt")
		if err := os.Mkdir(filepath.Join(dir, "dir.mpr"), 0o755); err != nil {
			t.Fatal(err)
		}
		path, note := discoverProject(dir)
		if path != "" || note != "" {
			t.Fatalf("got (%q, %q), want nothing", path, note)
		}
	})
}
