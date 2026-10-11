// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// ChipCoV8 #14: `mxcli test --local` hard-coded 8081/8091/6544, so a machine
// with 8081 taken (a VS Code helper) could not run tests at all. The ports are
// now options, defaulting to the old values.

func TestLocalAppOptions_DefaultPortsUnchanged(t *testing.T) {
	o := localAppOptions(RunOptions{ProjectPath: "/x/App.mpr"}, "", nil, nil)
	if o.AppPort != 8081 || o.AdminPort != 8091 || o.ServePort != 6544 {
		t.Errorf("default ports = %d/%d/%d, want 8081/8091/6544", o.AppPort, o.AdminPort, o.ServePort)
	}
}

func TestLocalAppOptions_CarriesConfiguredPorts(t *testing.T) {
	o := localAppOptions(RunOptions{ProjectPath: "/x/App.mpr", AppPort: 9081, AdminPort: 9091, ServePort: 7544}, "", nil, nil)
	if o.AppPort != 9081 || o.AdminPort != 9091 || o.ServePort != 7544 {
		t.Errorf("ports = %d/%d/%d, want 9081/9091/7544", o.AppPort, o.AdminPort, o.ServePort)
	}
}

func TestEndpointClient_UsesConfiguredAppPort(t *testing.T) {
	c := testEndpointClient(RunOptions{AppPort: 9081}, "tok")
	if !strings.HasPrefix(c.baseURL, "http://127.0.0.1:9081/") {
		t.Errorf("baseURL = %q, want it on the configured app port 9081", c.baseURL)
	}
	// Control: the default is still the old port.
	c = testEndpointClient(RunOptions{}, "tok")
	if !strings.HasPrefix(c.baseURL, "http://127.0.0.1:8081/") {
		t.Errorf("baseURL = %q, want the default app port 8081", c.baseURL)
	}
}

func TestValidateOptions_Ports(t *testing.T) {
	tests := []struct {
		name    string
		opts    RunOptions
		wantErr string
	}{
		{name: "local with ports", opts: RunOptions{Local: true, AppPort: 9081, AdminPort: 9091, ServePort: 7544}},
		{name: "docker path refuses ports", opts: RunOptions{AppPort: 9081}, wantErr: "--app-port"},
		{name: "attach refuses ports", opts: RunOptions{Attach: true, AdminPort: 9091}, wantErr: "--admin-port"},
		{name: "out of range", opts: RunOptions{Local: true, ServePort: 70000}, wantErr: "--serve-port"},
		{name: "collision with a default", opts: RunOptions{Local: true, AdminPort: 8081}, wantErr: "same port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

// The probe must fail fast — before the project is touched — and say which
// port, and which flag moves it.
func TestCheckTestPortsFree_NamesPortAndFlag(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	free := freePort(t)
	free2 := freePort(t)

	for _, tc := range []struct {
		opts RunOptions
		flag string
	}{
		{RunOptions{Local: true, AppPort: busy, AdminPort: free, ServePort: free2}, "--app-port"},
		{RunOptions{Local: true, AppPort: free, AdminPort: busy, ServePort: free2}, "--admin-port"},
		{RunOptions{Local: true, AppPort: free, AdminPort: free2, ServePort: busy}, "--serve-port"},
	} {
		err := checkTestPortsFree(tc.opts)
		if err == nil {
			t.Fatalf("%s: no error with port %d held", tc.flag, busy)
		}
		msg := err.Error()
		if !strings.Contains(msg, fmt.Sprint(busy)) || !strings.Contains(msg, tc.flag) {
			t.Errorf("%s: error does not name the port %d and the flag:\n%s", tc.flag, busy, msg)
		}
	}

	// Control: with nothing held the probe passes, so the error above is the
	// listener's doing and not the probe failing unconditionally.
	if err := checkTestPortsFree(RunOptions{Local: true, AppPort: free, AdminPort: free2, ServePort: freePort(t)}); err != nil {
		t.Errorf("free ports reported busy: %v", err)
	}

	// --skip-build starts no build server, so its port is not probed.
	if err := checkTestPortsFree(RunOptions{Local: true, SkipBuild: true, AppPort: free, AdminPort: free2, ServePort: busy}); err != nil {
		t.Errorf("--skip-build probed the serve port: %v", err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return p
}
