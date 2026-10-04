// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/mendixlabs/mxcli/cmd/mxcli/docker"

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
