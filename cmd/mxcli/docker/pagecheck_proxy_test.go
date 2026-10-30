// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ako/mxcli#1800, end to end through the real probe: a page on this machine,
// reached by a name the environment's no_proxy does not list, opens the
// runtime's dev channel ws://<host>/mxdevtools/. Chromium sends a ws:// URL to
// the HTTPS proxy (RFC 6455 §4.1.3) — here a fake one that, like the agent
// proxy of the report, cannot reach the app and answers the handshake 502 —
// while the page's own http:// loads go direct and render. The check failed
// the page on the two handshake errors.
//
// The host is this machine's own non-loopback address, an IP literal: Chromium
// bypasses localhost and 127.0.0.0/8 by itself, and a host name that resolves
// to loopback would need an /etc/hosts entry. The routing decision is the same
// one: the host is not in no_proxy. Host-name resolution is covered by
// browser_proxy_test.go.

func TestProbeReachesALocalAppsDevChannelDirectly(t *testing.T) {
	host := nonLoopbackAddrOrSkip(t)
	proxy := newRefusingProxy(t)
	app := fakeDevModePage(t, host)
	proxyEnv(t, proxy.url)

	// Control: with the address not known as this machine's, the channel goes
	// through the proxy and the reported symptom appears.
	stubResolver(t, nil)
	sig := probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: app.URL + "/"}}, WaitMs: 3000})
	if !has502Handshake(sig.ConsoleErrors) || proxy.connects.Load() == 0 {
		t.Fatalf("control: the harness did not reproduce the proxied handshake (connects=%d): %q",
			proxy.connects.Load(), sig.ConsoleErrors)
	}

	// The fix: the same page, with the address known as this machine's.
	stubResolver(t, nil, host)
	proxy.connects.Store(0)
	sig = probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: app.URL + "/"}}, WaitMs: 3000})
	if len(sig.ConsoleErrors) != 0 {
		t.Errorf("console errors for a page that rendered: %q", sig.ConsoleErrors)
	}
	if n := proxy.connects.Load(); n != 0 {
		t.Errorf("the browser sent %d request(s) for an app on this machine to the proxy", n)
	}
	if fails := EvaluatePage(sig, CheckSpec{}); len(fails) != 0 {
		t.Errorf("the page fails the check: %v", fails)
	}
}

func has502Handshake(errs []string) bool {
	for _, e := range errs {
		if strings.Contains(e, "/mxdevtools/") && strings.Contains(e, "Unexpected response code: 502") {
			return true
		}
	}
	return false
}

// nonLoopbackAddrOrSkip returns an IPv4 address of this machine that is not
// loopback, to serve the fake app on.
func nonLoopbackAddrOrSkip(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("no interface addresses: %v", err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			return n.IP.String()
		}
	}
	t.Skip("this machine has no non-loopback IPv4 address to serve the fake app on")
	return ""
}

// proxyEnv gives the probe the environment of the report: an HTTPS proxy and
// nothing else, with only the loopback names exempt.
func proxyEnv(t *testing.T, proxyURL string) {
	for _, k := range []string{"https_proxy", "HTTPS_PROXY"} {
		t.Setenv(k, proxyURL)
	}
	for _, k := range []string{"http_proxy", "HTTP_PROXY", "all_proxy", "ALL_PROXY"} {
		t.Setenv(k, "")
	}
	for _, k := range []string{"no_proxy", "NO_PROXY"} {
		t.Setenv(k, "localhost,127.0.0.1,::1")
	}
}

// refusingProxy accepts a CONNECT and answers whatever is sent through the
// tunnel with the agent proxy's 502, as measured: `curl -p -x <proxy>
// http://frontend.local:8180/mxdevtools/` → 200 Connection Established, then
// 502 Bad Gateway "upstream request failed".
type refusingProxy struct {
	url      string
	connects atomic.Int32
}

func newRefusingProxy(t *testing.T) *refusingProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &refusingProxy{url: "http://" + ln.Addr().String()}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go p.serve(c)
		}
	}()
	return p
}

func (p *refusingProxy) serve(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		fmt.Fprint(c, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
		return
	}
	p.connects.Add(1)
	fmt.Fprint(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
	if _, err := http.ReadRequest(br); err != nil {
		return
	}
	body := "upstream request failed"
	fmt.Fprintf(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain; charset=utf-8\r\n"+
		"Content-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
}

// fakeDevModePage serves a rendered page on host that opens the dev channel
// the way the Mendix client does, and accepts the WebSocket handshake as the
// runtime does (101) — so any failure of the channel is the route's.
func fakeDevModePage(t *testing.T, host string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Mendix - Narrate</title></head><body>
<div class="mx-page"><h1>Narrate</h1><p>The page rendered.</p></div>
<script>new WebSocket('ws://' + location.host + '/mxdevtools/');</script></body></html>`)
	})
	mux.HandleFunc("/mxdevtools/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "websocket only", http.StatusBadRequest)
			return
		}
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		// Hold the socket open, as the runtime's server-push channel does,
		// until the browser closes the page.
		go func() { _, _ = io.Copy(io.Discard, conn); conn.Close() }()
	})
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("cannot listen on %s: %v", host, err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
