// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#866, verbatim: "In a container restarted with the system cluster
// down, the `mxcli init` SessionStart hook runs `--ensure-db`, provisions a
// fresh **empty** cluster, and both apps boot healthy against it. Every
// captured row (7,966 laps here) is absent, and no error says so. An empty
// database is indistinguishable from a new one."
//
// The #823 user-owned fallback is right when there is no PostgreSQL to start,
// and wrong when there is one that did not start: a new cluster on the same
// port answers every probe the app makes, so the only place the difference can
// be stated is here, before initdb runs.

// withSystemClusterRoot points the system-cluster probe at root.
func withSystemClusterRoot(t *testing.T, root string) {
	t.Helper()
	old := systemClusterRoot
	systemClusterRoot = root
	t.Cleanup(func() { systemClusterRoot = old })
}

// debianCluster lays out what postgresql-common leaves for cluster ver/name
// under root: its configuration in /etc/postgresql/<ver>/<name>/ (with the port
// and the data directory, as Debian writes them) and an initialized data
// directory in /var/lib/postgresql/<ver>/<name>. It returns the data directory.
func debianCluster(t *testing.T, root, ver, name, port string) string {
	t.Helper()
	dataDir := filepath.Join(root, "var", "lib", "postgresql", ver, name)
	confDir := filepath.Join(root, "etc", "postgresql", ver, name)
	for _, d := range []string{dataDir, confDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte(ver+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf := "# Debian postgresql-common\n" +
		"data_directory = '/var/lib/postgresql/" + ver + "/" + name + "'\t\t# use data in another directory\n" +
		"port = " + port + "\t\t\t\t# (change requires restart)\n"
	if err := os.WriteFile(filepath.Join(confDir, "postgresql.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

// stubSystemClusterThatWillNotStart is the reported machine: a service manager
// that runs and starts nothing, a server that never becomes ready, and the
// portable tools a user-owned fallback would use.
func stubSystemClusterThatWillNotStart(t *testing.T, dir, logPath string) {
	t.Helper()
	writeStub(t, dir, "service", `echo "service $*" >> "`+logPath+`"; echo "Starting PostgreSQL 16 database server: main failed!"; exit 1`)
	writeStub(t, dir, "pg_isready", "exit 1")
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))
}

// The reported case: the system cluster exists, its port is the app's, and it
// is down. Falling back to `initdb` here is what produced the healthy-looking
// empty database. The run must stop and name the data directory instead.
func TestStartLocalPostgres_RefusesNewClusterWhileSystemClusterIsDown(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	dataDir := debianCluster(t, root, "16", "main", port)
	stubSystemClusterThatWillNotStart(t, dir, logPath)

	var out bytes.Buffer
	_, err := startLocalPostgres("127.0.0.1", port, &out)
	calls := readCalls(t, logPath)
	if strings.Contains(calls, "initdb") || strings.Contains(calls, "pg_ctl_start") {
		t.Fatalf("a NEW, EMPTY user-owned cluster was provisioned on port %s while the system "+
			"cluster at %s is down — the app now boots healthy against an empty database and "+
			"no error says so (ako/mxcli#866)\ncalls:\n%s\nerr: %v\noutput:\n%s",
			port, dataDir, calls, err, out.String())
	}
	if !strings.Contains(calls, "service postgresql start") {
		t.Errorf("the system cluster must still be tried first, calls:\n%s", calls)
	}
	if err == nil {
		t.Fatal("expected a refusal naming the stopped system cluster")
	}
	msg := err.Error()
	first := strings.SplitN(msg, "\n", 2)[0]
	for _, want := range []string{"refusing to create a new, empty PostgreSQL cluster", dataDir, "not running"} {
		if !strings.Contains(first, want) {
			t.Errorf("the refusal's FIRST line (the one a hook banner shows) lacks %q:\n%s", want, msg)
		}
	}
	for _, want := range []string{
		"sudo pg_ctlcluster 16 main start", // how to start exactly that cluster
		"sudo service postgresql start",
		"MXCLI_ENSURE_DB_NEW_CLUSTER=1", // the deliberate way past it
		"--db-type hsqldb",
		"main failed!", // what the service manager said
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal lacks %q:\n%s", want, msg)
		}
	}
	// Only the first line may read as an error: the bootstrap hook names the
	// failure by the first "Error:" line, and guidance must not compete.
	rest := strings.SplitN(msg, "(a service manager ran first", 2)[0]
	if strings.Contains(strings.ToLower(strings.SplitN(rest, "\n", 2)[1]), "error") {
		t.Errorf("guidance lines must not contain 'error':\n%s", rest)
	}
}

// CONTROL: a system cluster configured for a different port is not this app's
// database — the fallback is the #823 behaviour and still runs, loudly.
func TestStartLocalPostgres_SystemClusterOnAnotherPortDoesNotBlock(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	other := "1"
	if port == other {
		other = "2"
	}
	debianCluster(t, root, "16", "main", other)
	stubSystemClusterThatWillNotStart(t, dir, logPath)

	var out bytes.Buffer
	if _, err := startLocalPostgres("127.0.0.1", port, &out); err != nil {
		t.Fatalf("a cluster on port %s must not block port %s: %v", other, port, err)
	}
	if calls := readCalls(t, logPath); !strings.Contains(calls, "initdb") {
		t.Fatalf("expected the user-owned fallback to run:\n%s", calls)
	}
	if !strings.Contains(out.String(), "NEW, EMPTY") {
		t.Errorf("a freshly initialized cluster must be announced loudly:\n%s", out.String())
	}
}

// CONTROL: a user-owned cluster that already exists holds whatever the earlier
// runs put there — restarting it is not "provisioning an empty one", and must
// keep working. The stopped system cluster is still worth a line, for the
// users the old behaviour already moved onto an empty cluster.
func TestStartLocalPostgres_ExistingUserClusterIsRestartedNotRefused(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	sysData := debianCluster(t, root, "16", "main", port)
	stubSystemClusterThatWillNotStart(t, dir, logPath)
	initClusterDir(t)

	var out bytes.Buffer
	if _, err := startLocalPostgres("127.0.0.1", port, &out); err != nil {
		t.Fatalf("restarting an existing user-owned cluster must not be refused: %v", err)
	}
	calls := readCalls(t, logPath)
	if strings.Contains(calls, "initdb") || !strings.Contains(calls, "pg_ctl_start") {
		t.Fatalf("expected a plain restart of the user-owned cluster:\n%s", calls)
	}
	if strings.Contains(out.String(), "NEW, EMPTY") {
		t.Errorf("a restarted cluster is not new and must not be announced as such:\n%s", out.String())
	}
	if !strings.Contains(out.String(), sysData) {
		t.Errorf("the stopped system cluster should be named next to the cluster in use:\n%s", out.String())
	}
}

// The deliberate way past the refusal, for a system cluster that is not this
// app's and cannot be started (no sudo): a new cluster, announced as such.
func TestStartLocalPostgres_NewClusterOptIn(t *testing.T) {
	dir, logPath := newStubPATH(t)
	withTimeouts(t, 20*time.Millisecond)
	port := unusedTCPPort(t)
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	debianCluster(t, root, "16", "main", port)
	stubSystemClusterThatWillNotStart(t, dir, logPath)
	t.Setenv("MXCLI_ENSURE_DB_NEW_CLUSTER", "1")

	var out bytes.Buffer
	if _, err := startLocalPostgres("127.0.0.1", port, &out); err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, logPath); !strings.Contains(calls, "initdb") {
		t.Fatalf("the opt-in must allow the new cluster:\n%s", calls)
	}
	if !strings.Contains(out.String(), "NEW, EMPTY") {
		t.Errorf("the new cluster must still be announced:\n%s", out.String())
	}
}

// The warning belongs to the moment initdb runs, and only to it.
func TestStartUserCluster_AnnouncesOnlyANewCluster(t *testing.T) {
	dir, logPath := newStubPATH(t)
	writeStub(t, dir, "initdb", initdbStub(logPath))
	writeStub(t, dir, "pg_ctl", pgctlStub(logPath, 3, 0))

	var first bytes.Buffer
	if err := startUserCluster("127.0.0.1", "5544", &first); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".mxcli", "postgres", "data")
	if !strings.Contains(first.String(), "WARNING") || !strings.Contains(first.String(), "NEW, EMPTY") ||
		!strings.Contains(first.String(), dataDir) {
		t.Errorf("initializing a cluster must say loudly that the database is new and empty, and where:\n%s", first.String())
	}

	var second bytes.Buffer
	if err := startUserCluster("127.0.0.1", "5544", &second); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second.String(), "NEW, EMPTY") {
		t.Errorf("restarting the same cluster must not claim it is new:\n%s", second.String())
	}
}
