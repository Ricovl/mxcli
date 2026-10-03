// SPDX-License-Identifier: Apache-2.0

// `run --local --watch` on Mendix 11.13: a page added while the loop runs is
// never bundled. The serve build writes web/pages/<Page>.js, the incremental
// rollup watcher rebuilds, the apply is reported as successful — and the browser
// 404s on dist/pages/<Page>.js when the page is opened. Restarting the loop
// fixed it, because a fresh rollup run re-globs web/pages.
//
// The defect is in mxbuild's rollup-plugin-mendix-pages.mjs: watchChange tests
// the changed path against "./pages/", which path.relative() never produces, so
// the page list is only ever globbed once, at watcher start.
package docker

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pagesDeployment writes a 11.13-shaped web/ tree: the generated page modules
// under web/pages, and the bundle under web/dist with the given pages emitted.
func pagesDeployment(t *testing.T, source, bundled []string) string {
	t.Helper()
	deploy := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(deploy, "web", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("dist/index.js", "//")
	for _, p := range source {
		write("pages/"+p, "export default {};")
	}
	for _, p := range bundled {
		write("dist/pages/"+p, "export default {};")
	}
	return deploy
}

func TestMissingPageChunks_NewPageNotBundled(t *testing.T) {
	// The reported failure: the serve build wrote the new page's module, the
	// running watcher did not emit it.
	deploy := pagesDeployment(t,
		[]string{"MyFirstModule.Home_Web.js", "MyFirstModule.NewPage.js"},
		[]string{"MyFirstModule.Home_Web.js"})
	got := missingPageChunks(deploy)
	if len(got) != 1 || got[0] != "MyFirstModule.NewPage.js" {
		t.Fatalf("got %v, want [MyFirstModule.NewPage.js]", got)
	}
}

func TestMissingPageChunks_AllBundledIsNotReported(t *testing.T) {
	// Control: this gates a bundler restart (a cold build), so a false positive
	// would cost seconds on every apply.
	deploy := pagesDeployment(t,
		[]string{"MyFirstModule.Home_Web.js", "Administration.Account_Edit.js"},
		[]string{"MyFirstModule.Home_Web.js", "Administration.Account_Edit.js"})
	if got := missingPageChunks(deploy); len(got) != 0 {
		t.Fatalf("fully bundled pages reported as missing: %v", got)
	}
}

func TestMissingPageChunks_NestedPathIsPreserved(t *testing.T) {
	// The plugin emits each page at its path relative to web/, so a page module in
	// a subfolder lands in the same subfolder under dist.
	deploy := pagesDeployment(t,
		[]string{"Mod/A.js", "Mod/B.js"},
		[]string{"Mod/A.js", "B.js"})
	got := missingPageChunks(deploy)
	if len(got) != 1 || got[0] != "Mod/B.js" {
		t.Fatalf("got %v, want [Mod/B.js]", got)
	}
}

func TestMissingPageChunks_IgnoresNonModules(t *testing.T) {
	deploy := pagesDeployment(t, []string{"A.js", "A.js.map", "notes.txt"}, []string{"A.js"})
	if got := missingPageChunks(deploy); len(got) != 0 {
		t.Fatalf("non-.js files reported as missing pages: %v", got)
	}
}

func TestMissingPageChunks_NoPageSourceIsNotReported(t *testing.T) {
	// Mendix 11.14+ writes web/dist itself and leaves no web/pages; the classic
	// client has no dist at all. Neither shape has anything to compare.
	deploy := pagesDeployment(t, nil, []string{"A.js"})
	if got := missingPageChunks(deploy); len(got) != 0 {
		t.Fatalf("no web/pages: got %v, want nothing", got)
	}
	noDist := t.TempDir()
	if err := os.MkdirAll(filepath.Join(noDist, "web", "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noDist, "web", "pages", "A.js"), []byte("//"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := missingPageChunks(noDist); len(got) != 0 {
		t.Fatalf("no web/dist/index.js: got %v, want nothing (clientBundlePresent's business)", got)
	}
}

func TestRecoverMissingPages_RestartsOnlyWhenAPageIsMissing(t *testing.T) {
	calls := 0
	restart := func() error { calls++; return nil }

	// Control first: nothing missing, nothing restarted, nothing printed.
	healthy := pagesDeployment(t, []string{"A.js"}, []string{"A.js"})
	var out strings.Builder
	did, err := recoverMissingPages(healthy, restart, &out)
	if err != nil || did || calls != 0 || out.Len() != 0 {
		t.Fatalf("healthy: did=%v err=%v calls=%d out=%q; want no restart", did, err, calls, out.String())
	}

	// The restart stands in for a fresh rollup run, which re-globs web/pages and
	// emits the new page.
	broken := pagesDeployment(t, []string{"A.js", "B.js"}, []string{"A.js"})
	rebundle := func() error {
		calls++
		return os.WriteFile(filepath.Join(broken, "web", "dist", "pages", "B.js"), []byte("//"), 0o600)
	}
	did, err = recoverMissingPages(broken, rebundle, &out)
	if err != nil || !did || calls != 1 {
		t.Fatalf("missing page: did=%v err=%v calls=%d; want one restart", did, err, calls)
	}
	if lines := strings.Count(strings.TrimSpace(out.String()), "\n"); lines != 0 || !strings.Contains(out.String(), "B.js") {
		t.Errorf("want one line naming the page, got %q", out.String())
	}

	// A restart that still leaves the page out is an error, not a success.
	stale := pagesDeployment(t, []string{"A.js", "C.js"}, []string{"A.js"})
	if _, err := recoverMissingPages(stale, restart, io.Discard); err == nil || !strings.Contains(err.Error(), "C.js") {
		t.Fatalf("page still missing after restart not reported: %v", err)
	}

	// A failed restart is surfaced, not swallowed.
	_, err = recoverMissingPages(stale, func() error { return errors.New("boom") }, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("restart failure not surfaced: %v", err)
	}
}

// TestEnsureClientServed_RecoversWhenAPageIsMissing: index.js is present and
// served, so the existing probe passes; a page module with no bundled chunk must
// still take the re-bundle branch. The bogus mxbuild path makes that branch fail
// visibly, which proves it ran (TestEnsureClientServed_NoRecoveryWhenServed is
// the control: same server, no missing page, no error).
func TestEnsureClientServed_RecoversWhenAPageIsMissing(t *testing.T) {
	deploy := pagesDeployment(t, []string{"A.js", "B.js"}, []string{"A.js"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dist/index.js" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := ensureClientServed(deploy, srv.URL+"/", "/nonexistent/mxbuild", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "re-bundle") {
		t.Fatalf("expected the re-bundle branch to run for a missing page chunk, got: %v", err)
	}
}
