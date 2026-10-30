// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pagecheck.go answers, in text, the question a screenshot is usually taken to
// answer: did this page render, and is anything wrong with it.
//
// A PNG costs roughly 1,500 tokens to read and a verdict about 100 — and the
// PNG's cost is not a one-off, because an image read into a conversation is
// re-charged on every later model call (ako/mxcli#614, and
// docs/11-proposals/PROPOSAL_agent_loop_efficiency.md). The picture is also
// strictly *less* informative for this question: a console error, which is the
// usual cause of a page that renders blank, does not appear in one.
//
// It uses the Playwright already required by --screenshot, driven through node
// rather than the CLI, because the Playwright CLI has no text-dump subcommand.
// `mxcli run --page-check` and `mxcli playwright check` share it, so the two
// verdicts cannot drift apart.

// PageSignals is what one page load tells us.
type PageSignals struct {
	Title    string   `json:"title"`
	Headings []string `json:"headings"`
	// Alerts are the visible Mendix banners — danger, warning, validation.
	Alerts []string `json:"alerts"`
	// ErrorAlerts is the subset that means "something broke": .alert-danger
	// banners and Mendix error dialogs. A warning or a validation message is
	// shown, but does not fail a check.
	ErrorAlerts   []string `json:"errorAlerts"`
	TextLen       int      `json:"textLen"`       // body innerText length; 0 is the blank-page tell
	Rows          int      `json:"rows"`          // grid / list rows rendered
	ConsoleErrors []string `json:"consoleErrors"` // what a screenshot cannot show

	// RequestedURL is the URL the page was loaded from, set by ProbePages so a
	// verdict can tell when FinalURL is somewhere else.
	RequestedURL string `json:"requestedUrl"`
	// FinalURL is where the browser ended up — a redirect to the login page
	// shows here, not in the title.
	FinalURL string `json:"finalUrl"`
	// Status is the HTTP status of the navigation itself (0 when unknown).
	Status int `json:"status"`
	// HTTPErrors are same-origin requests answered with >= 400 after the page
	// started loading — a failing xas/ call is how a broken data source shows.
	HTTPErrors []string `json:"httpErrors"`
	// LoginForm reports that the Mendix sign-in form is on screen: the page
	// asked for was not rendered, whatever the title says.
	LoginForm bool `json:"loginForm"`
	// SignedInAs is who the page was loaded as — "demo_user (user role User)" —
	// set by `playwright check` when it signed in; empty for an anonymous load.
	// It is not measured, only used to word a page the user was refused.
	SignedInAs string `json:"-"`
	// TextFound and Counts answer the probe's text / count queries, index-aligned
	// with ProbeOptions.Texts and ProbeOptions.CountSelectors.
	TextFound []bool `json:"textFound"`
	Counts    []int  `json:"counts"`
	// Screenshot is the PNG written for this page, when one was asked for.
	Screenshot string `json:"screenshot"`

	// Unsettled reports that the page was still loading when the wait ran out:
	// a same-origin request was still open, or the text was still changing.
	// Everything else then describes a partly rendered page, so a check fails
	// on it rather than vouching for what it happened to catch (ako/mxcli#845).
	Unsettled bool `json:"unsettled"`
	// PendingRequests are the same-origin requests still open at that point,
	// as "METHOD /path" (at most three).
	PendingRequests []string `json:"pendingRequests"`
	// WaitedMs is how long the probe waited for the page to settle once it had
	// mounted.
	WaitedMs int `json:"waitedMs"`
	// DataWidgets counts the list views, data grids and galleries on the page.
	// With one present the verdict always reports rows — rows=0 included — so
	// an empty list does not read the same as a page that has none.
	DataWidgets int `json:"dataWidgets"`
}

// pageProbeJS runs in node with Playwright. It reports signals and never
// throws for a bad page — a page that fails to render is the thing being
// measured, not an error in measuring it.
//
// One browser serves every page in the request, so checking five pages costs
// one Chromium launch, not five.
const pageProbeJS = `
let pw;
try { pw = require('playwright'); } catch (e) { pw = require('playwright-core'); }
const { chromium } = pw;
const req = JSON.parse(require('fs').readFileSync(process.argv[2], 'utf8'));
(async () => {
  const launch = req.executablePath ? { executablePath: req.executablePath } : {};
  const b = await chromium.launch(launch);
  const ctxOpts = req.storage ? { storageState: req.storage } : {};
  const ctx = await b.newContext(ctxOpts);
  const results = [];
  for (const target of req.pages) {
    const p = await ctx.newPage();
    const consoleErrors = [];
    const httpErrors = [];
    const origin = (() => { try { return new URL(target.url).origin; } catch (e) { return ''; } })();
    p.on('console', m => {
      if (m.type() !== 'error') return;
      // Chromium echoes every failed load as a console error; a same-origin
      // one is already reported, with its method and path, as an HTTP error.
      const loc = (m.location() || {}).url || '';
      if (/^Failed to load resource/.test(m.text()) && loc.startsWith(origin)) return;
      consoleErrors.push(m.text());
    });
    p.on('pageerror', e => consoleErrors.push(String(e && e.message || e)));
    p.on('response', r => {
      try {
        const u = new URL(r.url());
        // The page's own status is reported separately (http=); this is for
        // what the page asked for afterwards.
        if (r.status() >= 400 && u.origin === origin && !/favicon/.test(u.pathname) &&
            r.request().resourceType() !== 'document') {
          httpErrors.push(r.status() + ' ' + r.request().method() + ' ' + u.pathname);
        }
      } catch (e) {}
    });
    // Same-origin requests still open. A data source is fetched after the page
    // mounts, and while it is out a list view shows nothing that says so:
    // measured on 11.13, .mx-listview sits empty — no progress element, no
    // aria-busy, no "no items" placeholder — for as long as its microflow runs,
    // and the text holds still. The open POST /xas/ is the only sign the page
    // is not finished (ako/mxcli#845).
    const inflight = new Map();
    let lastNet = Date.now();
    const pathOf = u => { try { return new URL(u).pathname; } catch (e) { return u; } };
    const sameOrigin = u => { try { return new URL(u).origin === origin; } catch (e) { return false; } };
    p.on('request', r => {
      // A redirect is one request as far as the page is concerned.
      if (r.redirectedFrom()) inflight.delete(r.redirectedFrom());
      if (!sameOrigin(r.url())) return;
      inflight.set(r, r.method() + ' ' + pathOf(r.url()));
      lastNet = Date.now();
    });
    const finished = r => { if (inflight.delete(r)) lastNet = Date.now(); };
    p.on('requestfinished', finished);
    p.on('requestfailed', finished);
    const out = { title: '', headings: [], alerts: [], errorAlerts: [], textLen: 0, rows: 0,
      consoleErrors, finalUrl: '', status: 0, httpErrors, loginForm: false,
      textFound: [], counts: [], screenshot: '',
      unsettled: false, pendingRequests: [], waitedMs: 0, dataWidgets: 0 };
    try {
      const resp = await p.goto(target.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
      out.status = resp ? resp.status() : 0;
      // Wait for the Mendix client to mount a page (or the login form), then
      // for the page to settle: no same-origin request open for 500ms, no
      // progress indicator, and the body text unchanged over three polls. Data
      // sources load after the page shell, so a fixed sleep is either too short
      // or wasted — and the text alone holds still while a list view's data is
      // out, which is how a verdict used to be taken before the rows arrived.
      // The sign-in form is the exception to the request rule: the client keeps
      // a POST /xas/ open behind it (measured on 11.13), so waiting for quiet
      // served the whole --wait and then reported the form as still loading.
      // The form is a verdict in itself once it holds still.
      await p.waitForSelector('.mx-page, #usernameInput', { timeout: 20000 }).catch(() => {});
      const mounted = Date.now();
      const deadline = mounted + (req.waitMs || 15000);
      let last = -1, stable = 0, settled = false;
      while (Date.now() < deadline) {
        const st = await p.evaluate(() => ({
          len: (document.body && document.body.innerText || '').length,
          busy: document.querySelectorAll('.mx-progress, .mx-underlay-loading, [aria-busy="true"]').length,
          login: !!document.querySelector('#usernameInput'),
        })).catch(() => ({ len: -2, busy: 0, login: false }));
        const quiet = st.login || (inflight.size === 0 && Date.now() - lastNet >= 500);
        if (st.len === last && st.busy === 0 && quiet) {
          if (++stable >= 3) { settled = true; break; }
        } else { stable = 0; }
        last = st.len;
        await p.waitForTimeout(250);
      }
      out.unsettled = !settled;
      out.waitedMs = Date.now() - mounted;
      out.pendingRequests = [...inflight.values()].slice(0, 3);
      out.finalUrl = p.url();
      out.title = await p.title();
      out.loginForm = (await p.locator('#usernameInput').count()) > 0;
      // The demo-user switcher (development mode) has its own "Select user"
      // heading, which is not the page's.
      out.headings = await p.$$eval('h1,h2', ns => ns.filter(n => !n.closest('.mx-demouserswitcher'))
        .map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      // An information dialog is how the client explains a redirect it made
      // itself — "The page you requested was not found" for an unknown /p/ URL
      // (ako/mxcli#998) — so it is shown, though it fails nothing.
      out.alerts = await p.$$eval('.alert-danger,.alert-warning,.mx-validation-message,.mx-dialog-info .mx-dialog-body',
        ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      out.errorAlerts = await p.$$eval('.alert-danger,.mx-dialog-error .mx-dialog-body',
        ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      const body = (await p.$eval('body', n => n.innerText || '')).trim();
      out.textLen = body.length;
      // Data rows only: a list view's "No items found" placeholder is an <li>
      // too, and a data grid 2 header row is a role=row without grid cells.
      out.rows = await p.$$eval('.mx-datagrid tbody tr, .mx-listview > ul > li:not(.mx-listview-empty), [role="row"]:has([role="gridcell"])', ns => ns.length);
      out.dataWidgets = await p.$$eval('.mx-listview, .mx-datagrid, .widget-datagrid, .widget-gallery', ns => ns.length);
      out.textFound = (req.texts || []).map(t => body.includes(t));
      for (const sel of (req.countSelectors || [])) {
        out.counts.push(await p.locator(sel).count().catch(() => -1));
      }
      if (target.screenshot) {
        await p.screenshot({ path: target.screenshot, fullPage: true });
        out.screenshot = target.screenshot;
      }
    } catch (e) {
      out.consoleErrors.push('probe: ' + String(e && e.message || e).split('\n')[0]);
    }
    results.push(out);
    await p.close();
  }
  console.log(JSON.stringify(results));
  await b.close();
})().catch(e => { process.stderr.write(String(e && e.stack || e) + '\n'); process.exit(1); });
`

// PageTarget is one page to probe.
type PageTarget struct {
	URL        string `json:"url"`
	Screenshot string `json:"screenshot,omitempty"` // PNG path; empty = none
}

// ProbeOptions configures ProbePages.
type ProbeOptions struct {
	Pages   []PageTarget
	Storage string // Playwright storage-state file, for pages behind a login
	// WaitMs caps how long a page may keep loading and changing after it mounts
	// (default DefaultPageWaitMs). A page that settles sooner is measured then;
	// one that has not settled by the cap is reported as still loading.
	WaitMs int
	// Texts and CountSelectors are evaluated on every page; the answers come
	// back index-aligned in PageSignals.TextFound / Counts.
	Texts          []string
	CountSelectors []string
	Timeout        time.Duration // whole run; default 60s + (30s + WaitMs) per page
	MxBuildPath    string        // node fallback when none is on PATH
}

// DefaultPageWaitMs is how long a page may take to settle before the probe
// gives up and reports it as still loading. It is a cap, not a sleep: a page
// is measured as soon as it settles. It was 4s while the probe waited only for
// the text to stop changing; now that it also waits for the page's requests, a
// slow first data source (a cold runtime) is worth waiting for rather than
// reporting as unfinished.
const DefaultPageWaitMs = 15000

// probeRequest is the JSON handed to pageProbeJS.
type probeRequest struct {
	Pages          []PageTarget `json:"pages"`
	Storage        string       `json:"storage,omitempty"`
	WaitMs         int          `json:"waitMs"`
	Texts          []string     `json:"texts"`
	CountSelectors []string     `json:"countSelectors"`
	ExecutablePath string       `json:"executablePath,omitempty"`
}

// CheckPage loads url and returns its signals. storage is an optional Playwright
// storage-state file, for pages behind a login.
func CheckPage(url, storage string, waitMs int, timeout time.Duration) (PageSignals, error) {
	sigs, err := ProbePages(ProbeOptions{
		Pages: []PageTarget{{URL: url}}, Storage: storage, WaitMs: waitMs, Timeout: timeout,
	})
	if err != nil {
		return PageSignals{}, err
	}
	return sigs[0], nil
}

// ProbePages loads every page in one browser and returns their signals in order.
func ProbePages(opts ProbeOptions) ([]PageSignals, error) {
	if len(opts.Pages) == 0 {
		return nil, fmt.Errorf("no pages to check")
	}
	node := resolveNodeForScript(opts.MxBuildPath)
	if node == "" {
		return nil, fmt.Errorf("node not found; the page check needs the Node that Playwright uses")
	}
	dir, err := os.MkdirTemp("", "mxcli-pagecheck-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	waitMs := opts.WaitMs
	if waitMs == 0 {
		waitMs = DefaultPageWaitMs
	}
	req := probeRequest{
		Pages: opts.Pages, Storage: opts.Storage, WaitMs: waitMs,
		Texts: opts.Texts, CountSelectors: opts.CountSelectors,
		ExecutablePath: headlessShellPath(),
	}
	reqJSON, _ := json.Marshal(req)
	reqPath := filepath.Join(dir, "request.json")
	// .cjs so Node treats it as CommonJS whatever the nearest package.json says.
	scriptPath := filepath.Join(dir, "probe.cjs")
	if err := os.WriteFile(reqPath, reqJSON, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(scriptPath, []byte(pageProbeJS), 0o600); err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout == 0 {
		// Each page may spend its whole settle wait on top of loading, and a
		// page that does is reported STILL LOADING — which the run must live
		// long enough to say, rather than timing out on all of them.
		perPage := 30*time.Second + time.Duration(waitMs)*time.Millisecond
		timeout = 60*time.Second + time.Duration(len(opts.Pages))*perPage
	}
	cmd := exec.Command(node, scriptPath, reqPath)
	// An app on this machine is reached directly, whatever name it is checked
	// by: its dev channel must not be sent to an egress proxy (ako/mxcli#1800).
	pageURLs := make([]string, len(opts.Pages))
	for i, pg := range opts.Pages {
		pageURLs[i] = pg.URL
	}
	cmd.Env = directRouteEnv(append(os.Environ(), "NODE_PATH="+playwrightNodePath(node)), pageURLs...)
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching the page check: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("page check failed: %w\n%s", err, clip(out.String(), 600))
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("page check timed out after %s", timeout)
	}

	// The probe prints one JSON line; Playwright may print noise before it.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var sigs []PageSignals
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &sigs); err != nil || len(sigs) != len(opts.Pages) {
		return nil, fmt.Errorf("page check produced no verdict: %v\n%s", err, clip(out.String(), 600))
	}
	for i := range sigs {
		sigs[i].RequestedURL = opts.Pages[i].URL
	}
	return sigs, nil
}

// headlessShellPath is the Chromium the devcontainer pins for playwright-cli
// (`mxcli init` creates the symlink). Empty lets Playwright resolve its own.
func headlessShellPath() string {
	if p := os.Getenv("MXCLI_CHROMIUM"); p != "" {
		return p
	}
	const devcontainer = "/usr/local/bin/mx-headless-shell"
	if _, err := os.Stat(devcontainer); err == nil {
		return devcontainer
	}
	return ""
}

// playwrightNodePath lists the directories `require('playwright')` may be
// found in: the global node_modules, @playwright/cli's bundled copy (the
// devcontainer installs only that), the directory of the `playwright` CLI on
// PATH, and an existing NODE_PATH.
func playwrightNodePath(node string) string {
	var dirs []string
	add := func(d string) {
		if d == "" {
			return
		}
		for _, x := range dirs {
			if x == d {
				return
			}
		}
		dirs = append(dirs, d)
	}
	add(os.Getenv("NODE_PATH"))
	global := globalNodeModules(node)
	add(global)
	add(filepath.Join(global, "@playwright", "cli", "node_modules"))
	if pkg := resolvePlaywrightPkgDir(); pkg != "" {
		add(filepath.Dir(pkg))
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

// globalNodeModules resolves the global node_modules so `require('playwright')`
// works from a temp file. `npm root -g` is authoritative; the common install
// path is the fallback when npm is absent.
func globalNodeModules(node string) string {
	if npm, err := exec.LookPath("npm"); err == nil {
		if out, err := exec.Command(npm, "root", "-g").Output(); err == nil {
			if p := strings.TrimSpace(string(out)); p != "" {
				return p
			}
		}
	}
	// npm installs global packages under <prefix>/lib/node_modules, next to
	// <prefix>/bin/node.
	if node != "" {
		if real, err := filepath.EvalSymlinks(node); err == nil {
			cand := filepath.Join(filepath.Dir(filepath.Dir(real)), "lib", "node_modules")
			if st, err := os.Stat(cand); err == nil && st.IsDir() {
				return cand
			}
		}
	}
	return "/usr/lib/node_modules"
}

// formatPageVerdict renders signals as the line that replaces the screenshot.
//
// Brevity is the point, so everything unbounded is clamped: the verdict must
// stay far cheaper than the image even on a page with hundreds of rows and a
// wall of console noise, or there is no reason to prefer it.
func formatPageVerdict(label string, s PageSignals) string {
	var b strings.Builder
	fmt.Fprintf(&b, "page %s", label)
	// Where the browser ended, when that is not where it was sent: the title
	// and heading describe the page at this URL, which for an unknown /p/ URL
	// is the home page (ako/mxcli#998).
	if where, moved := s.landedAt(); moved {
		fmt.Fprintf(&b, "  url=%s", clip(where, 100))
	}
	if s.Status >= 400 {
		fmt.Fprintf(&b, "  http=%d", s.Status)
	}
	// A page the signed-in user may not open lands on the sign-in form too,
	// with messageCode=403 in the URL: saying LOGIN PAGE sent readers to sign in.
	switch {
	case s.AccessDenied():
		b.WriteString("  ACCESS DENIED")
	case s.LoginForm:
		b.WriteString("  LOGIN PAGE")
	}
	if s.Title != "" {
		fmt.Fprintf(&b, "  title=%q", clip(s.Title, 60))
	}
	if len(s.Headings) > 0 {
		fmt.Fprintf(&b, "  h=%q", clip(s.Headings[0], 60))
	}
	if s.Unsettled {
		b.WriteString("  STILL LOADING")
	}
	// rows=0 is said when there is a list to fill: leaving the field out made an
	// empty list look like a page without one (ako/mxcli#845).
	if s.Rows > 0 || s.DataWidgets > 0 {
		fmt.Fprintf(&b, "  rows=%d", s.Rows)
	}
	if s.TextLen == 0 {
		b.WriteString("  NO VISIBLE TEXT")
	} else {
		fmt.Fprintf(&b, "  text=%d", s.TextLen)
	}
	fmt.Fprintf(&b, "  console-errors=%d\n", len(s.ConsoleErrors))

	// Detail lines, capped. These are what make the verdict actionable rather
	// than merely cheap, and the console error is the one a picture cannot give.
	for _, a := range firstN(dedup(append(append([]string{}, s.ErrorAlerts...), s.Alerts...)), 2) {
		fmt.Fprintf(&b, "  ALERT  %s\n", clip(a, 140))
	}
	for _, e := range firstN(s.ConsoleErrors, 2) {
		fmt.Fprintf(&b, "  ERR    %s\n", clip(e, 140))
	}
	for _, h := range firstN(s.HTTPErrors, 2) {
		fmt.Fprintf(&b, "  HTTP   %s\n", clip(h, 140))
	}
	if s.Unsettled {
		fmt.Fprintf(&b, "  WAIT   %s\n", clip(s.unsettledReason(), 140))
	}
	return b.String()
}

// landedAt reports where the browser ended up when that differs from the URL
// the page was loaded from: the path, query and fragment on the same origin,
// the whole URL on another. A trailing slash on an empty path is no move.
func (s PageSignals) landedAt() (string, bool) {
	req, fin, ok := s.locations()
	if !ok {
		return "", false
	}
	if strings.EqualFold(req.Scheme+"://"+req.Host, fin.Scheme+"://"+fin.Host) {
		if pathOrRoot(req.Path) == pathOrRoot(fin.Path) && req.RawQuery == fin.RawQuery && req.Fragment == fin.Fragment {
			return "", false
		}
		return relativeURL(fin), true
	}
	return s.FinalURL, true
}

// relativeURL is u's path, query and fragment: how a URL on the app's own
// origin is shown.
func relativeURL(u *url.URL) string {
	where := pathOrRoot(u.EscapedPath())
	if u.RawQuery != "" {
		where += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		where += "#" + u.EscapedFragment()
	}
	return where
}

// wrongPage reports a /p/ page that the browser did not stay on, and where it
// went instead. Measured on 11.13 and 11.15: the runtime answers every
// /p/<x> with a 301 to /#/<x>, and the client replaces the location with
// /p/<x> for a page it has and with / for one it does not — rendering the home
// page, with HTTP 200 throughout (ako/mxcli#998). Matching is exact: /p/ITEMS
// and /p/items/ landed on / too. Only the path is compared; the query string
// survives the redirect, and so may a change of scheme in front of the app
// (http to https), which leaves the page the same. A page that went to the
// sign-in form is that form's failure, not this one, and so is a page the
// user was refused (/login.html?messageCode=403).
func (s PageSignals) wrongPage() (requested, landed string, home bool) {
	req, fin, ok := s.locations()
	if !ok || s.NeedsLogin() || s.AccessDenied() || !strings.Contains(req.Path, "/p/") || req.Path == fin.Path {
		return "", "", false
	}
	sameOrigin := strings.EqualFold(req.Scheme+"://"+req.Host, fin.Scheme+"://"+fin.Host)
	where, _ := s.landedAt()
	if where == "" {
		where = s.FinalURL
	}
	return req.Path, where, sameOrigin && pathOrRoot(fin.Path) == "/"
}

// locations parses the requested and final URLs; ok is false when either is
// missing (a probe that never got the page).
func (s PageSignals) locations() (req, fin *url.URL, ok bool) {
	if s.RequestedURL == "" || s.FinalURL == "" {
		return nil, nil, false
	}
	req, err1 := url.Parse(s.RequestedURL)
	fin, err2 := url.Parse(s.FinalURL)
	if err1 != nil || err2 != nil {
		return nil, nil, false
	}
	return req, fin, true
}

func pathOrRoot(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

// unsettledReason says why a page counted as still loading.
func (s PageSignals) unsettledReason() string {
	waited := fmt.Sprintf("%.1fs", float64(s.WaitedMs)/1000)
	if len(s.PendingRequests) > 0 {
		return fmt.Sprintf("not settled after %s: still waiting for %s", waited, strings.Join(s.PendingRequests, ", "))
	}
	return fmt.Sprintf("not settled after %s: the page was still changing", waited)
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func firstN(ss []string, n int) []string {
	if len(ss) > n {
		return ss[:n]
	}
	return ss
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
