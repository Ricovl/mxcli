// SPDX-License-Identifier: Apache-2.0

// `mxcli oql` and `mxcli log` find the admin API of a `mxcli run --local` through
// the dev-loop handshake (ako/mxcli#982). Before this, both assumed port 8090,
// so a loop started with --admin-port printed a query hint that failed with
// "cannot connect … localhost:8090".
package main

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

// fakeAdminAPI answers the 11.11+ OQL preview route, but only for one password.
func fakeAdminAPI(t *testing.T, pass string) (port int) {
	t.Helper()
	want := base64.StdEncoding.EncodeToString([]byte(pass))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-M2EE-Authentication") != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"n":"1"}]}`))
	}))
	t.Cleanup(srv.Close)
	_, p, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ = strconv.Atoi(p)
	return port
}

func publishFakeLoop(t *testing.T, project string, port int, pass string) {
	t.Helper()
	if err := writeDevLoopHandshake(project, devLoopHandshake{
		Project: project, PID: os.Getpid(), AppPort: 18080,
		AdminPort: port, AdminPass: pass, Started: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

// The reported symptom, end to end: the hint `mxcli oql -p <app>` must reach the
// admin port the loop recorded, with the password it recorded.
func TestOQL_UsesRunLocalAdminPort(t *testing.T) {
	t.Setenv("ADMIN_PORT", "")
	t.Setenv("M2EE_ADMIN_PASS", "")
	p := handshakeProject(t)
	port := fakeAdminAPI(t, "loop-secret")
	publishFakeLoop(t, p, port, "loop-secret")

	opts := devLoopAdminOptions(p, docker.M2EEOptions{ProjectPath: p}, adminFlagsSet{})
	res, err := docker.ExecuteOQL(docker.OQLOptions{
		Host: opts.Host, Port: opts.Port, Token: opts.Token,
		ProjectPath: opts.ProjectPath, Direct: opts.Direct,
	}, "SELECT 1")
	if err != nil {
		t.Fatalf("oql against the run --local admin port: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(res.Rows))
	}
}

// Control: without a handshake the defaults stand, so the test above is
// measuring the handshake and not something else that happens to work.
func TestDevLoopAdminOptions_NoHandshakeKeepsDefaults(t *testing.T) {
	p := handshakeProject(t)
	in := docker.M2EEOptions{Host: "127.0.0.1", Port: 8090, Token: "x", Direct: true}
	got := devLoopAdminOptions(p, in, adminFlagsSet{})
	if got != in {
		t.Fatalf("no handshake must leave options alone: got %+v", got)
	}
}

// Explicit flags win over the handshake.
func TestDevLoopAdminOptions_FlagsWin(t *testing.T) {
	p := handshakeProject(t)
	publishFakeLoop(t, p, 18091, "loop-secret")
	in := docker.M2EEOptions{Host: "127.0.0.1", Port: 9999, Token: "mine", Direct: true}
	got := devLoopAdminOptions(p, in, adminFlagsSet{port: true, token: true})
	if got.Port != 9999 || got.Token != "mine" {
		t.Fatalf("explicit flags overridden: %+v", got)
	}
	got = devLoopAdminOptions(p, in, adminFlagsSet{})
	if got.Port != 18091 || got.Token != "loop-secret" || !got.Direct {
		t.Fatalf("handshake not applied: %+v", got)
	}
}

// A handshake left behind by a dead loop is ignored, not trusted.
func TestDevLoopAdminOptions_StaleHandshakeIgnored(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{PID: 1 << 30, AdminPort: 18091, AdminPass: "s"}); err != nil {
		t.Fatal(err)
	}
	in := docker.M2EEOptions{Port: 8090}
	if got := devLoopAdminOptions(p, in, adminFlagsSet{}); got.Port != 8090 {
		t.Fatalf("stale handshake used: %+v", got)
	}
}

// `mxcli log` goes through the same resolution: the options it builds from its
// flags must pick up the loop's port when --admin-port was not given.
func TestLogAdminOptions_UsesRunLocalAdminPort(t *testing.T) {
	p := handshakeProject(t)
	publishFakeLoop(t, p, 18092, "loop-secret")
	cmd := newLogFlagsCmd()
	if err := cmd.ParseFlags([]string{"-p", p}); err != nil {
		t.Fatal(err)
	}
	got := logAdminOptions(cmd)
	if got.Port != 18092 || got.Token != "loop-secret" {
		t.Fatalf("log ignores run-local.json: %+v", got)
	}
	cmd = newLogFlagsCmd()
	if err := cmd.ParseFlags([]string{"-p", p, "--admin-port", "9999"}); err != nil {
		t.Fatal(err)
	}
	if got := logAdminOptions(cmd); got.Port != 9999 {
		t.Fatalf("--admin-port overridden: %+v", got)
	}
	if hint := logConnectionHint(cmd, docker.ErrAdminUnreachable); !strings.Contains(hint, ":9999") {
		t.Fatalf("hint names the wrong port: %q", hint)
	}
}

func newLogFlagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "list"}
	c.Flags().StringP("project", "p", "", "")
	c.Flags().String("admin-host", "127.0.0.1", "")
	c.Flags().Int("admin-port", 8090, "")
	c.Flags().String("admin-pass", "mxcli-local-dev", "")
	return c
}
