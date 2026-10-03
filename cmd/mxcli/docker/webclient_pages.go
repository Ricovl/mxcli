// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// missingPageChunks returns the generated page modules (web/pages/**/*.js) that
// have no bundled counterpart under web/dist/pages, as slash-separated paths
// relative to web/pages.
//
// Why this exists: on deployments that mxcli bundles with rollup (Mendix 11.13
// and earlier), mxbuild's rollup-plugin-mendix-pages globs web/pages once, at
// bundler start, and never again — its watchChange compares the changed path
// against "./pages/", a prefix path.relative() never produces. So under --watch a
// page ADDED while the loop runs is written by the serve build, the incremental
// bundler rebuilds without it, and the browser 404s on dist/pages/<Page>.js when
// the page is opened. Neither the index.js probe nor danglingClientChunks can see
// it: pages are loaded by dynamic import, so nothing in the static graph refers to
// the missing file.
//
// The check is gated on the deployment's shape, not the Mendix version: it only
// applies when web/pages holds page modules AND web/dist/index.js exists. Mendix
// 11.14+ (mxbuild writes dist itself, no web/pages) and the classic client (no
// dist) both yield nothing. An empty result means "nothing missing".
func missingPageChunks(deployDir string) []string {
	webDir := filepath.Join(deployDir, "web")
	if _, err := os.Stat(filepath.Join(webDir, "dist", "index.js")); err != nil {
		return nil // no bundle at all is clientBundlePresent's business
	}
	srcDir := filepath.Join(webDir, "pages")
	distDir := filepath.Join(webDir, "dist", "pages")

	var missing []string
	_ = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".js") {
			return nil // unreadable entries are skipped: this gates a recovery
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return nil
		}
		if _, err := os.Stat(filepath.Join(distDir, rel)); err != nil {
			missing = append(missing, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(missing)
	return missing
}

// recoverMissingPages runs rebundle when a page module has no bundled chunk, and
// reports it in one line. It returns whether a re-bundle ran. Under --watch
// rebundle restarts the incremental bundler (a one-shot bundle would be right
// until the stale watcher's next rebuild, which still does not know the page);
// otherwise it is a one-shot BuildWebClient.
func recoverMissingPages(deployDir string, rebundle func() error, out io.Writer) (bool, error) {
	missing := missingPageChunks(deployDir)
	if len(missing) == 0 {
		return false, nil
	}
	fmt.Fprintf(out, "  %d page(s) missing from the client bundle (%s) — mxbuild's incremental bundler does not pick up added pages; re-bundling...\n",
		len(missing), strings.Join(missing, ", "))
	if err := rebundle(); err != nil {
		return true, fmt.Errorf("re-bundling for added pages: %w", err)
	}
	if still := missingPageChunks(deployDir); len(still) > 0 {
		return true, fmt.Errorf("pages still not bundled after re-bundle: %s", strings.Join(still, ", "))
	}
	return true, nil
}
