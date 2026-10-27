// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/security"
)

// `mxcli playwright check /p/secret /p/items -p app.mpr --role User`, where the
// User role may not open /p/secret, signed in and then reported
//
//	page /p/secret  url=/login.html?messageCode=403#/secret  LOGIN PAGE  title="Login"  h="Sign in"  text=34  console-errors=1
//	  ERR    [Startup] An error occurred while creating a new session. i_: Internal server error …
//	  HTTP   403 POST /xas/
//	  FAIL   the sign-in form is showing, not the page (pass --user/--password or --role)
//
// — telling a signed-in user to sign in. Every later run then signed in again
// beside the live session (it read as one the runtime had forgotten), one
// more session per run; checked alone, the page had its working session
// signed out after every run.
//
// Measured on 11.13.0 and 11.15.0, the same on both. The runtime answers the
// client's POST /xas/ get_session_data (referrer /secret) with 403 and the
// client goes to /login.html?messageCode=403#/secret: the sign-in form, whose
// message element (#loginMessage) stays empty and hidden, because login.js
// reads the code as `"403".split("=")[1]`. Anonymous, and with a session the
// runtime has forgotten, the same request is answered 401 and the client goes
// to /login.html#/secret — the same form, no messageCode — and that holds with
// guest access enabled too (11.15). So the query parameter is the signal; the
// page itself has nothing in it to tell the two apart, and its text is
// translated anyway.

// What the probe returned for /p/secret, signed in as a user whose role may not
// open it (11.13.0, from the live run above).
func deniedSignals(requested string) PageSignals {
	u, _ := url.Parse(requested)
	page := strings.TrimPrefix(u.Path, "/p/")
	return PageSignals{
		RequestedURL: requested, FinalURL: u.Scheme + "://" + u.Host + "/login.html?messageCode=403#/" + page,
		Title: "Login", Headings: []string{"Sign in"}, TextLen: 34, Status: 200, LoginForm: true,
		ConsoleErrors: []string{"[Startup] An error occurred while creating a new session. i_: Internal server error\n    at MO (http://localhost:8180/dist/chunks/y6JB9J-k.js:32:33456)"},
		HTTPErrors:    []string{"403 POST /xas/"},
	}
}

// What the probe returned for the same page with no session (11.13.0).
func anonymousSignInSignals(requested string) PageSignals {
	u, _ := url.Parse(requested)
	page := strings.TrimPrefix(u.Path, "/p/")
	return PageSignals{
		RequestedURL: requested, FinalURL: u.Scheme + "://" + u.Host + "/login.html#/" + page,
		Title: "Login", Headings: []string{"Sign in"}, TextLen: 34, Status: 200, LoginForm: true,
		ConsoleErrors: []string{"[Startup] Could not create a session because the server responded with 401 (Unauthorized)."},
		HTTPErrors:    []string{"401 POST /xas/"},
	}
}

func TestEvaluatePage_RoleDeniedPageIsAccessDenied(t *testing.T) {
	s := deniedSignals("http://localhost:8180/p/secret")
	s.SignedInAs = "demo_user (user role User)"
	fails := EvaluatePage(s, CheckSpec{})
	if len(fails) != 1 {
		t.Fatalf("want one reason, got %d: %q", len(fails), fails)
	}
	f := fails[0]
	for _, want := range []string{"access denied", "demo_user (user role User)", "/p/secret", "403"} {
		if !strings.Contains(f, want) {
			t.Errorf("the failure does not say %q: %q", want, f)
		}
	}
	// The user is signed in: advice to sign in, or to pass credentials, is wrong.
	for _, wrong := range []string{"sign-in form", "not signed in", "--user", "--password", "--role"} {
		if strings.Contains(f, wrong) {
			t.Errorf("the failure for a signed-in user's refused page says %q: %q", wrong, f)
		}
	}
}

// Without a known identity the failure still names the refusal, and still
// does not send the user to sign in.
func TestEvaluatePage_RoleDeniedWithoutIdentity(t *testing.T) {
	fails := EvaluatePage(deniedSignals("http://localhost:8180/p/secret"), CheckSpec{})
	if len(fails) != 1 || !strings.Contains(fails[0], "access denied") || !strings.Contains(fails[0], "/p/secret") ||
		strings.Contains(fails[0], "sign-in form") {
		t.Errorf("got %q", fails)
	}
}

// Control: the anonymous sign-in form is still the sign-in failure.
func TestEvaluatePage_AnonymousSignInFormIsUnchanged(t *testing.T) {
	fails := EvaluatePage(anonymousSignInSignals("http://localhost:8180/p/secret"), CheckSpec{})
	if len(fails) != 1 || !strings.Contains(fails[0], "the sign-in form is showing") {
		t.Errorf("got %q, want only the sign-in failure", fails)
	}
}

// The retry and sign-out decisions hang on NeedsLogin. A refused page is not a
// session problem: the runtime knew the session and said no to the page, and
// signing in again as the same user gets the same answer.
func TestNeedsLogin_RoleDeniedIsNotASessionProblem(t *testing.T) {
	denied := deniedSignals("http://localhost:8180/p/secret")
	if !denied.AccessDenied() {
		t.Error("the 403 landing is not recognised as access denied")
	}
	if denied.NeedsLogin() {
		t.Error("a page refused to a signed-in user reads as needing a sign-in")
	}
	anon := anonymousSignInSignals("http://localhost:8180/p/secret")
	if anon.AccessDenied() || !anon.NeedsLogin() {
		t.Errorf("control: the anonymous sign-in form: AccessDenied=%v NeedsLogin=%v", anon.AccessDenied(), anon.NeedsLogin())
	}
}

// The verdict line, which `run --page-check` prints too, says what happened
// rather than LOGIN PAGE.
func TestVerdictSaysAccessDenied(t *testing.T) {
	got := formatPageVerdict("/p/secret", deniedSignals("http://localhost:8180/p/secret"))
	if !strings.Contains(got, "ACCESS DENIED") || strings.Contains(got, "LOGIN PAGE") {
		t.Errorf("verdict:\n%s", got)
	}
	if !strings.Contains(got, "url=/login.html?messageCode=403#/secret") {
		t.Errorf("the landing URL is not reported:\n%s", got)
	}
	if got := formatPageVerdict("/p/secret", anonymousSignInSignals("http://localhost:8180/p/secret")); !strings.Contains(got, "LOGIN PAGE") {
		t.Errorf("control: the anonymous sign-in form is no longer LOGIN PAGE:\n%s", got)
	}
}

// The reported pair, three runs on one saved session: the refused page fails
// as access denied, the other passes, and no run signs in again — before the
// fix runs two and three each signed in beside the live session.
func TestRunPageCheck_RoleDeniedPageKeepsTheSession(t *testing.T) {
	fb := &cookieJarBrowser{denied: "/p/secret"}
	fb.install(t)
	var out bytes.Buffer
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", Targets: []string{"/p/secret", "/p/items"},
		User: "demo_user", Password: "x", StorageDir: t.TempDir(), Stdout: &out}
	for i := 0; i < 3; i++ {
		out.Reset()
		res, err := RunPageCheck(opts)
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed != 1 || !strings.Contains(out.String(), "access denied: signed in as demo_user") ||
			strings.Contains(out.String(), "sign-in form") {
			t.Errorf("run %d:\n%s", i+1, out.String())
		}
	}
	if len(fb.logins) != 1 || len(fb.live) != 1 || len(fb.signOuts) != 0 {
		t.Errorf("after three checks: %d sign-in(s), %d live session(s), %d sign-out(s); want 1, 1 and 0",
			len(fb.logins), len(fb.live), len(fb.signOuts))
	}
}

// Checked alone, a refused page leaves a valid session: it is kept for the
// next check, not signed out as one that "got no page past the sign-in form".
func TestRunPageCheck_RoleDeniedPageAloneKeepsTheSession(t *testing.T) {
	fb := &cookieJarBrowser{denied: "/p/secret"}
	fb.install(t)
	var out bytes.Buffer
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", Targets: []string{"/p/secret"},
		User: "demo_user", Password: "x", StorageDir: t.TempDir(), Stdout: &out}
	for i := 0; i < 3; i++ {
		res, err := RunPageCheck(opts)
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed != 1 {
			t.Fatalf("run %d: the refused page passed:\n%s", i+1, out.String())
		}
	}
	if len(fb.logins) != 1 || len(fb.signOuts) != 0 || len(fb.live) != 1 {
		t.Errorf("after three checks: %d sign-in(s), %d sign-out(s), %d live; want 1, 0 and 1:\n%s",
			len(fb.logins), len(fb.signOuts), len(fb.live), out.String())
	}
}

// --role names the user role in the failure; the role comes from the demo
// user, and --user names no role it does not know.
func TestCredentialsNameTheUserRoles(t *testing.T) {
	ps := &security.ProjectSecurity{SecurityLevel: "CheckEverything", EnableDemoUsers: true,
		DemoUsers: []*security.DemoUser{{UserName: "demo_user", Password: "u", UserRoles: []string{"User"}}}}
	c, err := pickDemoUser(ps, "User")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.signedInAs(); got != "demo_user (user role User)" {
		t.Errorf("--role User: %q", got)
	}
	if got := (credentials{user: "someone"}).signedInAs(); got != "someone" {
		t.Errorf("--user: %q", got)
	}
}

// fakeRefusingApp serves the measured startup: /p/<x> redirects to /#/<x>, the
// client asks POST /xas/ for its session, and on a 403 goes to
// /login.html?messageCode=403#/<x>, on a 401 to /login.html#/<x>. Both land on
// the same sign-in form.
func fakeRefusingApp(sessionStatus int) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/p/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/#/"+strings.TrimPrefix(r.URL.Path, "/p/"), http.StatusMovedPermanently)
	})
	mux.HandleFunc("/xas/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(sessionStatus)
	})
	mux.HandleFunc("/login.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Login</title></head><body>
<h2 id="loginHeader">Sign in</h2><form id="loginForm"><div id="loginMessage" class="alert alert-danger" style="display:none"></div>
<input id="usernameInput"><input id="passwordInput" type="password"><button id="loginButton">Sign in</button></form>
</body></html>`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Mendix</title></head><body><script>
fetch('/xas/', {method: 'POST', body: '{"action":"get_session_data"}'}).then(r => {
  if (r.status === 403) {
    console.error('[Startup] An error occurred while creating a new session. i_: Internal server error');
    location.replace('/login.html?messageCode=403' + location.hash);
  } else if (r.status === 401) {
    location.replace('/login.html' + location.hash);
  }
});
</script></body></html>`)
	})
	return httptest.NewServer(mux)
}

// The real probe against the measured startup: a refused page must come back
// as access denied, and the anonymous one as the sign-in form.
func TestProbeTellsARefusedPageFromTheSignInForm(t *testing.T) {
	denied := fakeRefusingApp(http.StatusForbidden)
	defer denied.Close()
	sig := probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: denied.URL + "/p/secret"}}, WaitMs: 3000})
	if !sig.LoginForm || !strings.Contains(sig.FinalURL, "messageCode=403") {
		t.Fatalf("the probe did not record the 403 landing: loginForm=%v finalUrl=%q", sig.LoginForm, sig.FinalURL)
	}
	sig.SignedInAs = "demo_user (user role User)"
	if f := EvaluatePage(sig, CheckSpec{}); len(f) != 1 || !strings.Contains(f[0], "access denied") {
		t.Errorf("refused page: %q\n%s", f, formatPageVerdict("/p/secret", sig))
	}

	anon := fakeRefusingApp(http.StatusUnauthorized)
	defer anon.Close()
	sig = probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: anon.URL + "/p/secret"}}, WaitMs: 3000})
	if f := EvaluatePage(sig, CheckSpec{}); len(f) != 1 || !strings.Contains(f[0], "the sign-in form is showing") {
		t.Errorf("control, anonymous: %q\n%s", f, formatPageVerdict("/p/secret", sig))
	}
}
