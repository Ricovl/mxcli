// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// fakeMacJDKEnv models a Mac with a Java 25 project in view: /usr/bin/java is
// the launcher stub (not a symlink), which forwards `-version` to whatever JDK
// is default — so isJDK("/usr", n) answers yes for any installed major. That
// is the machine on which `mxcli new` handed MxBuild --java-home=/usr and hung.
func fakeMacJDKEnv(javaHomes map[int]string, asked *[]int) jdkEnv {
	installed := map[string]int{}
	for m, h := range javaHomes {
		installed[h] = m
	}
	return jdkEnv{
		goos:   "darwin",
		getenv: func(string) string { return "" },
		macJavaHome: func(major int) (string, error) {
			if asked != nil {
				*asked = append(*asked, major)
			}
			if h, ok := javaHomes[major]; ok {
				return h, nil
			}
			return "", errors.New("Unable to find any JVMs matching version \"" + strconv.Itoa(major) + "\".")
		},
		lookPath:     func(string) (string, error) { return "/usr/bin/java", nil },
		evalSymlinks: func(p string) (string, error) { return p, nil }, // the stub is a real file
		isJDK: func(home string, major int) bool {
			if home == "/usr" {
				// The stub forwards to the default JDK; pretend every major
				// is "default", the worst case.
				return true
			}
			return installed[home] == major
		},
		glob:        func(string) ([]string, error) { return nil, nil },
		searchPaths: func(major int) []string { return jdkSearchPathsFor("darwin", major) },
		versionOf:   func(string) string { return "(unknown)" },
	}
}

func TestResolveJDK_Darwin_AsksJavaHomeForProjectMajor(t *testing.T) {
	const jdk25 = "/Library/Java/JavaVirtualMachines/temurin-25.jdk/Contents/Home"
	const jdk21 = "/Library/Java/JavaVirtualMachines/temurin-21.jdk/Contents/Home"
	var asked []int
	env := fakeMacJDKEnv(map[int]string{21: jdk21, 25: jdk25}, &asked)

	got, err := resolveJDKWith(env, 25)
	if err != nil {
		t.Fatalf("resolveJDK(25): %v", err)
	}
	if got != jdk25 {
		t.Fatalf("resolveJDK(25) = %q, want java_home -v 25's answer %q", got, jdk25)
	}
	if len(asked) != 1 || asked[0] != 25 {
		t.Fatalf("java_home was asked for %v, want [25]", asked)
	}
}

func TestResolveJDK_Darwin_NeverReturnsUsrStub(t *testing.T) {
	// JDK 21 installed, project wants 25: java_home -v 25 fails, and the old
	// code fell through to /usr/bin/java -> home /usr, which the stub "passed".
	env := fakeMacJDKEnv(map[int]string{21: "/Library/Java/JavaVirtualMachines/temurin-21.jdk/Contents/Home"}, nil)

	got, err := resolveJDKWith(env, 25)
	if got == "/usr" {
		t.Fatalf("resolveJDK(25) returned the /usr/bin/java stub's home /usr")
	}
	if err == nil {
		t.Fatalf("resolveJDK(25) = %q, want an error (no JDK 25 installed)", got)
	}
	msg := err.Error()
	for _, want := range []string{"JDK 25 not found", "java_home -v 25", "temurin@25"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q:\n%s", want, msg)
		}
	}
}

func TestResolveJDK_Darwin_RejectsJavaHomeEnvAtUsr(t *testing.T) {
	env := fakeMacJDKEnv(nil, nil)
	env.getenv = func(k string) string {
		if k == "JAVA_HOME" {
			return "/usr"
		}
		return ""
	}
	if got, err := resolveJDKWith(env, 21); err == nil || got == "/usr" {
		t.Fatalf("resolveJDK(21) with JAVA_HOME=/usr = (%q, %v), want an error", got, err)
	}
}

// Control: on Linux a PATH java whose home is a real JDK still resolves, and
// /usr is not special there (a distro JDK can legitimately live under it via
// symlinks resolved to /usr/lib/jvm/...).
func TestResolveJDK_Linux_PathFallbackStillWorks(t *testing.T) {
	env := fakeMacJDKEnv(nil, nil)
	env.goos = "linux"
	env.macJavaHome = func(int) (string, error) { t.Fatal("java_home must not run off macOS"); return "", nil }
	env.evalSymlinks = func(string) (string, error) { return "/usr/lib/jvm/temurin-25-jdk/bin/java", nil }
	env.isJDK = func(home string, major int) bool { return home == "/usr/lib/jvm/temurin-25-jdk" && major == 25 }
	got, err := resolveJDKWith(env, 25)
	if err != nil || got != "/usr/lib/jvm/temurin-25-jdk" {
		t.Fatalf("resolveJDK(25) on linux = (%q, %v)", got, err)
	}
}
