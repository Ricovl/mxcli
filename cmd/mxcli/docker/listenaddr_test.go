// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"net"
	"net/url"
	"strings"
	"testing"
)

// applyDefaults is where the empty-means-loopback default actually lives, so a
// test that skips it would assert a shape the runtime never sees.
func TestApplyDefaultsPinsListenAddr(t *testing.T) {
	var o LocalRuntimeOptions
	o.applyDefaults()
	if o.ListenAddr != "127.0.0.1" {
		t.Errorf("applyDefaults left ListenAddr = %q, want 127.0.0.1", o.ListenAddr)
	}

	var w LocalRuntimeOptions
	w.ListenAddr = "0.0.0.0"
	w.applyDefaults()
	if w.ListenAddr != "0.0.0.0" {
		t.Errorf("applyDefaults overwrote an explicit ListenAddr: %q", w.ListenAddr)
	}
}

// The M2EE admin API is a privileged surface and must never be widened onto the
// network by --listen-addr: whatever the app binds to, the admin bind stays on
// loopback. This is the value that reaches both the runtime's
// M2EE_ADMIN_LISTEN_ADDRESSES and mxcli's own client Host (both use
// m2eeAdminListenAddr), so a regression here is silent otherwise — the app
// comes up and only the admin API is on the network. The want string is
// hard-coded, not read from the constant, so changing the constant fails here.
func TestLocalRuntimeEnvAdminBindIsAlwaysLoopback(t *testing.T) {
	const want = "M2EE_ADMIN_LISTEN_ADDRESSES=127.0.0.1"
	for _, listen := range []string{"", "127.0.0.1", "0.0.0.0", "::", "192.168.2.35"} {
		var o LocalRuntimeOptions
		o.AdminPass = "pw"
		o.ListenAddr = listen
		o.applyDefaults()
		env := localRuntimeEnv(o)
		found := false
		for _, kv := range env {
			if kv == want {
				found = true
			}
			if strings.HasPrefix(kv, "M2EE_ADMIN_LISTEN_ADDRESSES=") && kv != want {
				t.Errorf("ListenAddr %q: admin bind leaked %q, want %q", listen, kv, want)
			}
		}
		if !found {
			t.Errorf("ListenAddr %q: env is missing %q", listen, want)
		}
	}
}

// The app's own bind is the appContainerParams value and is unaffected by the
// admin bind: a caller asking for 0.0.0.0 must get 0.0.0.0, or the flag would
// silently do nothing.
func TestAppContainerParamsKeepsWildcard(t *testing.T) {
	for _, listen := range []string{"", "0.0.0.0", "192.168.2.35"} {
		var o LocalRuntimeOptions
		o.AppPort = 8095
		o.ListenAddr = listen
		o.applyDefaults()
		p := appContainerParams(o)
		got, ok := p["runtime_listen_addresses"].(string)
		if !ok {
			t.Fatalf("ListenAddr %q: runtime_listen_addresses is %T, want string", listen, p["runtime_listen_addresses"])
		}
		want := listen
		if want == "" {
			want = "127.0.0.1"
		}
		if got != want {
			t.Errorf("runtime_listen_addresses for %q = %q, want %q", listen, got, want)
		}
		if got := p["runtime_port"]; got != 8095 {
			t.Errorf("runtime_port = %v, want 8095", got)
		}
	}
}

// AppURL is what mxcli dials (health check, screenshots, "App is running at"),
// so it must be a destination: a wildcard bind maps to loopback, and an IPv6
// host is bracketed — `http://:::8080/` does not parse.
func TestAppURLIsDialable(t *testing.T) {
	cases := map[string]string{
		"":             "http://127.0.0.1:8080/",
		"127.0.0.1":    "http://127.0.0.1:8080/",
		"0.0.0.0":      "http://127.0.0.1:8080/",
		"::":           "http://[::1]:8080/",
		"::1":          "http://[::1]:8080/",
		"192.168.2.35": "http://192.168.2.35:8080/",
		"fe80::1":      "http://[fe80::1]:8080/",
	}
	for listen, want := range cases {
		o := LocalRuntimeOptions{ListenAddr: listen}
		o.applyDefaults()
		rt := &LocalRuntime{opts: o}
		got := rt.AppURL()
		if got != want {
			t.Errorf("ListenAddr %q: AppURL = %q, want %q", listen, got, want)
		}
		u, err := url.Parse(got)
		if err != nil || u.Port() != "8080" {
			t.Errorf("ListenAddr %q: AppURL %q does not parse to port 8080 (err %v)", listen, got, err)
		}
	}
}

func TestValidateListenAddr(t *testing.T) {
	for _, ok := range []string{"", "127.0.0.1", "0.0.0.0", "::", "::1", "192.168.2.35"} {
		if err := ValidateListenAddr(ok); err != nil {
			t.Errorf("ValidateListenAddr(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"localhost", "0.0.0.0:8080", "[::]", "*", "192.168.2", " 0.0.0.0"} {
		if err := ValidateListenAddr(bad); err == nil {
			t.Errorf("ValidateListenAddr(%q) = nil, want an error", bad)
		}
	}
}

func TestNetworkAppURLs(t *testing.T) {
	cidr := func(s string) net.Addr {
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		return n
	}
	addrs := []net.Addr{
		cidr("127.0.0.1/8"), cidr("::1/128"), cidr("fe80::1/64"),
		cidr("192.168.2.35/24"), cidr("2001:db8::5/64"),
	}
	check := func(listen string, want []string) {
		t.Helper()
		got := networkAppURLs(listen, 8080, addrs)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("networkAppURLs(%q) = %v, want %v", listen, got, want)
		}
	}
	check("0.0.0.0", []string{"http://192.168.2.35:8080/"})
	check("::", []string{"http://192.168.2.35:8080/", "http://[2001:db8::5]:8080/"})
	check("127.0.0.1", nil)
	check("192.168.2.35", nil)
}

// The root-URL warning fires only for a wildcard bind that nothing gives a
// reachable root URL: not for loopback, not when the model or hub supplied
// one, not when --runtime-setting did.
func TestWarnWildcardWithoutRootURL(t *testing.T) {
	cases := []struct {
		listen, root string
		settings     map[string]any
		warn         bool
	}{
		{"0.0.0.0", "", nil, true},
		{"::", "", nil, true},
		{"127.0.0.1", "", nil, false},
		{"192.168.2.35", "", nil, false},
		{"0.0.0.0", "http://dev.box:8080/", nil, false},
		{"0.0.0.0", "", map[string]any{"ApplicationRootUrl": "http://dev.box:8080/"}, false},
	}
	for _, c := range cases {
		var b bytes.Buffer
		warnWildcardWithoutRootURL(&b, c.listen, 8080, c.root, c.settings)
		if got := b.Len() > 0; got != c.warn {
			t.Errorf("listen %q root %q settings %v: warned=%v, want %v (%q)", c.listen, c.root, c.settings, got, c.warn, b.String())
		}
	}
}
