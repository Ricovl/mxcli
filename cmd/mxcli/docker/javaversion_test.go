// SPDX-License-Identifier: Apache-2.0

// Version-aware JDK selection. Mendix Studio Pro 11.14 supports Java 25 and its
// blank app asks for it, so a project's Settings > Model > JavaVersion is now a
// real variable rather than always 21. mxcli resolved a JDK 21 unconditionally
// and refused any other JAVA_HOME, which made an 11.14 project impossible to
// build (`error: release version 25 not supported`) or to boot when built
// elsewhere (`UnsupportedClassVersionError … class file version 69.0`).
package docker

import (
	"errors"
	"strings"
	"testing"
)

func TestParseJavaMajor(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		// Both spellings Mendix has used for the setting.
		{"21", 21, true},
		{"25", 25, true},
		{"Java21", 21, true},
		{"Java25", 25, true},
		{" 25 ", 25, true},

		// Not understood: the caller must fall back rather than guess. Guessing a
		// JDK from an unreadable model trades a clear failure for a confusing one.
		{"", 0, false},
		{"Java", 0, false},
		{"tip", 0, false},
		{"25-ea", 0, false},
		{"-3", 0, false},
	}
	for _, tc := range tests {
		got, ok := ParseJavaMajor(tc.in)
		if ok != tc.ok {
			t.Errorf("ParseJavaMajor(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseJavaMajor(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestJavaMajorRegexDoesNotMatchALongerNumber(t *testing.T) {
	// The trap in matching `java -version` text: a bare "21" also occurs inside
	// "210". The major must be followed by a dot, whitespace or the closing
	// quote. Without this a future JDK 210 would satisfy a request for 21 — and,
	// more immediately, "2" would satisfy "25" if the pattern were ever loosened.
	re := javaMajorRegex(21)
	for _, out := range []string{
		`openjdk version "21.0.4" 2024-07-16`,
		`openjdk version "21" 2023-09-19`,
	} {
		if !re.MatchString(out) {
			t.Errorf("JDK 21 pattern should match %q", out)
		}
	}
	for _, out := range []string{
		`openjdk version "210.0.1" 2099-01-01`,
		`openjdk version "25" 2025-09-16`,
		`openjdk version "17.0.9" 2023-10-17`,
	} {
		if re.MatchString(out) {
			t.Errorf("JDK 21 pattern must not match %q", out)
		}
	}
	// And the new case has to work in its own right.
	if !javaMajorRegex(25).MatchString(`openjdk version "25" 2025-09-16`) {
		t.Error("JDK 25 pattern should match a 25 version string")
	}
}

func TestJdkSearchPathsTrackTheRequestedMajor(t *testing.T) {
	// The search paths are globs with the version baked in, so they have to move
	// with the request or a JDK 25 is never found even when installed.
	// windowsProgramDirs reads the environment, so the Windows list is empty
	// without it — the same setup the existing search-path test does.
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("LOCALAPPDATA", `C:\Users\dev\AppData\Local`)
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, major := range []int{21, 25} {
			paths := jdkSearchPathsFor(goos, major)
			if len(paths) == 0 {
				t.Fatalf("%s/%d: no search paths", goos, major)
			}
			want := map[int]string{21: "21", 25: "25"}[major]
			other := map[int]string{21: "25", 25: "21"}[major]
			for _, p := range paths {
				if !strings.Contains(p, want) {
					t.Errorf("%s/%d: %q does not pin the requested major", goos, major, p)
				}
				if strings.Contains(p, other) {
					t.Errorf("%s/%d: %q still mentions %s", goos, major, p, other)
				}
			}
		}
	}
}

func TestJavaMajorOrDefault(t *testing.T) {
	// Zero means "the caller did not say", which must behave exactly as mxcli did
	// before this change — otherwise every construction site that forgets the
	// field silently changes behaviour.
	if got := javaMajorOrDefault(0); got != DefaultJavaMajor {
		t.Errorf("javaMajorOrDefault(0) = %d, want %d", got, DefaultJavaMajor)
	}
	if got := javaMajorOrDefault(-1); got != DefaultJavaMajor {
		t.Errorf("javaMajorOrDefault(-1) = %d, want %d", got, DefaultJavaMajor)
	}
	if got := javaMajorOrDefault(25); got != 25 {
		t.Errorf("javaMajorOrDefault(25) = %d, want 25", got)
	}
}

// mendixlabs/mxcli#1437: a Mendix 9 model stores no JavaVersion at all, and the
// fallback was a flat 21 — so `run --local` skipped a JDK 17 JAVA_HOME and
// mxbuild 9.24's Gradle failed with "Unsupported class file major version 65".
// The fallback must come from the project's Mendix version.
func TestProjectJavaMajorDefaultsToJava17ForMendix9(t *testing.T) {
	p := v1Fixture(t) // Mendix 9.24.30, no JavaVersion setting
	got, fromProject := ProjectJavaMajor(p)
	if got != 17 || fromProject {
		t.Errorf("ProjectJavaMajor(9.24 project) = (%d, %v), want (17, false)", got, fromProject)
	}
}

func TestDefaultJavaMajorForMendixVersion(t *testing.T) {
	tests := []struct {
		major int
		want  int
	}{
		{9, 17},
		{10, DefaultJavaMajor},
		{11, DefaultJavaMajor},
		{0, DefaultJavaMajor}, // unknown version: behave as before
	}
	for _, tc := range tests {
		if got := defaultJavaMajorForMendix(tc.major); got != tc.want {
			t.Errorf("defaultJavaMajorForMendix(%d) = %d, want %d", tc.major, got, tc.want)
		}
	}
}

// The "build for a release you already have" hint suggests lowering JavaVersion
// to 21. For a Mendix 9 project (needs 17) that advice is a no-op: 9.x has no
// JavaVersion setting, and 21 is not a JDK it can build with.
func TestResolveJDKDoesNotSuggestAlterSettingsBelowDefault(t *testing.T) {
	env := jdkEnv{
		goos:         "linux",
		getenv:       func(string) string { return "" },
		lookPath:     func(string) (string, error) { return "", errors.New("not found") },
		evalSymlinks: func(p string) (string, error) { return p, nil },
		isJDK:        func(string, int) bool { return false },
		glob:         func(string) ([]string, error) { return nil, nil },
		searchPaths:  func(int) []string { return nil },
		versionOf:    func(string) string { return "" },
	}
	_, err := resolveJDKWith(env, 17)
	if err == nil {
		t.Fatal("expected JDK 17 not found")
	}
	if strings.Contains(err.Error(), "alter settings") {
		t.Errorf("JDK 17 error suggests alter settings, which no-ops on Mendix 9.x:\n%v", err)
	}
	// A project asking above the default can still be steered down to it.
	_, err = resolveJDKWith(env, 25)
	if err == nil || !strings.Contains(err.Error(), "alter settings") {
		t.Errorf("JDK 25 error should keep the alter settings hint, got: %v", err)
	}
}

func TestProjectJavaMajorFallsBackOnAnUnreadableProject(t *testing.T) {
	got, fromProject := ProjectJavaMajor("/nonexistent/project.mpr")
	if got != DefaultJavaMajor || fromProject {
		t.Errorf("ProjectJavaMajor(missing) = (%d, %v), want (%d, false)", got, fromProject, DefaultJavaMajor)
	}
}
