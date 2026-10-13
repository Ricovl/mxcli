// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mxcli-formula1 findings #12: `ALTER MODULE X ADD JAR DEPENDENCY (…)` writes the
// coordinate and `list jar dependencies` reports it, but nothing puts the jar on
// the classpath. Measured on 11.12.1: a full `mxbuild --target=deploy` emits a
// build.gradle with no dependencies block and downloads nothing, while
// `mx sync-java-dependencies` fetches it into vendorlib/ — resolution is a
// separate step. Knowing which coordinates are unvendored is what lets a caller
// run that step only when it is needed.
func TestUnvendoredJarDependencies(t *testing.T) {
	dir := t.TempDir()
	vendorlib := filepath.Join(dir, "vendorlib")
	if err := os.MkdirAll(vendorlib, 0o755); err != nil {
		t.Fatal(err)
	}
	// mx writes <artifact>-<version>.jar.
	if err := os.WriteFile(filepath.Join(vendorlib, "commons-lang3-3.14.0.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	deps := []JarDependencyRef{
		{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.14.0"}, // present
		{Group: "org.duckdb", Artifact: "duckdb_jdbc", Version: "1.5.5.1"},          // missing
	}

	got := UnvendoredJarDependencies(dir, deps)
	if len(got) != 1 || got[0] != "org.duckdb:duckdb_jdbc:1.5.5.1" {
		t.Fatalf("got %v, want only org.duckdb:duckdb_jdbc:1.5.5.1", got)
	}
}

// A different version of a vendored artifact is still missing: the file name
// carries the version, and the wrong jar is not the right jar.
func TestUnvendoredJarDependencies_WrongVersionVendored(t *testing.T) {
	dir := t.TempDir()
	writeVendoredJar(t, dir, "commons-lang3-3.14.0.jar")
	deps := []JarDependencyRef{{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.12.0"}}
	got := UnvendoredJarDependencies(dir, deps)
	if len(got) != 1 || got[0] != "org.apache.commons:commons-lang3:3.12.0" {
		t.Errorf("got %v, want the declared 3.12.0", got)
	}
}

// mendixlabs/mxcli#1450: two modules declare the same artifact at different
// versions (ExcelImporter commons-lang3 3.18.0, CommunityCommons 3.20.0).
// `mx sync-java-dependencies` runs Gradle, whose conflict resolution keeps the
// newest, so vendorlib/ holds 3.20.0 only. Checking each declaration on its own
// reported "missing  org.apache.commons:commons-lang3:3.18.0" forever: the gate
// failed, sync deleted the jar and then errored "still missing", and
// `run --local` re-synced on every start.
func TestUnvendoredJarDependencies_ConflictResolvesToNewest(t *testing.T) {
	dir := t.TempDir()
	writeVendoredJar(t, dir, "commons-lang3-3.20.0.jar")
	deps := []JarDependencyRef{
		{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.18.0"},
		{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.20.0"},
	}
	if got := UnvendoredJarDependencies(dir, deps); len(got) != 0 {
		t.Errorf("superseded declaration reported as missing: %v", got)
	}

	// The winner itself is still required: with nothing vendored, the newest
	// version is the one missing — never the superseded one.
	empty := t.TempDir()
	got := UnvendoredJarDependencies(empty, deps)
	if len(got) != 1 || got[0] != "org.apache.commons:commons-lang3:3.20.0" {
		t.Errorf("got %v, want only the resolved 3.20.0", got)
	}
}

func TestSupersededJarDependencies(t *testing.T) {
	deps := []JarDependencyRef{
		{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.20.0"},
		{Group: "org.apache.commons", Artifact: "commons-lang3", Version: "3.18.0"},
		{Group: "org.duckdb", Artifact: "duckdb_jdbc", Version: "1.5.5.1"},
		// Same version declared twice is one requirement, not a conflict.
		{Group: "org.duckdb", Artifact: "duckdb_jdbc", Version: "1.5.5.1"},
	}
	got := SupersededJarDependencies(deps)
	want := "org.apache.commons:commons-lang3:3.18.0 (resolved to 3.20.0)"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want [%s]", got, want)
	}
}

// Gradle's version ordering, which decides the conflict winner. A plain string
// comparison gets the first case wrong ("3.9.0" > "3.20.0").
func TestCompareJarVersions(t *testing.T) {
	cases := []struct{ a, b string }{ // a < b
		{"3.9.0", "3.20.0"},
		{"3.18.0", "3.20.0"},
		{"1.0", "1.0.1"},
		{"1.0-beta", "1.0"},
		{"1.0-rc1", "1.0"},
		{"1.0-alpha", "1.0-beta"},
		{"1.0-dev", "1.0-alpha"},
		{"1.0-SNAPSHOT", "1.0"},
		{"1.0-rc", "1.0-SNAPSHOT"},
		{"1.0.a", "1.0.1"},
		{"1.5.5", "1.5.5.1"},
	}
	for _, c := range cases {
		if compareJarVersions(c.a, c.b) >= 0 {
			t.Errorf("want %s < %s", c.a, c.b)
		}
		if compareJarVersions(c.b, c.a) <= 0 {
			t.Errorf("want %s > %s", c.b, c.a)
		}
	}
	for _, v := range []string{"1.0", "3.20.0", "1.0-beta"} {
		if compareJarVersions(v, v) != 0 {
			t.Errorf("want %s == %s", v, v)
		}
	}
}

func writeVendoredJar(t *testing.T, dir, name string) {
	t.Helper()
	vendorlib := filepath.Join(dir, "vendorlib")
	if err := os.MkdirAll(vendorlib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendorlib, name), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An incomplete coordinate cannot be checked or resolved, and must not be
// reported as missing — that would send the caller into a sync that cannot help.
func TestUnvendoredJarDependencies_SkipsIncompleteCoordinates(t *testing.T) {
	dir := t.TempDir()
	deps := []JarDependencyRef{
		{Group: "g", Artifact: "", Version: "1.0"},
		{Group: "g", Artifact: "a", Version: ""},
	}
	if got := UnvendoredJarDependencies(dir, deps); len(got) != 0 {
		t.Errorf("expected incomplete coordinates to be skipped, got: %v", got)
	}
}

// No vendorlib/ at all is the fresh-clone case: everything declared is missing.
func TestUnvendoredJarDependencies_NoVendorlib(t *testing.T) {
	deps := []JarDependencyRef{{Group: "g", Artifact: "a", Version: "1.0"}}
	if got := UnvendoredJarDependencies(t.TempDir(), deps); len(got) != 1 {
		t.Errorf("got %v, want the one declared dependency", got)
	}
}

// mx's own failure text has to reach the caller. This whole finding was a
// silent failure, so a sync that does not work must not look like one that did.
func TestSyncJavaDependencies_SurfacesMxFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "NoSuch.mpr")
	err := SyncJavaDependencies(missing, "", "0.0.0-not-a-version", nil)
	if err == nil {
		t.Fatal("expected an error for a project file that does not exist")
	}
	// Either mx is unavailable (named as such) or mx ran and complained; both
	// are errors that say what happened, which is the contract under test.
	msg := err.Error()
	if !strings.Contains(msg, "mx not found") && !strings.Contains(msg, "sync-java-dependencies failed") {
		t.Errorf("error should name the failure, got: %v", err)
	}
}
