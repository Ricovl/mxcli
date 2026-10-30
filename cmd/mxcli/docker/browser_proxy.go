// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

// browser_proxy.go keeps the browsers mxcli launches against an app from
// routing a connection to this machine through an egress proxy.
//
// Chromium on Linux takes its proxy from the environment (http_proxy,
// https_proxy, all_proxy, no_proxy). For a WebSocket it prefers the HTTPS
// proxy, ws:// included (RFC 6455 §4.1.3). So where only https_proxy is set —
// the Claude Code agent proxy is one — every http:// load of a local app goes
// direct while the Mendix runtime's dev channel, ws://<host>/mxdevtools/, goes
// to the proxy unless <host> is in no_proxy. localhost and 127.0.0.1 are; the
// app's own host name usually is not: an /etc/hosts entry such as
// frontend.local, which is how two local apps get separate cookie jars. A proxy
// elsewhere cannot reach this machine's loopback, and the agent proxy answers
// the handshake "502 upstream request failed". Chromium logs that as a console
// error, twice, and `playwright check` failed a page that had rendered
// (ako/mxcli#1800). With http_proxy set too, the page itself goes the same way.
//
// The runtime is not the cause: it answers the handshake 101 on the configured
// host name (measured on 11.15.0, curl with the proxy bypassed). Nor is
// `run --local`, which puts nothing between the browser and the runtime.
//
// So a host that resolves only to this machine is added to no_proxy for the
// browser mxcli starts. Nothing is hidden by it: the channel then connects as
// it does on 127.0.0.1, and any console error the page does have still counts.

// lookupHostIPs resolves a host name. A seam for tests.
var lookupHostIPs = func(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, nil
}

// thisMachineIPs lists the addresses assigned to this machine's interfaces.
// A seam for tests.
var thisMachineIPs = func() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			ips = append(ips, n.IP)
		}
	}
	return ips
}

// directRouteEnv returns env for a browser that will load urls, with every
// host among them that resolves only to this machine added to no_proxy and
// NO_PROXY. env is returned unchanged when it configures no proxy, or when no
// such host needs adding; the proxy settings themselves are never touched, so
// a remote page (a --hub preview URL) is still fetched through the proxy.
func directRouteEnv(env []string, urls ...string) []string {
	if !envConfiguresProxy(env) {
		return env
	}
	// Chromium reads no_proxy, and NO_PROXY only when that is unset.
	effective, ok := envGet(env, "no_proxy")
	if !ok {
		effective, _ = envGet(env, "NO_PROXY")
	}
	var hosts []string
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "" || containsFold(hosts, host) || withBypassHosts(effective, []string{host}) == effective ||
			!resolvesToThisMachine(host) {
			continue
		}
		hosts = append(hosts, host)
	}
	if len(hosts) == 0 {
		return env
	}
	out := append([]string(nil), env...)
	for _, key := range []string{"no_proxy", "NO_PROXY"} {
		cur, _ := envGet(env, key)
		if merged := withBypassHosts(cur, hosts); merged != cur {
			// Appended: os/exec gives a child the last value of a duplicate key.
			out = append(out, key+"="+merged)
		}
	}
	return out
}

// envConfiguresProxy reports whether env sets any proxy Chromium reads.
func envConfiguresProxy(env []string) bool {
	for _, key := range []string{"http_proxy", "https_proxy", "all_proxy"} {
		for _, k := range []string{key, strings.ToUpper(key)} {
			if v, _ := envGet(env, k); strings.TrimSpace(v) != "" {
				return true
			}
		}
	}
	return false
}

// resolvesToThisMachine reports whether every address host resolves to is
// loopback or assigned to this machine. A host that does not resolve, or that
// also resolves elsewhere, is left to the proxy.
func resolvesToThisMachine(host string) bool {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		var err error
		if ips, err = lookupHostIPs(host); err != nil || len(ips) == 0 {
			return false
		}
	}
	local := thisMachineIPs()
	for _, ip := range ips {
		if ip.IsLoopback() {
			continue
		}
		mine := false
		for _, l := range local {
			if l.Equal(ip) {
				mine = true
				break
			}
		}
		if !mine {
			return false
		}
	}
	return true
}

// withBypassHosts appends hosts to a comma-separated no_proxy list, skipping
// any it already names. An IPv6 literal is bracketed, as bypass lists write it.
func withBypassHosts(list string, hosts []string) string {
	var have []string
	for _, e := range strings.Split(list, ",") {
		if e = strings.TrimSpace(e); e != "" {
			have = append(have, e)
		}
	}
	out := strings.TrimRight(strings.TrimSpace(list), ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil && ip.To4() == nil {
			h = "[" + h + "]"
		}
		if containsFold(have, h) {
			continue
		}
		if out != "" {
			out += ","
		}
		out += h
		have = append(have, h)
	}
	return out
}

// envGet returns the value of key in env, the last one when it repeats.
func envGet(env []string, key string) (string, bool) {
	val, ok := "", false
	for _, kv := range env {
		if k, v, found := strings.Cut(kv, "="); found && k == key {
			val, ok = v, true
		}
	}
	return val, ok
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
