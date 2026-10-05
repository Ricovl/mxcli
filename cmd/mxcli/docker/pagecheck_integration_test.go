// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestPlaywrightCheck_AgainstRunningApp drives the real probe and login against
// an app that is already running (`mxcli run --local`), so it needs one:
//
//	MXCLI_PAGECHECK_URL=http://localhost:8080 \
//	MXCLI_PAGECHECK_PROJECT=/path/app.mpr \   # optional: demo-user login
//	go test -tags integration ./cmd/mxcli/docker/ -run TestPlaywrightCheck_AgainstRunningApp
//
// Without MXCLI_PAGECHECK_URL it skips — which reads like a pass, so the PR
// that touches the probe states the run it was verified with.
func TestPlaywrightCheck_AgainstRunningApp(t *testing.T) {
	base := os.Getenv("MXCLI_PAGECHECK_URL")
	if base == "" {
		t.Skip("MXCLI_PAGECHECK_URL not set; needs a running Mendix app")
	}
	project := os.Getenv("MXCLI_PAGECHECK_PROJECT")

	var out bytes.Buffer
	res, err := RunPageCheck(PageCheckOptions{
		BaseURL: base, Targets: []string{"/"}, ProjectPath: project,
		StorageDir: t.TempDir(), Stdout: &out,
	})
	if err != nil {
		t.Fatalf("check could not run: %v", err)
	}
	if res.Failed != 0 {
		t.Errorf("the home page failed:\n%s", out.String())
	}

	// The control: a page that cannot pass must fail, or the green run above
	// says nothing.
	out.Reset()
	res, err = RunPageCheck(PageCheckOptions{
		BaseURL: base, Targets: []string{"/mxcli-no-such-page.html"}, ProjectPath: project,
		StorageDir: t.TempDir(), Stdout: &out,
	})
	if err != nil {
		t.Fatalf("check could not run: %v", err)
	}
	if res.Failed != 1 || !strings.Contains(out.String(), "HTTP 404") {
		t.Errorf("a 404 page did not fail with HTTP 404:\n%s", out.String())
	}
}
