// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The bootstrap script decides which mxcli serves the project before any
// mxcli runs, so it carries its own copy of the version ordering. These tests
// run the generated script under sh with stand-in binaries (ako/mxcli#952).

func requireSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the bootstrap script is POSIX sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
}

// bootstrapFunctions is the script's helper-function prelude: everything
// before the first statement that acts.
func bootstrapFunctions(t *testing.T) string {
	t.Helper()
	script := fmt.Sprintf(bootstrapScriptTemplate, "App.mpr")
	i := strings.Index(script, "\nstale=\n")
	if i < 0 {
		t.Fatal("bootstrap script has no 'stale=' line to cut the prelude at")
	}
	return strings.Replace(script[:i], "set -e\n", "", 1)
}

// TestBootstrapVersionGuard_AgreesWithGo runs the script's version_lt over the
// same table the Go comparison is tested with.
func TestBootstrapVersionGuard_AgreesWithGo(t *testing.T) {
	requireSh(t)
	prelude := bootstrapFunctions(t)
	for _, c := range versionOrderCases {
		t.Run(c.name, func(t *testing.T) {
			sh := prelude + fmt.Sprintf("\nif version_lt %q \"$(ymd %q)\" %q \"$(ymd %q)\"; then echo older; else echo not; fi\n",
				c.have, c.haveBuilt, c.want, c.wantBuilt)
			out, err := exec.Command("sh", "-c", sh).CombinedOutput()
			if err != nil {
				t.Fatalf("sh: %v\n%s", err, out)
			}
			got := strings.TrimSpace(string(out)) == "older"
			if got != c.older {
				t.Errorf("sh version_lt(%s, %s) = %v (output %q), want %v", c.have, c.want, got, out, c.older)
			}
		})
	}
}

// fakeMxcli writes a stand-in mxcli reporting version, logging every other
// invocation so a test can tell which binary ran the rest of the script.
func fakeMxcli(t *testing.T, path, version string) {
	t.Helper()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo \"mxcli version " + version + " (2026-10-01T00:00:00Z)\"; exit 0; fi\n" +
		"echo \"" + version + " $*\" >> \"$BOOT_LOG\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

type bootEnv struct {
	project, bin, log string
}

// newBootEnv sets up a project with the bootstrap script, an optional stamp,
// and a bin dir holding a fake curl that "downloads" a nightly from the future.
func newBootEnv(t *testing.T, stampVersion string) bootEnv {
	t.Helper()
	root := t.TempDir()
	e := bootEnv{project: filepath.Join(root, "app"), bin: filepath.Join(root, "bin"), log: filepath.Join(root, "boot.log")}
	for _, d := range []string{e.project, e.bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writeBootstrapScript(filepath.Join(e.project, ".claude"), "App.mpr"); err != nil {
		t.Fatal(err)
	}
	if stampVersion != "" {
		stamp := fmt.Sprintf("{\n  \"version\": %q,\n  \"written\": \"2026-10-01\"\n}\n", stampVersion)
		if err := os.MkdirAll(filepath.Join(e.project, ".ai-context"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.project, toolingStampRel), []byte(stamp), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// curl -fsSL -o FILE URL: write a nightly stand-in to FILE.
	curl := "#!/bin/sh\nout=\nwhile [ $# -gt 0 ]; do if [ \"$1\" = \"-o\" ]; then out=$2; shift; fi; shift; done\n" +
		"echo \"curl $out\" >> \"$BOOT_LOG\"\n" +
		"printf '#!/bin/sh\\nif [ \"$1\" = \"--version\" ]; then echo \"mxcli version nightly-20991231-abcdef0\"; exit 0; fi\\necho \"nightly $*\" >> \"$BOOT_LOG\"\\n' > \"$out\"\n"
	if err := os.WriteFile(filepath.Join(e.bin, "curl"), []byte(curl), 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e bootEnv) run(t *testing.T) (stderr string) {
	t.Helper()
	cmd := exec.Command("sh", bootstrapScriptName)
	cmd.Dir = e.project
	cmd.Env = append(os.Environ(), "PATH="+e.bin+":/usr/bin:/bin", "BOOT_LOG="+e.log, "MXCLI_TAG=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}
	return string(out)
}

func (e bootEnv) projectBinaryVersion(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(filepath.Join(e.project, "mxcli"), "--version").Output()
	if err != nil {
		t.Fatalf("./mxcli --version: %v", err)
	}
	return strings.Fields(string(out))[2]
}

func (e bootEnv) downloaded(t *testing.T) bool {
	data, _ := os.ReadFile(e.log)
	return strings.Contains(string(data), "curl ")
}

// The reported skew: v0.24.0 on PATH, tooling from a newer mxcli. The old
// bootstrap linked it in; the guard refuses and downloads instead.
func TestBootstrapVersionGuard_OlderPathBinaryIsNotLinked(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "v0.25.0")
	fakeMxcli(t, filepath.Join(e.bin, "mxcli"), "v0.24.0")

	out := e.run(t)
	if !e.downloaded(t) {
		t.Errorf("older PATH binary was linked instead of downloading:\n%s", out)
	}
	if got := e.projectBinaryVersion(t); got != "nightly-20991231-abcdef0" {
		t.Errorf("./mxcli is %s, want the downloaded nightly", got)
	}
	if !strings.Contains(out, "not linking it") {
		t.Errorf("no explanation for skipping the PATH binary:\n%s", out)
	}
}

// Control: a PATH binary at least as new as the stamp is linked, no download.
func TestBootstrapVersionGuard_NewerPathBinaryIsLinked(t *testing.T) {
	requireSh(t)
	for _, c := range []struct{ name, stamp, onPath string }{
		{"newer than stamp", "v0.24.0", "v0.25.0"},
		{"no stamp (pre-#952 project)", "", "v0.24.0"},
		{"dev build is never second-guessed", "v0.25.0", "v0.24.0-888-g4ba1495f2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newBootEnv(t, c.stamp)
			fakeMxcli(t, filepath.Join(e.bin, "mxcli"), c.onPath)
			out := e.run(t)
			if e.downloaded(t) {
				t.Errorf("downloaded although the PATH binary qualifies:\n%s", out)
			}
			if got := e.projectBinaryVersion(t); got != c.onPath {
				t.Errorf("./mxcli is %s, want the PATH binary %s", got, c.onPath)
			}
		})
	}
}

// A project binary older than the stamp is replaced — by a newer PATH binary
// when there is one.
func TestBootstrapVersionGuard_StaleProjectBinaryIsReplaced(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "v0.25.0")
	fakeMxcli(t, filepath.Join(e.project, "mxcli"), "v0.24.0")
	fakeMxcli(t, filepath.Join(e.bin, "mxcli"), "v0.26.0")

	out := e.run(t)
	if got := e.projectBinaryVersion(t); got != "v0.26.0" {
		t.Errorf("./mxcli is %s, want v0.26.0 from PATH:\n%s", got, out)
	}
	if e.downloaded(t) {
		t.Error("downloaded although PATH had a newer binary")
	}
	// The rest of the script ran on the replacement, not the stale binary.
	log, _ := os.ReadFile(e.log)
	if strings.Contains(string(log), "v0.24.0 ") {
		t.Errorf("stale binary still ran:\n%s", log)
	}
}

// ./mxcli symlinked to an old PATH binary: the replacement must not be
// written through the link into the PATH binary itself.
func TestBootstrapVersionGuard_DownloadDoesNotWriteThroughSymlink(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "v0.25.0")
	onPath := filepath.Join(e.bin, "mxcli")
	fakeMxcli(t, onPath, "v0.24.0")
	if err := os.Symlink(onPath, filepath.Join(e.project, "mxcli")); err != nil {
		t.Fatal(err)
	}

	e.run(t)
	if got := e.projectBinaryVersion(t); got != "nightly-20991231-abcdef0" {
		t.Errorf("./mxcli is %s, want the downloaded nightly", got)
	}
	out, err := exec.Command(onPath, "--version").Output()
	if err != nil || !strings.Contains(string(out), "v0.24.0") {
		t.Errorf("PATH binary was modified: %q %v", out, err)
	}
}
