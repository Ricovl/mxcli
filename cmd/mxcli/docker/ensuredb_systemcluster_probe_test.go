// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The probe behind the ako/mxcli#866 refusal: which data directories count as
// an existing cluster, on which port, and how to start each.

// The layouts other distributions use. Port matching is the guard's scope: a
// configuration it can read decides; one it cannot read (a 0700 data
// directory, as a non-root user) is PostgreSQL's default, 5432.
func TestFindSystemClusters_Layouts(t *testing.T) {
	cases := []struct {
		name, dir string
		conf      string // postgresql.conf in the data directory, if any
		wantPort  string
		hint      string
	}{
		{name: "RHEL/Fedora", dir: "var/lib/pgsql/data", wantPort: "5432", hint: "systemctl start postgresql"},
		{name: "PGDG", dir: "var/lib/pgsql/16/data", conf: "port = 5433\n", wantPort: "5433", hint: "systemctl start postgresql"},
		{name: "Arch", dir: "var/lib/postgres/data", conf: "#port = 5432\n", wantPort: "5432", hint: "systemctl start postgresql"},
		{name: "Homebrew", dir: "opt/homebrew/var/postgresql@16", conf: "port = 5440 # mine\n", wantPort: "5440", hint: "brew services start postgresql@16"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			withSystemClusterRoot(t, root)
			dataDir := filepath.Join(root, filepath.FromSlash(tc.dir))
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("16\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.conf != "" {
				if err := os.WriteFile(filepath.Join(dataDir, "postgresql.conf"), []byte(tc.conf), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			c, ok := conflictingSystemCluster(tc.wantPort)
			if !ok {
				t.Fatalf("no cluster found at %s for port %s (found %+v)", dataDir, tc.wantPort, findSystemClusters())
			}
			if c.dataDir != dataDir {
				t.Errorf("data dir = %q, want %q", c.dataDir, dataDir)
			}
			if !strings.Contains(c.startCommand(), tc.hint) {
				t.Errorf("start hint = %q, want it to mention %q", c.startCommand(), tc.hint)
			}
			if _, ok := conflictingSystemCluster("6543"); ok {
				t.Errorf("a cluster on %s must not match another port", tc.wantPort)
			}
		})
	}
}

// A directory without PG_VERSION is not a cluster (an empty package directory,
// a cluster that was dropped) — refusing there would block the fallback for
// nothing.
func TestFindSystemClusters_IgnoresUninitializedDirectories(t *testing.T) {
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	for _, d := range []string{"var/lib/postgresql/16/main", "var/lib/pgsql/data", "etc/postgresql/16/main"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := findSystemClusters(); len(got) != 0 {
		t.Fatalf("no initialized data directory exists, found %+v", got)
	}
}

// A non-root user cannot look inside /var/lib/postgresql/16/main (0700,
// owned by postgres), so stat(PG_VERSION) is refused rather than missing. That
// refusal is evidence of a cluster, not of its absence — the devcontainer user
// is exactly who #866 happened to. Injected, because this suite may run as root.
func TestFindSystemClusters_UnreadableDataDirectoryCounts(t *testing.T) {
	root := t.TempDir()
	withSystemClusterRoot(t, root)
	dataDir := debianCluster(t, root, "16", "main", "5432")
	old := clusterProbeStat
	clusterProbeStat = func(name string) (fs.FileInfo, error) {
		if strings.HasPrefix(name, dataDir+string(filepath.Separator)) {
			return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrPermission}
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { clusterProbeStat = old })

	c, ok := conflictingSystemCluster("5432")
	if !ok || c.dataDir != dataDir {
		t.Fatalf("a permission-denied data directory must count as a cluster; got %+v, %v", c, ok)
	}
	// And a missing one still does not.
	clusterProbeStat = func(name string) (fs.FileInfo, error) {
		if strings.HasSuffix(name, "PG_VERSION") {
			return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
		}
		return os.Stat(name)
	}
	if _, ok := conflictingSystemCluster("5432"); ok {
		t.Fatal("a data directory without PG_VERSION is not a cluster")
	}
}
