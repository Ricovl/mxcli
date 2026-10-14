// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeMxcliFailingRun is a stand-in mxcli whose `run` fails the way a session
// without a Postgres superuser does; every other command succeeds.
func fakeMxcliFailingRun(t *testing.T, path string) {
	t.Helper()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo \"mxcli version v0.25.0 (2026-10-01T00:00:00Z)\"; exit 0; fi\n" +
		"if [ \"$1\" = \"run\" ]; then echo \"Caching MxBuild...\"; echo \"Error: no local PostgreSQL superuser available to create the app database\" >&2; exit 3; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The hook runs the script under `|| true` so a failure never blocks the
// session — which made a failed database provisioning invisible. The script
// must say so loudly itself, and still exit with the failure's status.
func TestBootstrapScript_ProvisioningFailureIsLoud(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "")
	fakeMxcliFailingRun(t, filepath.Join(e.project, "mxcli"))

	cmd := exec.Command("sh", bootstrapScriptName)
	cmd.Dir = e.project
	cmd.Env = append(os.Environ(), "PATH="+e.bin+":/usr/bin:/bin", "BOOT_LOG="+e.log, "MXCLI_TAG=")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("script exit = %v, want status 3 from the failed run\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	got := stderr.String()
	for _, want := range []string{
		"ERROR: the app database was not provisioned",
		"no local PostgreSQL superuser available",
		"./mxcli run --local --setup --ensure-db -p App.mpr",
		"--db-type hsqldb",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr lacks %q:\n%s", want, got)
		}
	}
	// The command's own output still reaches the session.
	if !strings.Contains(stdout.String()+got, "Caching MxBuild") {
		t.Errorf("run output was swallowed:\nstdout:\n%s\nstderr:\n%s", stdout.String(), got)
	}
}

// Control: a successful run prints no banner and exits 0.
func TestBootstrapScript_ProvisioningSuccessIsQuiet(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "")
	fakeMxcli(t, filepath.Join(e.project, "mxcli"), "v0.25.0")
	out := e.run(t)
	if strings.Contains(out, "ERROR") {
		t.Errorf("banner on success:\n%s", out)
	}
	log, _ := os.ReadFile(e.log)
	if !strings.Contains(string(log), "run --local --setup --ensure-db -p App.mpr") {
		t.Errorf("run was not invoked:\n%s", log)
	}
}
