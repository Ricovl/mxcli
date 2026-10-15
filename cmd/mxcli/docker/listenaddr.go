// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"net"
	"strconv"
)

// ValidateListenAddr refuses a --listen-addr the runtime could not bind to,
// before anything is built or booted. The value goes to the runtime verbatim
// (runtime_listen_addresses), so a typo used to surface only after the build and
// the JVM start, as a bind failure in the runtime log. Empty means the default.
func ValidateListenAddr(addr string) error {
	if addr == "" {
		return nil
	}
	if net.ParseIP(addr) == nil {
		return fmt.Errorf("invalid --listen-addr %q: want an IP address — 127.0.0.1 (loopback, the default), 0.0.0.0 (all IPv4 interfaces), :: (all interfaces) or one interface's address; no host name, port or brackets", addr)
	}
	return nil
}

// isWildcardListenAddr reports whether addr binds every interface (0.0.0.0, ::).
func isWildcardListenAddr(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && ip.IsUnspecified()
}

// appDialHost is the host mxcli connects to the app on, given the address the
// app binds to. A wildcard is a bind address, not a destination: dialling
// 0.0.0.0 only works by accident on some platforms, and `::` in a URL without
// brackets does not parse. So a wildcard dials loopback of the same family; any
// other address is dialled as is (an app bound to one interface's address is not
// on loopback at all).
func appDialHost(listen string) string {
	ip := net.ParseIP(listen)
	if ip == nil || !ip.IsUnspecified() {
		return listen
	}
	if ip.To4() != nil {
		return "127.0.0.1"
	}
	return "::1"
}

// appURLFor is the base URL of an app bound to listen:port, as mxcli dials it.
// net.JoinHostPort brackets an IPv6 host.
func appURLFor(listen string, port int) string {
	return "http://" + net.JoinHostPort(appDialHost(listen), strconv.Itoa(port)) + "/"
}

// networkAppURLs lists the URLs another machine can reach a wildcard-bound app
// at: one per non-loopback, non-link-local address of this host, of the family
// the wildcard covers (0.0.0.0 is IPv4 only; :: is both). Nil for a non-wildcard
// bind, whose AppURL already names the one address it serves on.
func networkAppURLs(listen string, port int, addrs []net.Addr) []string {
	bind := net.ParseIP(listen)
	if bind == nil || !bind.IsUnspecified() {
		return nil
	}
	v4Only := bind.To4() != nil
	var out []string
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		if v4Only && ip.To4() == nil {
			continue
		}
		out = append(out, "http://"+net.JoinHostPort(ip.String(), strconv.Itoa(port))+"/")
	}
	return out
}
