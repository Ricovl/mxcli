// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	mxversion "github.com/mendixlabs/mxcli/mdl/types"
)

// TestBuild_PreservesMPRv2StorageFormat is the end-to-end guard for
// mendixlabs/mxcli#808, the counterpart to TestCheck_PreservesMPRv2StorageFormat
// (#763). Build's pre-check step ran its own bare `mx update-widgets`, which
// rewrites an MPRv2 project into the self-contained MPRv1 format — inlining every
// unit into the .mpr and deleting mprcontents/. #764 protected Check only, so
// `mxcli docker build`, `docker run` and `docker reload` kept converting projects
// while reporting success.
//
// DryRun stops Build immediately after the check step, so this exercises the whole
// buggy path (update-widgets + mx check) without paying for a full MxBuild. The
// deferred restore still runs on the DryRun return.
//
// Requires a resolvable mx/MxBuild and a JDK for the version the scaffolded
// project asks for (provided by the CI integration job); skips otherwise.
func TestBuild_PreservesMPRv2StorageFormat(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	// Scaffold a fresh project. `mx create-project` (no template arg) writes App.mpr
	// into the working directory and produces MPRv2 storage.
	dir := t.TempDir()
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed, cannot scaffold fixture: %v\n%s", err, out)
	}
	mprPath := filepath.Join(dir, "App.mpr")
	if _, err := os.Stat(mprPath); err != nil {
		t.Skipf("mx create-project did not produce App.mpr: %v", err)
	}

	// The JDK requirement comes from the PROJECT, so it can only be checked once
	// the project exists. Mendix 11.14's blank app asks for Java 25 while every
	// version up to 11.13 asks for 21 — a skip guard that resolved a fixed 21
	// would look for the wrong JDK on exactly the row that motivated making the
	// version per-project.
	major, _ := ProjectJavaMajor(mprPath)
	if _, err := resolveJDK(major); err != nil {
		t.Skipf("no JDK for Java %d, which this project asks for: %v", javaMajorOrDefault(major), err)
	}

	// Precondition: the fixture must be MPRv2, or the test proves nothing.
	if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Skipf("scaffolded project is %v, not MPRv2 — nothing to protect", v)
	}

	// Precondition: Build only supports Mendix >= 11.6.1 (portable app distribution);
	// below that it refuses before reaching the update-widgets step this test is about.
	// The nightly matrix includes 10.24, which is MPRv2 — so the format check above
	// passes and Build then fails its own version guard, which is a property of the
	// matrix row rather than a regression.
	//
	// This is a genuine capability gate, not a masked failure: there is no PAD build to
	// protect on 10.x. The Check counterpart has no version guard and does run there,
	// so MPRv2 preservation is still covered on every matrix row.
	if pv := mprProductVersion(t, mprPath); !pv.IsAtLeastFull(11, 6, 1) {
		t.Skipf("Build (portable app distribution) requires Mendix >= 11.6.1; scaffolded project is %s — "+
			"TestCheck_PreservesMPRv2StorageFormat covers this version", pv.ProductVersion)
	}

	var stdout bytes.Buffer
	if err := Build(BuildOptions{
		ProjectPath: mprPath,
		DryRun:      true,
		Stdout:      &stdout,
	}); err != nil {
		t.Fatalf("Build failed: %v\nstdout:\n%s", err, stdout.String())
	}

	// Postcondition: still MPRv2. Without the fix, update-widgets would have left it
	// MPRv1 with mprcontents/ deleted.
	if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("Build converted the project to %v; the MPRv2 storage format must be preserved (#808)", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "mprcontents")); err != nil {
		t.Errorf("mprcontents/ missing after Build, storage format was not preserved: %v", err)
	}
}

// mprProductVersion opens the .mpr and returns its Mendix product version.
func mprProductVersion(t *testing.T, mprPath string) *mxversion.ProjectVersion {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("openReadOnly(%s): %v", mprPath, err)
	}
	defer reader.Disconnect()
	return reader.ProjectVersion()
}

// TestBuild_LeavesProjectUntouched is the end-to-end guard for ako/mxcli#961:
// with real mx and MxBuild, a full `docker build` of an MPRv1 and an MPRv2
// project leaves every file outside the output directory (.docker/)
// byte-identical with the same mtime, and still produces the PAD package.
// Before the fix, update-widgets rewrote the v1 .mpr (and every v2 .mxunit was
// rewritten and restored with new mtimes), and mx check / MxBuild wrote
// theme-cache/, deployment/, javasource/ proxies, the .launch file, .classpath
// and .project into the project.
func TestBuild_LeavesProjectUntouched(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	for _, format := range []string{"v2", "v1"} {
		t.Run(format, func(t *testing.T) {
			// Not t.TempDir(): see TestCheck_LeavesProjectUntouched.
			dir, err := os.MkdirTemp("", "bld")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			cmd := exec.Command(mxPath, "create-project")
			cmd.Dir = dir
			PrepareMxCommand(cmd)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("mx create-project failed: %v\n%s", err, out)
			}
			mprPath := filepath.Join(dir, "App.mpr")
			if pv := mprProductVersion(t, mprPath); !pv.IsAtLeastFull(11, 6, 1) {
				t.Skipf("Build requires Mendix >= 11.6.1; scaffolded project is %s", pv.ProductVersion)
			}
			major, _ := ProjectJavaMajor(mprPath)
			if _, err := resolveJDK(major); err != nil {
				t.Skipf("no JDK for Java %d: %v", javaMajorOrDefault(major), err)
			}
			if format == "v1" {
				if err := updateWidgetsCmd(mxPath, mprPath, io.Discard, io.Discard); err != nil {
					t.Skipf("could not produce a v1 fixture: %v", err)
				}
				if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV1 {
					t.Skipf("fixture is %v, not v1", v)
				}
			}

			before := treeState(t, dir)
			var stdout bytes.Buffer
			if err := Build(BuildOptions{ProjectPath: mprPath, Stdout: &stdout}); err != nil {
				t.Fatalf("Build: %v\n%s", err, stdout.String())
			}
			var changed []string
			for _, d := range diffStates(before, treeState(t, dir)) {
				if !strings.Contains(d, ".docker") {
					changed = append(changed, d)
				}
			}
			if len(changed) > 0 {
				t.Errorf("docker build modified the project:\n  %s", strings.Join(changed, "\n  "))
			}
			if _, err := os.Stat(filepath.Join(dir, ".docker", "build", "Dockerfile")); err != nil {
				t.Errorf("no PAD in the output directory: %v\n%s", err, stdout.String())
			}
		})
	}
}
