// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ako/mxcli#998: `mxcli playwright check / /p/doesnotexist` reported the home
// page's verdict for a page that does not exist, and exited 0:
//
//	page /  title="Mendix - Homepage"  h="Welcome to your homepage"  text=1680  console-errors=0
//	page /p/doesnotexist  title="Mendix - Homepage"  h="Welcome to your homepage"  text=1774  console-errors=0
//	OK 2 page(s)
//
// Measured on 11.13.0 and 11.15.0, both the same: the runtime answers any
// /p/<x> with `301 Location: /#/<x>`, whether or not a page has that URL.
// The client then resolves the fragment. A known page gets its location
// replaced with /p/<x>. An unknown one gets / plus an information dialog,
// "The page you requested was not found. You are redirected to the home
// page.", and the home page renders with HTTP 200 throughout. So the location
// changes, and the final path is the signal: it holds for the client's
// redirect and for any in-place render alike, because both leave a location
// that is not the /p/ path asked for. The dialog's text would also tell, but it
// is translated, so it is reported and not matched.
// URL matching is exact: /p/ITEMS and /p/items/ also went to / on 11.13, and
// the query string survives (/p/items?x=1 stays).

// What the probe returned for the reported pair, with the location it ended on.
func unknownPageSignals() PageSignals {
	return PageSignals{
		RequestedURL: "http://localhost:8180/p/doesnotexist", FinalURL: "http://localhost:8180/",
		Title: "Mendix - Homepage", Headings: []string{"Welcome to your homepage"}, TextLen: 1774, Status: 200,
		Alerts: []string{"The page you requested was not found. You are redirected to the home page."},
	}
}

func TestEvaluatePage_UnknownPageURLFails(t *testing.T) {
	fails := EvaluatePage(unknownPageSignals(), CheckSpec{})
	if len(fails) != 1 || !strings.Contains(fails[0], "ended on /") || !strings.Contains(fails[0], "/p/doesnotexist") {
		t.Errorf("a /p/ URL that landed on the home page passed, or failed for something else: %v", fails)
	}
}

// Controls: a page that stayed where it was asked for passes — the known page,
// the root, and the root asked for without its slash.
func TestEvaluatePage_PageThatStayedPasses(t *testing.T) {
	for _, c := range []struct{ req, fin string }{
		{"http://localhost:8180/p/items", "http://localhost:8180/p/items"},
		{"http://localhost:8180/p/items?x=1", "http://localhost:8180/p/items?x=1"},
		{"http://localhost:8180/", "http://localhost:8180/"},
		{"http://localhost:8180", "http://localhost:8180/"},
		// Not a /p/ URL: where the client takes a fragment route is its business.
		{"http://localhost:8180/#/items", "http://localhost:8180/p/items"},
		// A proxy upgrading the scheme leaves the page the same.
		{"http://app.example.com/p/items", "https://app.example.com/p/items"},
	} {
		s := healthy()
		s.RequestedURL, s.FinalURL = c.req, c.fin
		if f := EvaluatePage(s, CheckSpec{}); len(f) != 0 {
			t.Errorf("%s -> %s failed: %v", c.req, c.fin, f)
		}
	}
}

// Anonymous, a /p/ URL lands on /login.html#/<x>. That is the sign-in form's
// failure, said once, not also a wrong-page one.
func TestEvaluatePage_SignInRedirectIsOneReason(t *testing.T) {
	s := PageSignals{RequestedURL: "http://localhost:8180/p/items", FinalURL: "http://localhost:8180/login.html#/items",
		Title: "Login", TextLen: 34, LoginForm: true}
	f := EvaluatePage(s, CheckSpec{})
	if len(f) != 1 || !strings.Contains(f[0], "sign-in form") {
		t.Errorf("got %v, want only the sign-in failure", f)
	}
}

// The verdict says where the browser ended when it is not where it was sent,
// and stays as it was when it is.
func TestVerdictReportsTheURLTheBrowserEndedOn(t *testing.T) {
	got := formatPageVerdict("/p/doesnotexist", unknownPageSignals())
	if !strings.Contains(got, "url=/ ") {
		t.Errorf("the final URL is not reported:\n%s", got)
	}
	if !strings.Contains(got, "was not found") {
		t.Errorf("the client's own not-found message is not reported:\n%s", got)
	}
	s := healthy()
	s.RequestedURL, s.FinalURL = "http://localhost:8180", "http://localhost:8180/"
	if got := formatPageVerdict("home", s); strings.Contains(got, "url=") {
		t.Errorf("a page that stayed put reports a url=:\n%s", got)
	}
}

// The reported command end to end, with the probe answering as the measured
// client does.
func TestRunPageCheck_UnknownPageURLFailsTheCheck(t *testing.T) {
	fb := &fakeBrowser{answer: func(_ int, p PageTarget) PageSignals {
		s := PageSignals{Title: "Mendix - Homepage", Headings: []string{"Welcome to your homepage"},
			TextLen: 1680, Status: 200, RequestedURL: p.URL, FinalURL: "http://localhost:8180/"}
		if strings.HasSuffix(p.URL, "/p/doesnotexist") {
			s.TextLen = 1774
		}
		return s
	}}
	fb.install(t)
	var out bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{BaseURL: "http://localhost:8180",
		Targets: []string{"/", "/p/doesnotexist"}, Stdout: &out})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || !strings.Contains(out.String(), "FAIL 1 of 2 page(s)") {
		t.Errorf("want the unknown page to fail and the home page to pass:\n%s", out.String())
	}
}

// fakeRoutingApp serves what the runtime and the client measurably do with a
// /p/ URL: a 301 to /#/<x>, then a client that replaces the location with
// /p/<x> for a page it knows and with / (plus the not-found dialog) for one it
// does not.
func fakeRoutingApp() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/p/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/#/"+strings.TrimPrefix(r.URL.Path, "/p/"), http.StatusMovedPermanently)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Mendix</title></head><body><script>
const route = location.hash.replace(/^#\//, '');
const page = document.createElement('div'); page.className = 'mx-page';
if (route === 'items') {
  history.replaceState(null, '', '/p/items' + location.search);
  document.title = 'Mendix - Items'; page.innerHTML = '<h1>Item overview</h1>';
} else {
  history.replaceState(null, '', '/');
  document.title = 'Mendix - Homepage'; page.innerHTML = '<h1>Welcome to your homepage</h1>';
  if (route) page.innerHTML += '<div class="modal-dialog mx-dialog-info mx-dialog"><div class="mx-dialog-body">' +
    '<p>The page you requested was not found. You are redirected to the home page.</p></div></div>';
}
document.body.appendChild(page);
</script></body></html>`)
	})
	return httptest.NewServer(mux)
}

// The real probe against the measured routing: it must report where the
// browser ended, and the check must fail the unknown page and pass the known.
func TestProbeReportsWhereAPageURLLanded(t *testing.T) {
	srv := fakeRoutingApp()
	defer srv.Close()
	opts := ProbeOptions{Pages: []PageTarget{{URL: srv.URL + "/p/doesnotexist"}, {URL: srv.URL + "/p/items"}}, WaitMs: 3000}
	opts.MxBuildPath = anyMxBuildPath()
	sigs, err := ProbePages(opts)
	if err != nil {
		if strings.Contains(err.Error(), "node not found") || strings.Contains(err.Error(), "Cannot find module") ||
			strings.Contains(err.Error(), "Executable doesn't exist") {
			t.Skipf("no browser tooling here (set NODE_PATH to a Playwright install): %v", err)
		}
		t.Fatal(err)
	}
	unknown, known := sigs[0], sigs[1]
	if unknown.RequestedURL != srv.URL+"/p/doesnotexist" || unknown.FinalURL != srv.URL+"/" {
		t.Errorf("unknown page: requested %q, ended %q", unknown.RequestedURL, unknown.FinalURL)
	}
	if f := EvaluatePage(unknown, CheckSpec{}); len(f) == 0 {
		t.Errorf("the unknown /p/ URL passed:\n%s", formatPageVerdict("/p/doesnotexist", unknown))
	}
	if f := EvaluatePage(known, CheckSpec{}); len(f) != 0 {
		t.Errorf("the known page failed: %v\n%s", f, formatPageVerdict("/p/items", known))
	}
}
