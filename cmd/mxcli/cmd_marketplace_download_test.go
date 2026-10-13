// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// downloadHandler serves a one-version list whose download resolves (303) to
// a CDN path ending in cdnName, the two hops Client.Download makes.
func downloadHandler(t *testing.T, cdnName, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/content/170/versions"):
			fmt.Fprintf(w, `{"items":[{"name":"Community Commons","versionNumber":"11.5.0","downloadUrl":%q}]}`, base+"/dl/170")
		case r.URL.Path == "/dl/170":
			http.Redirect(w, r, base+"/cdn/"+cdnName, http.StatusSeeOther)
		case r.URL.Path == "/cdn/"+cdnName:
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}
}

// `--output <existing dir>` writes the package into that directory under its
// CDN filename (mendixlabs/mxcli#1443). It used to stream to a temp file
// beside the directory and rename it onto the directory: "rename … file exists".
func TestMarketplaceDownload_OutputDirectory(t *testing.T) {
	dir := t.TempDir()
	out, err := runMarketplace(t, downloadHandler(t, "CommunityCommons.mpk", "mpk-bytes"),
		"download", "170", "--output", dir)
	if err != nil {
		t.Fatalf("download into an existing directory: %v\n%s", err, out)
	}
	want := filepath.Join(dir, "CommunityCommons.mpk")
	got, rerr := os.ReadFile(want)
	if rerr != nil || string(got) != "mpk-bytes" {
		t.Fatalf("want the package at %s, read %q (%v)\n%s", want, got, rerr, out)
	}
	if !strings.Contains(out, want) {
		t.Errorf("output does not name %s:\n%s", want, out)
	}
	assertNoPartFiles(t, dir)
	assertNoPartFiles(t, filepath.Dir(dir))
}

// A trailing separator names a directory even when it does not exist yet.
func TestMarketplaceDownload_OutputTrailingSeparator(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mods") + string(os.PathSeparator)
	out, err := runMarketplace(t, downloadHandler(t, "CommunityCommons.mpk", "mpk-bytes"),
		"download", "170", "-o", dir)
	if err != nil {
		t.Fatalf("download into a new directory: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "CommunityCommons.mpk")); err != nil {
		t.Fatalf("package not written into %s: %v\n%s", dir, err, out)
	}
}

// Control: a file path keeps working as before.
func TestMarketplaceDownload_OutputFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "dbc.mpk")
	out, err := runMarketplace(t, downloadHandler(t, "CommunityCommons.mpk", "mpk-bytes"),
		"download", "170", "-o", dest)
	if err != nil {
		t.Fatalf("download to a file path: %v\n%s", err, out)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "mpk-bytes" {
		t.Fatalf("want the package at %s, read %q (%v)", dest, got, err)
	}
}

func assertNoPartFiles(t *testing.T, dir string) {
	t.Helper()
	parts, _ := filepath.Glob(filepath.Join(dir, ".mxcli-download-*.part"))
	if len(parts) > 0 {
		t.Errorf("temp files left behind in %s: %v", dir, parts)
	}
}
