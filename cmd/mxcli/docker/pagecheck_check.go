// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/security"
)

// pagecheck_check.go is `mxcli playwright check`: the text verdict of
// `run --page-check`, against an app that is already running, for several
// pages in one call, logging in when the page needs it.
//
// It exists because agents kept hand-rolling the same playwright-cli sequence —
// open the login page, fill two fields, click, goto, sleep, eval, screenshot,
// read the PNG — about a sixth of the tool calls in one measured session. Every
// step of that is mechanical, so it belongs in the tool.

// Seams for tests: the real ones drive a browser.
var (
	probePagesFn = ProbePages
	loginFn      = LoginAndSaveStorage
	signOutFn    = SignOut
)

// PageCheckOptions configures RunPageCheck.
type PageCheckOptions struct {
	BaseURL     string   // app root, e.g. http://localhost:8080
	Targets     []string // paths relative to BaseURL, or full URLs; empty = the root
	ProjectPath string   // optional; enables --role and the demo-user fallback

	User     string
	Password string
	Role     string // log in as the project's demo user holding this user role

	// FreshLogin ignores a saved session and signs in again. The session it
	// replaces is signed out first, so it does not hold a slot until it expires.
	FreshLogin bool
	// SignOut ends the run signed out: the session it used is signed out and
	// not kept for the next check.
	SignOut bool
	// StorageDir holds the saved sessions (default <projectDir>/.mxcli/playwright-check,
	// or ~/.mxcli/playwright-check without a project).
	StorageDir string
	// RuntimeLogPath is read when a sign-in is refused, to name the cause the
	// sign-in page does not (the unlicensed runtime's session cap). Default
	// <projectDir>/.mxcli/runtime.log, where `run --local` writes it.
	RuntimeLogPath string

	Spec       CheckSpec
	Screenshot string // PNG path; per-page suffix when there are several pages
	WaitMs     int

	Stdout io.Writer
	Stderr io.Writer
}

// PageCheckResult summarizes a run.
type PageCheckResult struct {
	Pages  int
	Failed int
}

// credentials is who to sign in as, and why.
type credentials struct {
	user, password string
	source         string // "", "--role Administrator", "demo user", ...
}

// RunPageCheck checks every target and prints one verdict per page. The error
// is reserved for "could not check" (no browser, login rejected); a page that
// checked and failed is counted in the result, not returned as an error.
func RunPageCheck(opts PageCheckOptions) (PageCheckResult, error) {
	var res PageCheckResult
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:8080"
	}
	if opts.RuntimeLogPath == "" && opts.ProjectPath != "" {
		opts.RuntimeLogPath = filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", "runtime.log")
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []string{""}
	}

	creds, err := resolveCheckCredentials(opts)
	if err != nil {
		return res, err
	}

	if opts.Screenshot != "" {
		if abs, err := filepath.Abs(opts.Screenshot); err == nil {
			opts.Screenshot = abs
		}
	}
	pages := make([]PageTarget, len(targets))
	for i, t := range targets {
		pages[i] = PageTarget{URL: resolveScreenshotURL(opts.BaseURL, t)}
		if opts.Screenshot != "" {
			pages[i].Screenshot = opts.Screenshot
			if len(targets) > 1 {
				pages[i].Screenshot = screenshotOutName(opts.Screenshot, t)
			}
			if err := os.MkdirAll(filepath.Dir(pages[i].Screenshot), 0o755); err != nil {
				return res, err
			}
		}
	}
	// A session is a set of cookies, and a cookie belongs to the host it was
	// set on: one signed in on localhost is not sent to 127.0.0.1 or to
	// frontend.local, though all three reach the same app. Signing in on the
	// base URL and then loading a page given as a full URL on another host
	// showed the sign-in form on every run, and signed in again on each
	// (ako/mxcli#1216). So the pages of each host get a session signed in on
	// that host.
	mxbuild := anyMxBuildPath()
	sigs := make([]PageSignals, len(pages))
	for _, g := range groupByOrigin(opts.BaseURL, pages) {
		o := opts
		o.BaseURL = g.appURL
		got, err := checkOrigin(o, creds, g.pages(pages), mxbuild, g.appURL != opts.BaseURL, w)
		if err != nil {
			return res, err
		}
		for j, i := range g.idx {
			sigs[i] = got[j]
		}
	}

	for i, s := range sigs {
		fails := EvaluatePage(s, opts.Spec)
		if s.NeedsLogin() && creds.user == "" {
			fails = replaceLoginHint(fails, opts.ProjectPath)
		}
		fmt.Fprint(w, formatCheckResult(pageLabel(targets[i]), s, fails))
		res.Pages++
		if len(fails) > 0 {
			res.Failed++
		}
	}
	if res.Failed > 0 {
		fmt.Fprintf(w, "FAIL %d of %d page(s)\n", res.Failed, res.Pages)
	} else {
		fmt.Fprintf(w, "OK %d page(s)\n", res.Pages)
	}
	return res, nil
}

// originGroup is the pages of one origin, as indexes into the request.
type originGroup struct {
	appURL string // where to sign in: the base URL, or the pages' own origin
	idx    []int
}

func (g originGroup) pages(all []PageTarget) []PageTarget {
	out := make([]PageTarget, len(g.idx))
	for j, i := range g.idx {
		out[j] = all[i]
	}
	return out
}

// groupByOrigin splits the pages by scheme, host and port, in the order each
// origin first appears. Pages on the base URL's origin sign in on the base URL
// itself; the others on their own origin.
func groupByOrigin(baseURL string, pages []PageTarget) []originGroup {
	baseOrigin := originOf(baseURL)
	var groups []originGroup
	at := map[string]int{}
	for i, p := range pages {
		o := originOf(p.URL)
		k, ok := at[o]
		if !ok {
			app := o
			if o == baseOrigin || o == "" {
				app = baseURL
			}
			k = len(groups)
			at[o] = k
			groups = append(groups, originGroup{appURL: app})
		}
		groups[k].idx = append(groups[k].idx, i)
	}
	return groups
}

// originOf is scheme://host[:port], lower-cased; "" when raw is not an
// absolute URL.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// checkOrigin probes pages that share one origin, opts.BaseURL, signing in on
// that origin when the check has credentials. nameHost adds the host to the
// login/logout lines, for an origin other than the one the user gave.
func checkOrigin(opts PageCheckOptions, creds credentials, pages []PageTarget, mxbuild string, nameHost bool, w io.Writer) ([]PageSignals, error) {
	probe := ProbeOptions{
		Pages: pages, WaitMs: opts.WaitMs,
		Texts: opts.Spec.Texts, CountSelectors: opts.Spec.CountSelectors(),
		MxBuildPath: mxbuild,
	}
	if creds.user == "" {
		return probePagesFn(probe)
	}

	storage := checkStoragePath(opts, creds.user)
	where := ""
	if nameHost {
		if u, err := url.Parse(opts.BaseURL); err == nil {
			where = " on " + u.Host
		}
	}
	signOut := func() {
		// Best effort, and never part of the verdict: a session that could not
		// be signed out holds its slot only until the runtime expires it.
		err := signOutFn(SignOutOptions{AppURL: opts.BaseURL, StoragePath: storage, MxBuildPath: mxbuild})
		if errors.Is(err, ErrNoSession) {
			return // already gone: nothing held a slot
		}
		if err != nil {
			fmt.Fprintf(opts.Stderr, "sign-out of %s%s did not complete (the session expires on its own): %v\n", creds.user, where, err)
			return
		}
		fmt.Fprintf(w, "logout %s%s\n", creds.user, where)
	}
	if opts.FreshLogin {
		if _, err := os.Stat(storage); err == nil {
			signOut()
		}
		_ = os.Remove(storage)
	}
	login := func() error {
		fmt.Fprintf(w, "login %s%s%s\n", creds.user, where, sourceSuffix(creds.source))
		return loginFn(LoginOptions{
			AppURL: opts.BaseURL, Username: creds.user, Password: creds.password,
			StoragePath: storage, MxBuildPath: mxbuild, RuntimeLogPath: opts.RuntimeLogPath,
		})
	}

	// A saved session is reused as-is: signing in again costs a browser round
	// trip and, on the unlicensed runtime, one of a handful of session slots.
	// Reused, it stays one session however many checks run.
	loggedIn := false
	if _, err := os.Stat(storage); err != nil {
		if err := login(); err != nil {
			return nil, err
		}
		loggedIn = true
	}
	probe.Storage = storage

	sigs, err := probePagesFn(probe)
	if err != nil {
		return nil, err
	}

	// A saved session the runtime no longer knows (it restarted, or the session
	// timed out) lands every page on the sign-in form. Sign in once and redo
	// just those pages.
	if !loggedIn && anyNeedsLogin(sigs) {
		if err := login(); err != nil {
			return nil, err
		}
		loggedIn = true
		var redo []int
		retry := probe
		retry.Pages = nil
		for i, s := range sigs {
			if s.NeedsLogin() {
				redo = append(redo, i)
				retry.Pages = append(retry.Pages, pages[i])
			}
		}
		again, err := probePagesFn(retry)
		if err != nil {
			return nil, err
		}
		for j, i := range redo {
			sigs[i] = again[j]
		}
	}

	// A session this run signed in that got no page past the sign-in form is
	// worth nothing to the next check, which would only sign in again beside
	// it — the per-run leak of #1216. Sign it out instead of keeping it; and
	// sign out whatever session was used when the run is to end signed out.
	if opts.SignOut || (loggedIn && allNeedLogin(sigs)) {
		signOut()
		_ = os.Remove(storage)
	}
	return sigs, nil
}

func anyNeedsLogin(sigs []PageSignals) bool {
	for _, s := range sigs {
		if s.NeedsLogin() {
			return true
		}
	}
	return false
}

func allNeedLogin(sigs []PageSignals) bool {
	for _, s := range sigs {
		if !s.NeedsLogin() {
			return false
		}
	}
	return len(sigs) > 0
}

func sourceSuffix(src string) string {
	if src == "" {
		return ""
	}
	return " (" + src + ")"
}

// replaceLoginHint makes the "sign-in form" failure name the fix that applies:
// without a project, --role cannot be resolved.
func replaceLoginHint(fails []string, projectPath string) []string {
	if projectPath != "" {
		return fails
	}
	for i, f := range fails {
		if strings.HasSuffix(f, "(pass --user/--password or --role)") {
			fails[i] = strings.TrimSuffix(f, "(pass --user/--password or --role)") + "(pass --user/--password, or -p app.mpr --role R)"
		}
	}
	return fails
}

// resolveCheckCredentials picks who to sign in as: --user wins; --role picks
// the project's demo user holding that role; with only -p, the project's demo
// users are the fallback (an administrator first), so a check against a
// secured app needs no credentials on the command line at all.
func resolveCheckCredentials(opts PageCheckOptions) (credentials, error) {
	if opts.User != "" {
		return credentials{user: opts.User, password: opts.Password}, nil
	}
	if opts.ProjectPath == "" {
		if opts.Role != "" {
			return credentials{}, fmt.Errorf("--role needs -p app.mpr to look up the demo users")
		}
		return credentials{}, nil
	}
	ps, err := readProjectSecurity(opts.ProjectPath)
	if err != nil {
		if opts.Role != "" {
			return credentials{}, err
		}
		return credentials{}, nil // best effort: no fallback without the model
	}
	return pickDemoUser(ps, opts.Role)
}

func readProjectSecurity(projectPath string) (*security.ProjectSecurity, error) {
	reader, err := openReadOnly(projectPath)
	if err != nil {
		return nil, fmt.Errorf("opening project: %w", err)
	}
	defer reader.Disconnect()
	ps, err := reader.GetProjectSecurity()
	if err != nil {
		return nil, fmt.Errorf("reading project security: %w", err)
	}
	return ps, nil
}

// pickDemoUser chooses a demo user. With a role, the user must hold it; without
// one, an administrator-looking role is preferred, then the first user. A
// project with security off or no demo users yields no credentials.
func pickDemoUser(ps *security.ProjectSecurity, role string) (credentials, error) {
	if ps == nil {
		return credentials{}, nil
	}
	if role != "" {
		var names []string
		for _, du := range ps.DemoUsers {
			for _, r := range du.UserRoles {
				if strings.EqualFold(shortRole(r), role) || strings.EqualFold(r, role) {
					return credentials{user: du.UserName, password: du.Password, source: "--role " + role}, nil
				}
			}
			names = append(names, du.UserName)
		}
		if len(ps.DemoUsers) == 0 {
			return credentials{}, fmt.Errorf("--role %s: the project has no demo users", role)
		}
		return credentials{}, fmt.Errorf("--role %s: no demo user holds that role (demo users: %s)", role, strings.Join(names, ", "))
	}
	if strings.EqualFold(ps.SecurityLevel, "off") || strings.EqualFold(ps.SecurityLevel, "CheckNothing") ||
		!ps.EnableDemoUsers || len(ps.DemoUsers) == 0 {
		return credentials{}, nil
	}
	admin := regexp.MustCompile(`(?i)admin`)
	for _, du := range ps.DemoUsers {
		for _, r := range du.UserRoles {
			if admin.MatchString(r) {
				return credentials{user: du.UserName, password: du.Password, source: "demo user"}, nil
			}
		}
	}
	du := ps.DemoUsers[0]
	return credentials{user: du.UserName, password: du.Password, source: "demo user"}, nil
}

// shortRole strips a module or project qualifier from a role name.
func shortRole(r string) string {
	if i := strings.LastIndex(r, "."); i >= 0 {
		return r[i+1:]
	}
	return r
}

// checkStoragePath is where a user's session is kept between checks, keyed by
// app origin and user so a second app or user never reuses the wrong one.
func checkStoragePath(opts PageCheckOptions, user string) string {
	dir := opts.StorageDir
	if dir == "" {
		if opts.ProjectPath != "" {
			dir = filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", "playwright-check")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".mxcli", "playwright-check")
		} else {
			dir = filepath.Join(os.TempDir(), "mxcli-playwright-check")
		}
	}
	host := opts.BaseURL
	if u, err := url.Parse(opts.BaseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return filepath.Join(dir, slugify(host)+"-"+slugify(user)+".json")
}

var nonSlug = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func slugify(s string) string {
	s = nonSlug.ReplaceAllString(s, "_")
	if s == "" {
		return "_"
	}
	return s
}

// anyMxBuildPath returns a cached MxBuild, whose bundled node serves when no
// node is on PATH. Empty when there is none.
func anyMxBuildPath() string {
	p, err := resolveMxBuild("")
	if err != nil {
		return ""
	}
	return p
}
