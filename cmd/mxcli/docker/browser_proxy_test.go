// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"net"
	"strings"
	"testing"
)

// ako/mxcli#1800: `mxcli playwright check http://frontend.local:8180/` failed a
// page that rendered, on two console errors:
//
//	WebSocket connection to 'ws://frontend.local:8180/mxdevtools/' failed:
//	Error during WebSocket handshake: Unexpected response code: 502
//
// frontend.local is an /etc/hosts name for 127.0.0.1. The runtime answers that
// handshake with 101 on the name; the 502 is the environment's HTTPS proxy,
// which Chromium uses for ws:// (RFC 6455 §4.1.3) and which cannot reach this
// machine's loopback. 127.0.0.1 and localhost are in no_proxy, which is why the
// same check on 127.0.0.1 passed. The browser mxcli launches must reach an app
// on this machine directly, whatever the name.

// stubResolver makes host resolution and the machine's own addresses
// deterministic for the duration of a test.
func stubResolver(t *testing.T, hosts map[string][]string, local ...string) {
	t.Helper()
	oldLookup, oldLocal := lookupHostIPs, thisMachineIPs
	t.Cleanup(func() { lookupHostIPs, thisMachineIPs = oldLookup, oldLocal })
	lookupHostIPs = func(host string) ([]net.IP, error) {
		var ips []net.IP
		for _, s := range hosts[host] {
			ips = append(ips, net.ParseIP(s))
		}
		if len(ips) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return ips, nil
	}
	thisMachineIPs = func() []net.IP {
		var ips []net.IP
		for _, s := range local {
			ips = append(ips, net.ParseIP(s))
		}
		return ips
	}
}

// The environment of the report: an HTTPS proxy, and a no_proxy that lists the
// loopback names but not the app's own host name.
var agentProxyEnv = []string{
	"PATH=/usr/bin",
	"https_proxy=http://127.0.0.1:40291",
	"HTTPS_PROXY=http://127.0.0.1:40291",
	"no_proxy=localhost,127.0.0.1,::1",
	"NO_PROXY=localhost,127.0.0.1,::1",
}

func TestDirectRouteEnv_ALoopbackHostNameBypassesTheProxy(t *testing.T) {
	stubResolver(t, map[string][]string{"frontend.local": {"127.0.0.1"}})
	env := directRouteEnv(agentProxyEnv, "http://frontend.local:8180/", "http://frontend.local:8180/p/narrate")
	for _, key := range []string{"no_proxy", "NO_PROXY"} {
		got, _ := envGet(env, key)
		if got != "localhost,127.0.0.1,::1,frontend.local" {
			t.Errorf("%s = %q: the browser sends ws://frontend.local:8180/mxdevtools/ to the HTTPS proxy, "+
				"which answers 502", key, got)
		}
	}
	// The proxy itself is left as it was: a remote page still needs it.
	if got, _ := envGet(env, "https_proxy"); got != "http://127.0.0.1:40291" {
		t.Errorf("https_proxy = %q, want it untouched", got)
	}
}

// An address assigned to this machine is as unreachable for a proxy elsewhere
// as loopback is — the case of an app on --listen-addr 0.0.0.0 checked through
// the machine's own name or address.
func TestDirectRouteEnv_AnAddressOfThisMachineBypassesTheProxy(t *testing.T) {
	stubResolver(t, map[string][]string{"devbox": {"192.0.2.2"}}, "192.0.2.2")
	env := directRouteEnv(agentProxyEnv, "http://devbox:8080/", "http://192.0.2.2:8080/")
	if got, _ := envGet(env, "no_proxy"); got != "localhost,127.0.0.1,::1,devbox,192.0.2.2" {
		t.Errorf("no_proxy = %q, want devbox and 192.0.2.2 added", got)
	}
}

// Controls: what is not on this machine keeps the proxy, and nothing changes
// where there is no proxy to bypass.
func TestDirectRouteEnv_LeavesEverythingElseAlone(t *testing.T) {
	stubResolver(t, map[string][]string{
		"frontend.local":     {"127.0.0.1"},
		"preview.example":    {"203.0.113.7"},
		"split.example":      {"127.0.0.1", "203.0.113.7"},
		"localhost":          {"127.0.0.1", "::1"},
		"already.listed.dev": {"127.0.0.1"},
	})
	cases := []struct {
		name string
		env  []string
		urls []string
	}{
		{"a remote host", agentProxyEnv, []string{"https://preview.example/"}},
		{"a host that also resolves elsewhere", agentProxyEnv, []string{"http://split.example:8080/"}},
		{"a host that does not resolve", agentProxyEnv, []string{"http://nowhere.invalid:8080/"}},
		{"a host no_proxy already lists", []string{"https_proxy=http://p:1", "no_proxy=already.listed.dev"}, []string{"http://ALREADY.listed.dev:8080/"}},
		{"no proxy configured", []string{"PATH=/usr/bin", "no_proxy=localhost"}, []string{"http://frontend.local:8180/"}},
		{"an unparsable URL", agentProxyEnv, []string{"http://[::1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := directRouteEnv(c.env, c.urls...)
			if strings.Join(got, "\n") != strings.Join(c.env, "\n") {
				t.Errorf("env changed:\n got %q\nwant %q", got, c.env)
			}
		})
	}
	// Control for the cases above: the same env does change for a local name.
	if got := directRouteEnv(agentProxyEnv, "http://frontend.local:8180/"); len(got) == len(agentProxyEnv) {
		t.Errorf("control: a loopback host name did not change the env")
	}
}

// Chromium reads no_proxy, falling back to NO_PROXY; an environment that sets a
// proxy and neither gets both.
func TestDirectRouteEnv_SetsNoProxyWhenThereIsNone(t *testing.T) {
	stubResolver(t, map[string][]string{"frontend.local": {"127.0.0.1"}})
	env := directRouteEnv([]string{"HTTPS_PROXY=http://127.0.0.1:40291"}, "http://frontend.local:8180/")
	for _, key := range []string{"no_proxy", "NO_PROXY"} {
		if got, ok := envGet(env, key); !ok || got != "frontend.local" {
			t.Errorf("%s = %q (set %v), want frontend.local", key, got, ok)
		}
	}
}

// An IPv6 literal is written the way a proxy bypass list spells one.
func TestDirectRouteEnv_IPv6LiteralIsBracketed(t *testing.T) {
	stubResolver(t, nil, "fd00::2")
	env := directRouteEnv([]string{"https_proxy=http://p:1"}, "http://[fd00::2]:8080/")
	if got, _ := envGet(env, "no_proxy"); got != "[fd00::2]" {
		t.Errorf("no_proxy = %q, want [fd00::2]", got)
	}
}
