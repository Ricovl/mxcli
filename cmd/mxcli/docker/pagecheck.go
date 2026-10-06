// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"fmt"
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
	// TextFound and Counts answer the probe's text / count queries, index-aligned
	// with ProbeOptions.Texts and ProbeOptions.CountSelectors.
	TextFound []bool `json:"textFound"`
	Counts    []int  `json:"counts"`
	// Screenshot is the PNG written for this page, when one was asked for.
	Screenshot string `json:"screenshot"`
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
    const out = { title: '', headings: [], alerts: [], errorAlerts: [], textLen: 0, rows: 0,
      consoleErrors, finalUrl: '', status: 0, httpErrors, loginForm: false,
      textFound: [], counts: [], screenshot: '' };
    try {
      const resp = await p.goto(target.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
      out.status = resp ? resp.status() : 0;
      // Wait for the Mendix client to mount a page (or the login form), then
      // for the body text to stop changing — data sources load after the page
      // shell, so a fixed sleep is either too short or wasted.
      await p.waitForSelector('.mx-page, #usernameInput', { timeout: 20000 }).catch(() => {});
      const deadline = Date.now() + (req.waitMs || 4000);
      let last = -1, stable = 0;
      while (Date.now() < deadline) {
        const st = await p.evaluate(() => ({
          len: (document.body && document.body.innerText || '').length,
          busy: document.querySelectorAll('.mx-progress, .mx-underlay-loading, [aria-busy="true"]').length,
        })).catch(() => ({ len: -2, busy: 0 }));
        if (st.len === last && st.busy === 0) { if (++stable >= 3) break; } else { stable = 0; }
        last = st.len;
        await p.waitForTimeout(250);
      }
      out.finalUrl = p.url();
      out.title = await p.title();
      out.loginForm = (await p.locator('#usernameInput').count()) > 0;
      // The demo-user switcher (development mode) has its own "Select user"
      // heading, which is not the page's.
      out.headings = await p.$$eval('h1,h2', ns => ns.filter(n => !n.closest('.mx-demouserswitcher'))
        .map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      out.alerts = await p.$$eval('.alert-danger,.alert-warning,.mx-validation-message',
        ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      out.errorAlerts = await p.$$eval('.alert-danger,.mx-dialog-error .mx-dialog-body',
        ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
      const body = (await p.$eval('body', n => n.innerText || '')).trim();
      out.textLen = body.length;
      // Data rows only: a list view's "No items found" placeholder is an <li>
      // too, and a data grid 2 header row is a role=row without grid cells.
      out.rows = await p.$$eval('.mx-datagrid tbody tr, .mx-listview > ul > li:not(.mx-listview-empty), [role="row"]:has([role="gridcell"])', ns => ns.length);
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
	// WaitMs caps how long a page may keep settling after it mounts (default 4000).
	WaitMs int
	// Texts and CountSelectors are evaluated on every page; the answers come
	// back index-aligned in PageSignals.TextFound / Counts.
	Texts          []string
	CountSelectors []string
	Timeout        time.Duration // whole run; default 60s + 30s per page
	MxBuildPath    string        // node fallback when none is on PATH
}

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
		waitMs = 4000
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
		timeout = 60*time.Second + time.Duration(len(opts.Pages))*30*time.Second
	}
	cmd := exec.Command(node, scriptPath, reqPath)
	cmd.Env = append(os.Environ(), "NODE_PATH="+playwrightNodePath(node))
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
	if s.Status >= 400 {
		fmt.Fprintf(&b, "  http=%d", s.Status)
	}
	if s.LoginForm {
		b.WriteString("  LOGIN PAGE")
	}
	if s.Title != "" {
		fmt.Fprintf(&b, "  title=%q", clip(s.Title, 60))
	}
	if len(s.Headings) > 0 {
		fmt.Fprintf(&b, "  h=%q", clip(s.Headings[0], 60))
	}
	if s.Rows > 0 {
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
	return b.String()
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
