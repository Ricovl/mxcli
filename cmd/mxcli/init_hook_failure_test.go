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

// ako/mxcli#866: the refusal to provision an empty cluster over a stopped
// system one is only useful if the session sees WHICH cluster to start. The
// banner shows one line; it must be mxcli's own "Error:" line, not the last
// line that happens to contain "error" — a failing service manager quotes
// pg_ctlcluster, whose output carries "Error:" lines of its own, after it.
func TestBootstrapScript_BannerNamesTheRefusalNotTheServiceOutput(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "")
	stderrText := `Error: ensuring database: starting local PostgreSQL: refusing to create a new, empty PostgreSQL cluster: the cluster in /var/lib/postgresql/16/main (port 5432) exists but is not running
  Start it, then rerun:
    sudo pg_ctlcluster 16 main start   (or: sudo service postgresql start)
(a service manager ran first but Postgres did not become ready:
sudo service: Starting PostgreSQL 16 database server: main
Error: /usr/lib/postgresql/16/bin/pg_ctl /usr/lib/postgresql/16/bin/pg_ctl start -D /var/lib/postgresql/16/main exited with status 1:
service: mkdir: cannot create directory '/var/run/postgresql': Permission denied)`
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo \"mxcli version v0.25.0 (2026-10-01T00:00:00Z)\"; exit 0; fi\n" +
		"if [ \"$1\" = \"run\" ]; then echo \"Ensuring database...\"; cat >&2 <<'MSG'\n" + stderrText + "\nMSG\nexit 1; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(e.project, "mxcli"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", bootstrapScriptName)
	cmd.Dir = e.project
	cmd.Env = append(os.Environ(), "PATH="+e.bin+":/usr/bin:/bin", "BOOT_LOG="+e.log, "MXCLI_TAG=")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()

	var banner string
	for _, l := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(l, "ERROR: the app database was not provisioned") {
			banner = l
		}
	}
	if banner == "" {
		t.Fatalf("no banner:\n%s", stderr.String())
	}
	if !strings.Contains(banner, "refusing to create a new, empty PostgreSQL cluster") ||
		!strings.Contains(banner, "/var/lib/postgresql/16/main") {
		t.Errorf("the banner must name the refusal and the data directory, got:\n%s", banner)
	}
}
