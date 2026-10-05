// SPDX-License-Identifier: Apache-2.0

package docker

import (
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
)

// PageCheckOptions configures RunPageCheck.
type PageCheckOptions struct {
	BaseURL     string   // app root, e.g. http://localhost:8080
	Targets     []string // paths relative to BaseURL, or full URLs; empty = the root
	ProjectPath string   // optional; enables --role and the demo-user fallback

	User     string
	Password string
	Role     string // log in as the project's demo user holding this user role

	// FreshLogin ignores a saved session and signs in again.
	FreshLogin bool
	// StorageDir holds the saved sessions (default <projectDir>/.mxcli/playwright-check,
	// or ~/.mxcli/playwright-check without a project).
	StorageDir string

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
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:8080"
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []string{""}
	}

	creds, err := resolveCheckCredentials(opts)
	if err != nil {
		return res, err
	}
	storage := ""
	if creds.user != "" {
		storage = checkStoragePath(opts, creds.user)
		if opts.FreshLogin {
			_ = os.Remove(storage)
		}
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
	probe := ProbeOptions{
		Pages: pages, WaitMs: opts.WaitMs,
		Texts: opts.Spec.Texts, CountSelectors: opts.Spec.CountSelectors(),
		MxBuildPath: anyMxBuildPath(),
	}

	login := func() error {
		fmt.Fprintf(w, "login %s%s\n", creds.user, sourceSuffix(creds.source))
		return loginFn(LoginOptions{
			AppURL: opts.BaseURL, Username: creds.user, Password: creds.password,
			StoragePath: storage, MxBuildPath: probe.MxBuildPath,
		})
	}

	// A saved session is reused as-is: signing in again costs a browser round
	// trip and, on the unlicensed runtime, one of a handful of session slots.
	loggedIn := false
	if storage != "" {
		if _, err := os.Stat(storage); err != nil {
			if err := login(); err != nil {
				return res, err
			}
			loggedIn = true
		}
		probe.Storage = storage
	}

	sigs, err := probePagesFn(probe)
	if err != nil {
		return res, err
	}

	// A saved session the runtime no longer knows (it restarted, or the session
	// timed out) lands every page on the sign-in form. Sign in once and redo
	// just those pages.
	if storage != "" && !loggedIn && anyNeedsLogin(sigs) {
		if err := login(); err != nil {
			return res, err
		}
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
			return res, err
		}
		for j, i := range redo {
			sigs[i] = again[j]
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

func anyNeedsLogin(sigs []PageSignals) bool {
	for _, s := range sigs {
		if s.NeedsLogin() {
			return true
		}
	}
	return false
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
