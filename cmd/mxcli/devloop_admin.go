// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// adminFlagsSet records which admin connection settings the user gave
// explicitly. Those always win; only the rest are taken from a dev loop.
type adminFlagsSet struct {
	host, port, token bool
}

// devLoopAdminOptions points admin-API options at the `mxcli run --local`
// serving projectPath, when one is live and the user did not say otherwise.
//
// The loop records its admin port and password in .mxcli/run-local.json; it is
// the only place a second process can learn them. Without this, `run --local
// --admin-port 8091` printed a `mxcli oql -p …` hint that failed with "cannot
// connect … localhost:8090", and `mxcli log` failed the same way (ako/mxcli#982).
//
// Precedence: explicit flags > live run-local.json > environment > .docker/.env
// > defaults. A handshake whose process is gone is ignored (readDevLoopHandshake
// refuses it), so a crashed loop cannot redirect a query to whatever took its
// port since. A live loop's admin API is loopback HTTP, never docker exec.
func devLoopAdminOptions(projectPath string, opts docker.M2EEOptions, set adminFlagsSet) docker.M2EEOptions {
	// An explicit host means "not the loop on this machine"; so do both port
	// and password, since nothing would be left to take from the handshake.
	if projectPath == "" || set.host || (set.port && set.token) {
		return opts
	}
	hs, err := readDevLoopHandshake(projectPath)
	if err != nil || hs.AdminPort == 0 {
		return opts
	}
	if !set.port {
		opts.Port = hs.AdminPort
	}
	if !set.token && hs.AdminPass != "" {
		opts.Token = hs.AdminPass
	}
	opts.Host = "127.0.0.1"
	opts.Direct = true
	return opts
}

// devLoopAdminHint explains an admin-API failure for a project whose `mxcli run
// --local` did not supply the connection — its .mxcli/run-local.json missing,
// or naming a process that is gone — so defaults went to whatever answers on
// that port. From a runtime that is up, that is HTTP 403, which reads as a
// wrong password; twice in one day it was a run that had stopped (or a
// handshake deleted under a live one), and the caller blamed the credentials
// (ako/mxcli#1542). The hint says so and quotes `run status`.
//
// It is "" when the handshake was used, when the user named the endpoint
// themselves, and for a failure that is neither reaching nor authenticating.
func devLoopAdminHint(projectPath string, set adminFlagsSet, err error) string {
	if err == nil || projectPath == "" || set.host || (set.port && set.token) {
		return ""
	}
	if !errors.Is(err, docker.ErrAdminAuth) && !errors.Is(err, docker.ErrAdminUnreachable) {
		return ""
	}
	hs, herr := readDevLoopHandshake(projectPath)
	if herr == nil {
		return ""
	}
	why := "is missing"
	if errors.Is(herr, errStaleDevLoopHandshake) {
		why = fmt.Sprintf("names pid %d, which is no longer running", hs.PID)
	} else if !errors.Is(herr, errNoDevLoopHandshake) {
		why = "could not be read"
	}
	hint := fmt.Sprintf("  .mxcli/run-local.json %s, so these were default connection settings, not those of\n"+
		"  this project's 'mxcli run --local'. When that run has stopped, a 401/403 here is NOT a\n"+
		"  wrong password. Check the run before the credentials:\n", why)
	if line := runStatusSummary(projectPath); line != "" {
		return hint + "    mxcli run status -p " + projectPath + "\n    → " + line + "\n"
	}
	return hint + "    mxcli run status -p " + projectPath + "\n"
}
