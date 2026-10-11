// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// testPort is one of the three ports a --local test boot binds, with the flag
// that sets it — so every message about a port can say how to move it.
type testPort struct {
	port int
	role string
	flag string
}

// appPort, adminPort and servePort resolve the configured ports, falling back
// to the defaults the test boot has always used (which deliberately differ from
// `run --local`'s, so the two can run side by side).
func (o RunOptions) appPort() int   { return portOr(o.AppPort, localTestAppPort) }
func (o RunOptions) adminPort() int { return portOr(o.AdminPort, localTestAdminPort) }
func (o RunOptions) servePort() int { return portOr(o.ServePort, localTestServePort) }

func portOr(p, def int) int {
	if p == 0 {
		return def
	}
	return p
}

// testPorts lists the ports a --local boot binds. The serve port is only bound
// when the run builds, so --skip-build leaves it out.
func (o RunOptions) testPorts() []testPort {
	ports := []testPort{
		{o.appPort(), "app", "--app-port"},
		{o.adminPort(), "runtime admin API", "--admin-port"},
	}
	if !o.SkipBuild {
		ports = append(ports, testPort{o.servePort(), "mxbuild serve", "--serve-port"})
	}
	return ports
}

// validatePorts refuses port flags a run would ignore, and values that cannot
// work, before anything is built.
func validatePorts(opts RunOptions) error {
	set := []testPort{
		{opts.AppPort, "app", "--app-port"},
		{opts.AdminPort, "runtime admin API", "--admin-port"},
		{opts.ServePort, "mxbuild serve", "--serve-port"},
	}
	for _, p := range set {
		if p.port == 0 {
			continue
		}
		if !opts.Local || opts.Attach {
			return fmt.Errorf("%s applies only to a --local test run "+
				"(--attach uses the ports of the app it attaches to; the Docker path configures the container's)", p.flag)
		}
		if p.port < 1 || p.port > 65535 {
			return fmt.Errorf("%s %d is not a valid port (1-65535)", p.flag, p.port)
		}
	}
	all := []testPort{
		{opts.appPort(), "app", "--app-port"},
		{opts.adminPort(), "runtime admin API", "--admin-port"},
		{opts.servePort(), "mxbuild serve", "--serve-port"},
	}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[i].port == all[j].port {
				return fmt.Errorf("%s and %s resolve to the same port %d; each needs its own",
					all[i].flag, all[j].flag, all[i].port)
			}
		}
	}
	return nil
}

// checkTestPortsFree probes every port a --local boot will bind and fails on
// the first one already taken, naming the port and the flag that moves it.
//
// StartLocalApp has its own guard, but it runs only after the test endpoint has
// been injected into the project and cannot name `mxcli test`'s flags. Probing
// here fails before the project is touched, with a message the user can act on.
func checkTestPortsFree(opts RunOptions) error {
	for _, p := range opts.testPorts() {
		if err := docker.CheckPortFree(p.port, p.role, p.flag); err != nil {
			return err
		}
	}
	return nil
}
