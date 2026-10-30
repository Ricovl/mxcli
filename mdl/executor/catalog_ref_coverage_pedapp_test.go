// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// ako/mxcli#1804: an incremental build rebuilds a document when a unit its
// refs name changes, so refs must cover every other document a pass reads
// for it. A read they miss is a row that goes stale without an error. The
// build traces every name its lookups read per document, and every read
// must be one a change would follow.

// tracedFullBuild builds the catalog from scratch with every read traced and
// returns how much of it refs cover.
func tracedFullBuild(t *testing.T, exec *Executor) *catalog.DependencyCoverage {
	t.Helper()
	var builder *catalog.Builder
	catalogBuilderHook = func(b *catalog.Builder) {
		b.SetTraceDependencies(true)
		builder = b
	}
	defer func() { catalogBuilderHook = nil }()
	ctx := exec.newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	ctx.catalogForce = true
	if err := buildCatalog(ctx, true, false, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	defer func() { ctx.Catalog.Close(); ctx.Catalog = nil }()
	cov, err := builder.DependencyCoverage()
	if err != nil {
		t.Fatal(err)
	}
	return cov
}

func coverageReport(cov *catalog.DependencyCoverage, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d reads of another document covered by refs (%d of the document's own unit, %d of names no unit holds)",
		cov.Covered, cov.Reads, cov.Self, cov.Unowned)
	bySite := map[string]int{}
	for _, u := range cov.Uncovered {
		bySite[u.Site]++
	}
	for site, n := range bySite {
		fmt.Fprintf(&b, "\n  %s: %d uncovered", site, n)
	}
	for i, u := range cov.Uncovered {
		if i == limit {
			fmt.Fprintf(&b, "\n  … %d more", len(cov.Uncovered)-limit)
			break
		}
		fmt.Fprintf(&b, "\n  %s read %s (held by %s's unit) through %s", u.Reader, u.Name, u.Holder, u.Site)
	}
	return b.String()
}

func TestCatalog_RefsCoverEveryCrossDocumentRead(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setupIncrementalProbes(t, exec, out)
	cov := tracedFullBuild(t, exec)
	if cov.Reads == 0 {
		t.Fatal("control: the build traced no read of another document; the tracer is not wired")
	}
	if len(cov.Uncovered) > 0 {
		t.Errorf("refs miss reads the incremental build depends on; a change to the read document leaves the reader's rows stale.\n"+
			"Emit a refs edge from the reader to what it reads (builder_references.go), and add an incremental step for it:\n%s",
			coverageReport(cov, 20))
	}
	t.Log(coverageReport(cov, 0))
}

// TestCatalog_RefCoverageOfProject measures ref coverage on a real project:
// MXCLI_REF_COVERAGE_MPR=/path/to/app.mpr (a copy: the catalog cache is
// written beside it). It reports, and fails only on an uncovered read, so a
// project that shows a gap is a reproduction to add to the test above.
func TestCatalog_RefCoverageOfProject(t *testing.T) {
	mpr := os.Getenv("MXCLI_REF_COVERAGE_MPR")
	if mpr == "" {
		t.Skip("set MXCLI_REF_COVERAGE_MPR to a project copy to measure it")
	}
	exec := New(&bytes.Buffer{})
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	cov := tracedFullBuild(t, exec)
	t.Logf("%s: %s", filepath.Base(mpr), coverageReport(cov, 50))
	if len(cov.Uncovered) > 0 {
		t.Fail()
	}
}
