// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/security"
)

// `mxcli playwright check` replaces a hand-rolled playwright-cli sequence and
// the screenshot read at its end. Its contract is the exit code — an agent
// branches on it instead of reading pixels — so these pin what fails a page
// and what does not.

func TestParseCountAssertion(t *testing.T) {
	cases := []struct {
		in, sel, op string
		n           int
	}{
		{".mx-listview-item>=1", ".mx-listview-item", ">=", 1},
		// The operator is read from the right, so a child combinator survives.
		{".mx-listview>ul>li>=2", ".mx-listview>ul>li", ">=", 2},
		{"li>3", "li", ">", 3},
		{"li<3", "li", "<", 3},
		{"li<=3", "li", "<=", 3},
		{"li==0", "li", "==", 0},
		{"li=4", "li", "==", 4},
		{"li!=0", "li", "!=", 0},
		{" .row >= 10 ", ".row", ">=", 10},
		// A bare selector, or one that merely ends in digits, means "at least one".
		{".mx-name-dgOrders", ".mx-name-dgOrders", ">=", 1},
		{".col-6", ".col-6", ">=", 1},
	}
	for _, c := range cases {
		got, err := ParseCountAssertion(c.in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if got.Selector != c.sel || got.Op != c.op || got.N != c.n {
			t.Errorf("%q: got (%q %s %d), want (%q %s %d)", c.in, got.Selector, got.Op, got.N, c.sel, c.op, c.n)
		}
	}
	for _, bad := range []string{"", "   ", ">=1"} {
		if _, err := ParseCountAssertion(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestCountAssertionHolds(t *testing.T) {
	ge2, _ := ParseCountAssertion("li>=2")
	if !ge2.Holds(2) || ge2.Holds(1) {
		t.Error(">=2 wrong")
	}
	eq0, _ := ParseCountAssertion("li==0")
	if !eq0.Holds(0) || eq0.Holds(1) {
		t.Error("==0 wrong")
	}
	// -1 is the probe's "the selector did not evaluate"; it must never pass,
	// not even `<5`.
	lt5, _ := ParseCountAssertion("li<5")
	if lt5.Holds(-1) {
		t.Error("an invalid selector satisfied <5")
	}
}

func healthy() PageSignals {
	return PageSignals{Title: "Items", Headings: []string{"Item overview"}, TextLen: 200, Rows: 2, Status: 200}
}

func TestEvaluatePage_HealthyPagePasses(t *testing.T) {
	if f := EvaluatePage(healthy(), CheckSpec{}); len(f) != 0 {
		t.Errorf("a healthy page failed: %v", f)
	}
}

// THE exit-code case from the brief: a page showing Mendix's "An error
// occurred" dialog must fail, or the check is no better than a screenshot.
func TestEvaluatePage_ErrorBannerFails(t *testing.T) {
	s := healthy()
	s.ErrorAlerts = []string{"An error occurred, please contact your system administrator."}
	s.Alerts = s.ErrorAlerts
	f := EvaluatePage(s, CheckSpec{})
	if len(f) == 0 || !strings.Contains(strings.Join(f, "|"), "error banner") {
		t.Errorf("an error banner did not fail the page: %v", f)
	}
}

// A warning or a validation message is shown in the verdict but is not a
// broken page.
func TestEvaluatePage_WarningDoesNotFail(t *testing.T) {
	s := healthy()
	s.Alerts = []string{"Please fill in a name"}
	if f := EvaluatePage(s, CheckSpec{}); len(f) != 0 {
		t.Errorf("a warning failed the page: %v", f)
	}
}

func TestEvaluatePage_ConsoleErrorFailsUnlessAllowed(t *testing.T) {
	s := healthy()
	s.ConsoleErrors = []string{"TypeError: x is undefined"}
	if f := EvaluatePage(s, CheckSpec{}); len(f) != 1 {
		t.Errorf("console error: got %v, want one failure", f)
	}
	if f := EvaluatePage(s, CheckSpec{AllowConsoleErrors: true}); len(f) != 0 {
		t.Errorf("--allow-console-errors still failed: %v", f)
	}
}

func TestEvaluatePage_HTTPErrors(t *testing.T) {
	s := healthy()
	s.Status = 404
	if f := EvaluatePage(s, CheckSpec{}); len(f) != 1 || f[0] != "HTTP 404" {
		t.Errorf("page 404: got %v", f)
	}
	s = healthy()
	s.HTTPErrors = []string{"560 POST /xas/"}
	if f := EvaluatePage(s, CheckSpec{}); len(f) != 1 {
		t.Errorf("failed xas call: got %v", f)
	}
}

// On the sign-in form the client's 401s are the symptom of not being signed
// in; the verdict must give the one reason, not three.
func TestEvaluatePage_SignInFormIsOneReason(t *testing.T) {
	s := PageSignals{Title: "Login", TextLen: 30, LoginForm: true,
		ConsoleErrors: []string{"[Startup] Could not create a session because the server responded with 401 (Unauthorized)."},
		HTTPErrors:    []string{"401 POST /xas/"}}
	f := EvaluatePage(s, CheckSpec{})
	if len(f) != 1 || !strings.Contains(f[0], "sign-in form") {
		t.Errorf("got %v, want the single sign-in failure", f)
	}
}

// A session the runtime has forgotten shows no form at all — just a 401 and a
// blank page. It must still read as "not signed in".
func TestEvaluatePage_Bare401IsNotSignedIn(t *testing.T) {
	s := PageSignals{TextLen: 0, HTTPErrors: []string{"401 POST /xas/"}}
	if !s.NeedsLogin() {
		t.Fatal("a 401 from the runtime is not recognised as needing a login")
	}
	f := EvaluatePage(s, CheckSpec{})
	if len(f) != 1 || !strings.Contains(f[0], "not signed in") {
		t.Errorf("got %v", f)
	}
}

func TestEvaluatePage_Assertions(t *testing.T) {
	ge1, _ := ParseCountAssertion(".mx-listview li>=1")
	eq1, _ := ParseCountAssertion(".mx-name-btnSeed==1")
	spec := CheckSpec{Texts: []string{"Item overview", "Gamma"}, Counts: []CountAssertion{ge1, eq1}}
	s := healthy()
	s.TextFound = []bool{true, false}
	s.Counts = []int{0, 1}
	f := EvaluatePage(s, spec)
	joined := strings.Join(f, "|")
	if len(f) != 2 || !strings.Contains(joined, `"Gamma" not found`) || !strings.Contains(joined, "got 0") {
		t.Errorf("got %v", f)
	}
	// Missing answers (the probe did not run the assertion) fail, never pass.
	if f := EvaluatePage(healthy(), spec); len(f) != 4 {
		t.Errorf("unanswered assertions: got %v, want 4 failures", f)
	}
}

func TestFormatCheckResult_PrintsPathNotImage(t *testing.T) {
	s := healthy()
	s.Screenshot = "/tmp/items.png"
	got := formatCheckResult("/p/items", s, []string{"assert-text \"x\" not found"})
	for _, want := range []string{"page /p/items", "FAIL   assert-text", "PNG    /tmp/items.png"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if len(got) > 400 {
		t.Errorf("result is %d bytes; it must stay a few lines:\n%s", len(got), got)
	}
}

// fakeBrowser stands in for the node probe and the login script.
type fakeBrowser struct {
	probes  [][]PageTarget
	storage []string
	logins  int
	answer  func(call int, p PageTarget) PageSignals
}

func (f *fakeBrowser) install(t *testing.T) {
	t.Helper()
	oldProbe, oldLogin := probePagesFn, loginFn
	t.Cleanup(func() { probePagesFn, loginFn = oldProbe, oldLogin })
	probePagesFn = func(o ProbeOptions) ([]PageSignals, error) {
		call := len(f.probes)
		f.probes = append(f.probes, o.Pages)
		f.storage = append(f.storage, o.Storage)
		out := make([]PageSignals, len(o.Pages))
		for i, p := range o.Pages {
			out[i] = f.answer(call, p)
		}
		return out, nil
	}
	loginFn = func(o LoginOptions) error {
		f.logins++
		return os.WriteFile(o.StoragePath, []byte(`{"cookies":[]}`), 0o600)
	}
}

func TestRunPageCheck_CountsFailedPages(t *testing.T) {
	fb := &fakeBrowser{answer: func(_ int, p PageTarget) PageSignals {
		if strings.HasSuffix(p.URL, "/p/broken") {
			s := healthy()
			s.ErrorAlerts = []string{"An error occurred"}
			return s
		}
		return healthy()
	}}
	fb.install(t)
	var out bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{
		BaseURL: "http://localhost:8080", Targets: []string{"/p/items", "/p/broken"}, Stdout: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 2 || res.Failed != 1 {
		t.Errorf("got %+v, want 2 pages / 1 failed\n%s", res, out.String())
	}
	if len(fb.probes) != 1 || len(fb.probes[0]) != 2 {
		t.Errorf("want ONE browser run for both pages, got %d runs", len(fb.probes))
	}
	if !strings.Contains(out.String(), "FAIL 1 of 2 page(s)") {
		t.Errorf("summary line missing:\n%s", out.String())
	}
	if fb.probes[0][1].URL != "http://localhost:8080/p/broken" {
		t.Errorf("target not resolved against the base URL: %q", fb.probes[0][1].URL)
	}
}

// The saved session is reused: a second check must not sign in again (each
// sign-in takes one of the unlicensed runtime's few session slots).
func TestRunPageCheck_ReusesSavedSession(t *testing.T) {
	fb := &fakeBrowser{answer: func(int, PageTarget) PageSignals { return healthy() }}
	fb.install(t)
	dir := t.TempDir()
	opts := PageCheckOptions{BaseURL: "http://localhost:8080", User: "demo_user", Password: "x",
		StorageDir: dir, Stdout: &bytes.Buffer{}}
	for i := 0; i < 2; i++ {
		if _, err := RunPageCheck(opts); err != nil {
			t.Fatal(err)
		}
	}
	if fb.logins != 1 {
		t.Errorf("signed in %d times over two checks, want 1", fb.logins)
	}
	if fb.storage[1] == "" || filepath.Dir(fb.storage[1]) != dir {
		t.Errorf("second check did not load the saved session: %q", fb.storage[1])
	}
	// --fresh-login is the explicit override.
	opts.FreshLogin = true
	if _, err := RunPageCheck(opts); err != nil {
		t.Fatal(err)
	}
	if fb.logins != 2 {
		t.Errorf("--fresh-login did not sign in again (logins=%d)", fb.logins)
	}
}

// A saved session the runtime has forgotten (it restarted) is renewed once,
// and only the pages it spoiled are loaded again.
func TestRunPageCheck_RenewsAStaleSession(t *testing.T) {
	fb := &fakeBrowser{answer: func(call int, p PageTarget) PageSignals {
		if call == 0 && strings.HasSuffix(p.URL, "/p/items") {
			return PageSignals{HTTPErrors: []string{"401 POST /xas/"}}
		}
		return healthy()
	}}
	fb.install(t)
	dir := t.TempDir()
	opts := PageCheckOptions{BaseURL: "http://localhost:8080", User: "demo_user", Password: "x",
		StorageDir: dir, Targets: []string{"/p/items", "/"}, Stdout: &bytes.Buffer{}}
	if err := os.WriteFile(checkStoragePath(opts, "demo_user"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := RunPageCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if fb.logins != 1 {
		t.Errorf("stale session: logins=%d, want 1", fb.logins)
	}
	if len(fb.probes) != 2 || len(fb.probes[1]) != 1 || !strings.HasSuffix(fb.probes[1][0].URL, "/p/items") {
		t.Errorf("want a second run of just /p/items, got %v", fb.probes)
	}
	if res.Failed != 0 {
		t.Errorf("after renewal every page should pass, got %+v", res)
	}
}

func TestRunPageCheck_ScreenshotPerPage(t *testing.T) {
	fb := &fakeBrowser{answer: func(_ int, p PageTarget) PageSignals {
		s := healthy()
		s.Screenshot = p.Screenshot
		return s
	}}
	fb.install(t)
	dir := t.TempDir()
	var out bytes.Buffer
	if _, err := RunPageCheck(PageCheckOptions{BaseURL: "http://localhost:8080",
		Targets: []string{"/p/a", "/p/b"}, Screenshot: filepath.Join(dir, "s.png"), Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	a, b := fb.probes[0][0].Screenshot, fb.probes[0][1].Screenshot
	if a == b || !strings.HasSuffix(a, "s-p-a.png") {
		t.Errorf("per-page screenshot paths: %q %q", a, b)
	}
	if !strings.Contains(out.String(), "PNG    "+a) {
		t.Errorf("path not printed:\n%s", out.String())
	}
}

func TestPickDemoUser(t *testing.T) {
	ps := &security.ProjectSecurity{
		SecurityLevel: "CheckEverything", EnableDemoUsers: true,
		DemoUsers: []*security.DemoUser{
			{UserName: "demo_user", Password: "u", UserRoles: []string{"User"}},
			{UserName: "demo_administrator", Password: "a", UserRoles: []string{"Administrator"}},
		},
	}
	c, err := pickDemoUser(ps, "user")
	if err != nil || c.user != "demo_user" || c.password != "u" {
		t.Errorf("--role user: %+v %v", c, err)
	}
	// Without a role, an administrator is preferred.
	c, _ = pickDemoUser(ps, "")
	if c.user != "demo_administrator" {
		t.Errorf("fallback picked %q, want the administrator", c.user)
	}
	if _, err := pickDemoUser(ps, "Auditor"); err == nil || !strings.Contains(err.Error(), "demo_user") {
		t.Errorf("unknown role should list the demo users, got %v", err)
	}
	// Demo users switched off: no implicit credentials.
	ps.EnableDemoUsers = false
	if c, _ := pickDemoUser(ps, ""); c.user != "" {
		t.Errorf("demo users disabled, still picked %q", c.user)
	}
}

func TestCheckStoragePathIsPerOriginAndUser(t *testing.T) {
	a := checkStoragePath(PageCheckOptions{BaseURL: "http://localhost:8080", ProjectPath: "/x/app.mpr"}, "demo_user")
	b := checkStoragePath(PageCheckOptions{BaseURL: "http://localhost:9090", ProjectPath: "/x/app.mpr"}, "demo_user")
	c := checkStoragePath(PageCheckOptions{BaseURL: "http://localhost:8080", ProjectPath: "/x/app.mpr"}, "demo_administrator")
	if a == b || a == c {
		t.Errorf("sessions collide: %s %s %s", a, b, c)
	}
	if filepath.Dir(a) != "/x/.mxcli/playwright-check" {
		t.Errorf("not under the project's .mxcli: %s", a)
	}
}
