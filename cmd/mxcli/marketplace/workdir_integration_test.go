//go:build integration

// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackageProject_BuildsUnderADeepTempDir reproduces mendixlabs/mxcli#1486
// with a real mx, on Linux, because the limit is mx's own and not Windows':
//
//	Extracting module files into C:\Users\<user>\AppData\Local\Temp\mxdiff1185592798\target-ref
//	The specified file name or path is too long, or a component of the specified path is too long.
//
// TMPDIR is pointed at a directory as deep as a Windows %TEMP%, and the package
// carries one file as deep as NanoflowCommons 7.x bundles. The control builds
// in the reported layout and must fail with the reported message; the fix
// builds in NewWorkDir's and must succeed.
func TestPackageProject_BuildsUnderADeepTempDir(t *testing.T) {
	const version = "11.12.1"
	if !mxVersionAvailable(version) {
		t.Skipf("mxbuild %s is not cached; run 'mxcli setup mxbuild --version %s'", version, version)
	}
	pkg := withDeepEntry(t, filepath.Join("..", "..", "..", "mx-modules", "BusinessEvents_3.12.0.mpk"), nanoflowCommons7Deepest)

	// %TEMP% for a 20-character user name, C:\Users\<user>\AppData\Local\Temp,
	// is 48 characters.
	deepTemp := "/tmp/mx1486-" + strings.Repeat("t", 48-len("/tmp/mx1486-"))
	if len(deepTemp) != 48 {
		t.Fatalf("deep temp is %d characters, want 48", len(deepTemp))
	}
	if err := os.MkdirAll(deepTemp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(deepTemp) })
	t.Setenv("TMPDIR", deepTemp)

	// Control: the layout the report shows, built exactly.
	reported := filepath.Join(deepTemp, "mxdiff1185592798", "target-ref")
	if err := os.MkdirAll(reported, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := PackageProject(context.Background(), pkg, version, reported, testBackend)
	if err == nil || !strings.Contains(err.Error(), "The specified file name or path is too long") {
		t.Fatalf("control: building under %s (%d characters) should fail as reported; got %v",
			reported, len(reported), err)
	}
	t.Logf("control failed as reported:\n%s", err)

	// Fix.
	work, err := NewWorkDir("mxd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) })
	refDir := filepath.Join(work, "t")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mpr, err := PackageProject(context.Background(), pkg, version, refDir, testBackend)
	if err != nil {
		t.Fatalf("building under %s (%d characters): %v", refDir, len(refDir), err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mpr), deepEntryName(nanoflowCommons7Deepest))); err != nil {
		t.Fatalf("mx did not write the deep entry: %v", err)
	}
	if strings.HasPrefix(work, deepTemp) {
		t.Errorf("NewWorkDir chose the deep TMPDIR (%s) over a shorter root", work)
	}
}

// deepEntryName is a project file path of exactly n characters, nested the way
// a bundled node_modules tree is.
func deepEntryName(n int) string {
	const head = "javasource/businessevents/node_modules/"
	var b strings.Builder
	b.WriteString(head)
	for b.Len() < n-len("x.txt")-20 {
		b.WriteString("deeply-nested-dir/")
	}
	b.WriteString(strings.Repeat("x", n-b.Len()-len(".txt")))
	b.WriteString(".txt")
	return b.String()
}

// withDeepEntry copies a package and adds one declared file of the given path
// length, so mx module-import has to write it.
func withDeepEntry(t *testing.T, src string, n int) string {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer zr.Close()

	name := deepEntryName(n)
	if len(name) != n {
		t.Fatalf("deep entry is %d characters, want %d", len(name), n)
	}
	out := filepath.Join(t.TempDir(), "deep.mpk")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range zr.File {
		if e.Name != "package.xml" {
			if err := zw.Copy(e); err != nil {
				t.Fatal(err)
			}
			continue
		}
		rc, err := e.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		manifest := strings.Replace(string(body), "<files>", "<files>\n      <file path=\""+name+"\" />", 1)
		w, err := zw.Create("package.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("deep")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}
