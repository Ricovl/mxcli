// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelsdk "github.com/mendixlabs/mxcli"
)

// TestCachedReferenceDescribesActionSourcesLikeAFreshOne is mendixlabs/mxcli#1441.
//
// `marketplace update --to` refused an untouched NanoflowCommons with "44
// element(s) have been changed locally", every one a JAVASCRIPT_ACTION. DESCRIBE
// of a JavaScript (or Java) action reads its source from javascriptsource/
// (javasource/) beside the .mpr, and the finished-reference cache kept only the
// model. A cache MISS hands out the freshly built reference with its sources and
// compares clean; a cache HIT hands out a reference whose actions describe as the
// "source not available" placeholder, against the project's real code.
//
// So the test diffs the same untouched module twice — miss, then hit — and both
// must be verified Unchanged. Asserting Unchanged (not merely "not Modified")
// matters: the gate treats the placeholder as inconclusive, which would turn the
// hit's false "Modified" into an equally blocking "Unknown".
func TestCachedReferenceDescribesActionSourcesLikeAFreshOne(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MXCLI_NO_REF_CACHE", "")
	t.Setenv("MXCLI_REF_CACHE_MAX", "0")

	const module = "Administration"
	// The reference built from the package: a project whose module carries
	// action sources on disk, as `mx module-import` leaves it.
	ref := copyFixture(t)
	execMDL(t, ref, `
create javascript action Administration.Issue1441Js() returns Boolean
as $$
return Promise.resolve(true);
$$;
create java action Administration.Issue1441Java() returns boolean
as $$
return true;
$$;
`)
	refDir := filepath.Dir(ref)
	assertSourceOnDisk(t, refDir, "javascriptsource")
	assertSourceOnDisk(t, refDir, "javasource")

	// The user's project: the same module, untouched.
	installedDir := t.TempDir()
	if err := copyTree(refDir, installedDir); err != nil {
		t.Fatalf("copy project: %v", err)
	}
	installed, err := SnapshotModule(filepath.Join(installedDir, filepath.Base(ref)), module, testBackend)
	if err != nil {
		t.Fatalf("snapshot project: %v", err)
	}

	mendixVersion := stampedVersion(t, ref)
	const versionID = "00000000-1441-0000-0000-000000000000"
	mpk := filepath.Join(t.TempDir(), "package.mpk")
	mustWrite(t, mpk, "pkg")

	// Run 1: cache miss — the fresh reference is compared, then cached.
	if got := CachedReference(versionID, mendixVersion, t.TempDir(), filepath.Join(t.TempDir(), "p.mpk")); got != "" {
		t.Fatalf("expected a cache miss on a fresh HOME, got %s", got)
	}
	fresh, err := SnapshotModule(ref, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot fresh reference: %v", err)
	}
	assertUntouched(t, "cache miss", installed, fresh)
	CacheReference(versionID, mendixVersion, refDir, mpk)

	// Run 2: cache hit — the restored reference must describe identically.
	hitDir := t.TempDir()
	hit := CachedReference(versionID, mendixVersion, hitDir, filepath.Join(t.TempDir(), "p.mpk"))
	if hit == "" {
		t.Fatal("expected a cache hit after CacheReference")
	}
	cached, err := SnapshotModule(hit, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot cached reference: %v", err)
	}
	assertUntouched(t, "cache hit", installed, cached)
}

func assertSourceOnDisk(t *testing.T, dir, sub string) {
	t.Helper()
	var found bool
	_ = filepath.Walk(filepath.Join(dir, sub), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.Contains(p, "Issue1441") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("creating the action wrote no source under %s/; the test would prove nothing", sub)
	}
}

func stampedVersion(t *testing.T, mpr string) string {
	t.Helper()
	r, err := modelsdk.Open(mpr)
	if err != nil {
		t.Fatalf("open %s: %v", mpr, err)
	}
	defer func() { _ = r.Disconnect() }()
	v, _ := r.GetMendixVersion()
	return v
}

func assertUntouched(t *testing.T, run string, installed, ref *Snapshot) {
	t.Helper()
	rep := Compare(installed, ref)
	for _, name := range []ElementKey{
		{Type: "JAVASCRIPT_ACTION", Name: "Issue1441Js"},
		{Type: "JAVA_ACTION", Name: "Issue1441Java"},
	} {
		if _, ok := ref.Elements[name]; !ok {
			t.Fatalf("%s: %s not enumerated; keys=%v", run, name, ref.Keys())
		}
	}
	for _, f := range rep.Findings {
		if f.Verdict != Unchanged {
			t.Errorf("%s: %s reported %s on an untouched module (%s)", run, f.Key, f.Verdict, f.Reason)
			if f.Verdict == Modified {
				t.Errorf("  project:   %s", f.InstalledMDL)
				t.Errorf("  reference: %s", f.PackageMDL)
			}
		}
	}
}
