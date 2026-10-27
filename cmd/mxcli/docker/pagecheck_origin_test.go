// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1216: `playwright check http://frontend.local:8180/ -p app.mpr
// --role R` failed every time with "the sign-in form is showing", while the
// same command without the URL passed. With -p the base URL comes from the
// running loop's handshake, http://localhost:<port>, and the sign-in ran against
// that — so the session cookies belonged to localhost, and the browser sent none
// of them to frontend.local. Cookies are scoped to a host, not to an app.
//
// Reproduced on a real 11.13.0 app with the host spelled differently
// (`playwright check http://127.0.0.1:8180/` against a loop the handshake lists
// as localhost:8180): FAIL, sign-in form; `http://localhost:8180/` and no URL
// both OK. Every failing run also signed in again — one more session on the
// unlicensed runtime per run, counted with the M2EE runtime_statistics action.

// cookieJarBrowser stands in for the login script, the probe and the sign-out,
// keeping the two properties this bug is about: a session saved by signing in
// on one host is not sent to another, and every sign-in is one more live
// session on the runtime until something signs it out.
type cookieJarBrowser struct {
	logins   []LoginOptions
	probes   []ProbeOptions
	signOuts []SignOutOptions
	live     map[string]bool // the runtime's sessions, by cookie value
	// forbidden pages render the sign-in form even when signed in, as the
	// client does for a page the user's role may not open (measured on 11.13:
	// /login.html?messageCode=403#/secret).
	forbidden  string
	signOutErr error
}

// fakeStorage is the shape of a Playwright storage state, reduced to cookies.
type fakeStorage struct {
	Cookies []struct {
		Value  string `json:"value"`
		Domain string `json:"domain"`
	} `json:"cookies"`
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func readFakeStorage(path string) fakeStorage {
	var st fakeStorage
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func (f *cookieJarBrowser) install(t *testing.T) {
	t.Helper()
	f.live = map[string]bool{}
	oldProbe, oldLogin, oldSignOut := probePagesFn, loginFn, signOutFn
	t.Cleanup(func() { probePagesFn, loginFn, signOutFn = oldProbe, oldLogin, oldSignOut })
	loginFn = func(o LoginOptions) error {
		f.logins = append(f.logins, o)
		// A new session on the runtime, its cookie set for the host the
		// sign-in ran on.
		id := fmt.Sprintf("s%d", len(f.logins))
		f.live[id] = true
		body := `{"cookies":[{"name":"XASSESSIONID","value":"` + id + `","domain":"` + hostOf(o.AppURL) + `"}]}`
		if err := os.MkdirAll(filepath.Dir(o.StoragePath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(o.StoragePath, []byte(body), 0o600)
	}
	probePagesFn = func(o ProbeOptions) ([]PageSignals, error) {
		f.probes = append(f.probes, o)
		st := readFakeStorage(o.Storage)
		out := make([]PageSignals, len(o.Pages))
		for i, p := range o.Pages {
			signedIn := false
			for _, c := range st.Cookies {
				if c.Domain == hostOf(p.URL) && f.live[c.Value] {
					signedIn = true
				}
			}
			if signedIn && (f.forbidden == "" || !strings.HasSuffix(p.URL, f.forbidden)) {
				out[i] = healthy()
			} else {
				// What the reporter saw: the client's anonymous session attempt
				// is refused, and the sign-in form renders.
				out[i] = PageSignals{Title: "Login", Headings: []string{"Sign in"}, TextLen: 34, LoginForm: true,
					ConsoleErrors: []string{"[Startup] Could not create a session because the server responded with 401 (Unauthorized)."},
					HTTPErrors:    []string{"401 POST /xas/"}}
			}
			out[i].FinalURL = p.URL
		}
		return out, nil
	}
	signOutFn = func(o SignOutOptions) error {
		f.signOuts = append(f.signOuts, o)
		if f.signOutErr != nil {
			return f.signOutErr
		}
		for _, c := range readFakeStorage(o.StoragePath).Cookies {
			if c.Domain == hostOf(o.AppURL) && f.live[c.Value] {
				delete(f.live, c.Value)
				return nil
			}
		}
		return ErrNoSession
	}
}

// The reported command: the page's host is not the base URL's host.
func TestRunPageCheck_SignsInOnTheHostOfThePage(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	var out bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{
		BaseURL: "http://localhost:8180", Targets: []string{"http://frontend.local:8180/"},
		User: "demo_enthusiast", Password: "x", StorageDir: t.TempDir(), Stdout: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 {
		t.Fatalf("a page the check had just signed in for failed:\n%s", out.String())
	}
	if len(fb.logins) != 1 || hostOf(fb.logins[0].AppURL) != "frontend.local" {
		t.Errorf("signed in on %v, want once on frontend.local", fb.logins)
	}
}

// Control: the same check with the page on the base URL's host passes before
// and after the fix, so the test above fails for the host and nothing else.
func TestRunPageCheck_SameHostControl(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	var out bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{
		BaseURL: "http://localhost:8180", Targets: []string{"http://localhost:8180/"},
		User: "demo_enthusiast", Password: "x", StorageDir: t.TempDir(), Stdout: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 || len(fb.logins) != 1 {
		t.Errorf("control failed (logins=%d):\n%s", len(fb.logins), out.String())
	}
}

// Pages on two hosts get a session each, every page passes, and each session
// is saved under its own host so a later check reuses the right one.
func TestRunPageCheck_PagesOnTwoHosts(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	dir := t.TempDir()
	var out bytes.Buffer
	opts := PageCheckOptions{
		BaseURL: "http://localhost:8180", Targets: []string{"/p/items", "http://127.0.0.1:8180/p/items", "/"},
		User: "demo_user", Password: "x", StorageDir: dir, Stdout: &out,
	}
	res, err := RunPageCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 3 || res.Failed != 0 {
		t.Fatalf("got %+v:\n%s", res, out.String())
	}
	if len(fb.logins) != 2 {
		t.Errorf("logins = %d, want one per host", len(fb.logins))
	}
	// The verdicts keep the order the pages were asked for in.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var pageLines []string
	for _, l := range lines {
		if strings.HasPrefix(l, "page ") {
			pageLines = append(pageLines, strings.Fields(l)[1])
		}
	}
	if strings.Join(pageLines, " ") != "/p/items http://127.0.0.1:8180/p/items /" {
		t.Errorf("verdict order = %v", pageLines)
	}
	// A second check signs in nowhere: both sessions are reused.
	if _, err := RunPageCheck(opts); err != nil {
		t.Fatal(err)
	}
	if len(fb.logins) != 2 {
		t.Errorf("second check signed in again (logins=%d); the sessions were not saved per host", len(fb.logins))
	}
}

// The login failure explanation reads the runtime log; `playwright check`
// never told the login where that is, so the session-cap explanation could not
// fire from the command most likely to exhaust the cap.
func TestRunPageCheck_GivesTheLoginTheRuntimeLog(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	proj := t.TempDir()
	logPath := filepath.Join(proj, ".mxcli", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ERROR - Core: Maximum number of sessions exceeded! (You are currently using a trial license)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunPageCheck(PageCheckOptions{
		BaseURL: "http://localhost:8180", ProjectPath: filepath.Join(proj, "app.mpr"),
		User: "demo_user", Password: "x", Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatal(err)
	}
	if len(fb.logins) != 1 {
		t.Fatalf("logins = %d", len(fb.logins))
	}
	if got := fb.logins[0].RuntimeLogPath; got != logPath {
		t.Errorf("login RuntimeLogPath = %q, want %q", got, logPath)
	}
	if loginFailureHint(fb.logins[0].RuntimeLogPath) == "" {
		t.Error("with that path the session-cap hint still does not fire")
	}
}

// The reported runs, counted on the runtime's side: six checks of the page on
// the other host. Each one used to sign in again beside the last, which is how
// the command came to exhaust the unlicensed runtime's session cap. Measured on
// 11.13 with the M2EE runtime_statistics action: three failing runs, three
// sessions for demo_user. Reused, it is one session however many checks run.
func TestRunPageCheck_RepeatedChecksHoldOneSession(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	opts := PageCheckOptions{
		BaseURL: "http://localhost:8180", Targets: []string{"http://frontend.local:8180/"},
		User: "demo_enthusiast", Password: "x", StorageDir: t.TempDir(), Stdout: &bytes.Buffer{},
	}
	failed := 0
	for i := 0; i < 6; i++ {
		res, err := RunPageCheck(opts)
		if err != nil {
			t.Fatal(err)
		}
		failed += res.Failed
	}
	if failed != 0 || len(fb.live) != 1 || len(fb.logins) != 1 {
		t.Errorf("after six checks: %d failed, %d live session(s), %d sign-in(s); want 0, 1 and 1",
			failed, len(fb.live), len(fb.logins))
	}
}

// --fresh-login replaces the saved session; the one it replaces is signed out
// rather than left to hold a slot until it expires.
func TestRunPageCheck_FreshLoginSignsOutTheSessionItReplaces(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", User: "demo_user", Password: "x",
		StorageDir: t.TempDir(), Stdout: &bytes.Buffer{}}
	if _, err := RunPageCheck(opts); err != nil {
		t.Fatal(err)
	}
	opts.FreshLogin = true
	if _, err := RunPageCheck(opts); err != nil {
		t.Fatal(err)
	}
	if len(fb.logins) != 2 || len(fb.live) != 1 {
		t.Errorf("logins=%d live=%d, want 2 sign-ins and 1 live session", len(fb.logins), len(fb.live))
	}
}

// After a runtime restart the saved session is already gone: --fresh-login
// finds nothing to sign out and says nothing about it.
func TestRunPageCheck_FreshLoginAfterARestart(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	var out, errOut bytes.Buffer
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", User: "demo_user", Password: "x",
		StorageDir: t.TempDir(), Stdout: &out, Stderr: &errOut}
	if _, err := RunPageCheck(opts); err != nil {
		t.Fatal(err)
	}
	fb.live = map[string]bool{} // the runtime restarted
	out.Reset()
	opts.FreshLogin = true
	res, err := RunPageCheck(opts)
	if err != nil || res.Failed != 0 {
		t.Fatalf("%+v %v\n%s", res, err, out.String())
	}
	if strings.Contains(out.String(), "logout") || errOut.Len() != 0 {
		t.Errorf("a session that was already gone was reported signed out:\n%s%s", out.String(), errOut.String())
	}
}

// --sign-out ends the run signed out: no live session and nothing saved, and
// the verdict is the same as without it.
func TestRunPageCheck_SignOutEndsTheRunSignedOut(t *testing.T) {
	fb := &cookieJarBrowser{}
	fb.install(t)
	var out bytes.Buffer
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", Targets: []string{"/p/items"},
		User: "demo_user", Password: "x", StorageDir: t.TempDir(), SignOut: true, Stdout: &out}
	res, err := RunPageCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 {
		t.Errorf("signing out changed the verdict:\n%s", out.String())
	}
	if len(fb.live) != 0 {
		t.Errorf("%d session(s) still live after --sign-out", len(fb.live))
	}
	if _, err := os.Stat(checkStoragePath(opts, "demo_user")); err == nil {
		t.Error("a signed-out session was kept for the next check")
	}
	if !strings.Contains(out.String(), "logout demo_user") {
		t.Errorf("the sign-out is not reported:\n%s", out.String())
	}
}

// A sign-out that fails is reported and changes nothing else: the check
// itself succeeded.
func TestRunPageCheck_FailedSignOutKeepsTheVerdict(t *testing.T) {
	fb := &cookieJarBrowser{signOutErr: fmt.Errorf("no signed-in Mendix client")}
	fb.install(t)
	var out, errOut bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{BaseURL: "http://localhost:8180", User: "demo_user", Password: "x",
		StorageDir: t.TempDir(), SignOut: true, Stdout: &out, Stderr: &errOut})
	if err != nil || res.Failed != 0 {
		t.Errorf("a failed sign-out changed the result: %+v %v\n%s", res, err, out.String())
	}
	if !strings.Contains(errOut.String(), "no signed-in Mendix client") {
		t.Errorf("the failed sign-out is not reported: %q", errOut.String())
	}
}

// A session this run signed in that got no page past the sign-in form — here a
// page the role may not open — is signed out and not kept. Kept, every later
// check re-signed-in beside it, one more session per run.
func TestRunPageCheck_SignsOutASessionThatGotNoPage(t *testing.T) {
	fb := &cookieJarBrowser{forbidden: "/p/secret"}
	fb.install(t)
	opts := PageCheckOptions{BaseURL: "http://localhost:8180", Targets: []string{"/p/secret"},
		User: "demo_user", Password: "x", StorageDir: t.TempDir(), Stdout: &bytes.Buffer{}}
	for i := 0; i < 3; i++ {
		res, err := RunPageCheck(opts)
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed != 1 {
			t.Fatalf("run %d: the forbidden page passed", i+1)
		}
	}
	if len(fb.live) != 0 {
		t.Errorf("%d session(s) left live by three checks of a page the role cannot open", len(fb.live))
	}
	// Control: a page the session does get keeps its session for reuse.
	fb.forbidden = ""
	opts.Targets = []string{"/p/items"}
	for i := 0; i < 2; i++ {
		if _, err := RunPageCheck(opts); err != nil {
			t.Fatal(err)
		}
	}
	if len(fb.live) != 1 || len(fb.signOuts) != 3 {
		t.Errorf("control: live=%d signOuts=%d, want the session kept (1) and no further sign-out (3)", len(fb.live), len(fb.signOuts))
	}
}
