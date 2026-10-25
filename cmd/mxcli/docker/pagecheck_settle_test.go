// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#845: `--page-check` could give its verdict before the page's data
// source had rendered, and nothing in the verdict said so. Measured on 11.13.0
// against a list view whose microflow data source takes 2s and 8s: while the
// data is out, the list view shows NOTHING that says it is loading — `.mx-listview`
// sits empty, with no progress element, no aria-busy, no "no items" placeholder,
// and the body text holds still. The only sign is the request itself, a
// POST /xas/ runtimeOperation that stays open. The old probe waited for the text
// to stop changing (750ms) or 4s, whichever came first, so both pages were
// reported `text=37` with no rows and `playwright check` exited 0 — "OK 4 page(s)".
// (A data grid 2 does show a spinner with aria-busy, which is why grids were
// measured correctly and list views were not.)
//
// The probe now also waits for the page's same-origin requests to finish, and a
// page still loading when the wait runs out is reported as such and fails.

func TestVerdictSaysWhenThePageWasStillLoading(t *testing.T) {
	got := formatPageVerdict("/p/items", PageSignals{
		Title: "Items", TextLen: 37, Unsettled: true, WaitedMs: 4000,
		PendingRequests: []string{"POST /xas/"}, DataWidgets: 1,
	})
	for _, want := range []string{"STILL LOADING", "POST /xas/", "4.0s", "rows=0"} {
		if !strings.Contains(got, want) {
			t.Errorf("verdict omits %q; a half-rendered page must not read as a finished one:\n%s", want, got)
		}
	}
}

func TestEvaluatePage_StillLoadingFails(t *testing.T) {
	sig := healthy()
	sig.Unsettled, sig.WaitedMs, sig.PendingRequests = true, 4000, []string{"POST /xas/"}
	fails := EvaluatePage(sig, CheckSpec{})
	if len(fails) != 1 || !strings.Contains(fails[0], "still loading") || !strings.Contains(fails[0], "--wait") {
		t.Errorf("a page measured before it finished loading must fail, naming --wait: %v", fails)
	}
	// Control: the same page, settled, passes.
	if f := EvaluatePage(healthy(), CheckSpec{}); len(f) != 0 {
		t.Errorf("control: a settled healthy page failed: %v", f)
	}
}

// The rows field used to be left out when it was 0, so "no rows" and "no grid"
// read the same — the missing key was the only trace of #845's symptom. A page
// that has a list view or grid now always says how many rows it rendered; a
// page without one still says nothing about rows.
func TestVerdictShowsRowsZeroOnlyWhenThereIsAListToFill(t *testing.T) {
	if got := formatPageVerdict("/p/items", PageSignals{Title: "Items", TextLen: 37, DataWidgets: 1}); !strings.Contains(got, "rows=0") {
		t.Errorf("an empty list view is not reported as rows=0:\n%s", got)
	}
	if got := formatPageVerdict("/p/about", PageSignals{Title: "About", TextLen: 300}); strings.Contains(got, "rows=") {
		t.Errorf("a page with no list or grid reports rows:\n%s", got)
	}
}

// fakeMendixPage serves the shape measured on 11.13: the page shell (.mx-page and
// an empty .mx-listview) renders at once, then the list asks POST /xas/ for its
// rows and draws them when the answer comes. delay is how long the "data source"
// takes — simulated latency, the thing under test, not a synchronization.
func fakeMendixPage(delay time.Duration) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/p/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Items</title></head><body>
<div class="mx-page"><h1>Item overview</h1>
<div class="mx-listview"><ul id="rows"></ul></div></div>
<script>
fetch('/xas/', {method: 'POST', body: '{"action":"runtimeOperation"}'})
  .then(r => r.json())
  .then(rows => rows.forEach(t => {
    const li = document.createElement('li'); li.textContent = t;
    document.getElementById('rows').appendChild(li);
  }));
</script></body></html>`)
	})
	mux.HandleFunc("/xas/", func(w http.ResponseWriter, r *http.Request) {
		// Reading the body to the end is what lets the server notice the
		// browser hanging up (r.Context() is cancelled by the background read
		// that starts after it), so a test that gives up early ends early.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-time.After(delay):
		case <-r.Context().Done(): // the probe gave up and closed the page
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `["Row A","Row B"]`)
	})
	return httptest.NewServer(mux)
}

// probeOrSkip runs the real probe, skipping when this machine has no Node or no
// Playwright package — which reads like a pass, so the PR that touches the
// probe states the run it was verified with.
func probeOrSkip(t *testing.T, opts ProbeOptions) PageSignals {
	t.Helper()
	opts.MxBuildPath = anyMxBuildPath()
	sigs, err := ProbePages(opts)
	if err != nil {
		if strings.Contains(err.Error(), "node not found") || strings.Contains(err.Error(), "Cannot find module") ||
			strings.Contains(err.Error(), "Executable doesn't exist") {
			t.Skipf("no browser tooling here (set NODE_PATH to a Playwright install): %v", err)
		}
		t.Fatalf("probe failed: %v", err)
	}
	return sigs[0]
}

// The reported symptom: a data source slower than the old text-stable window.
// Before the fix this came back rows=0 with nothing flagged.
func TestProbeWaitsForTheDataSourceToRender(t *testing.T) {
	srv := fakeMendixPage(2 * time.Second)
	defer srv.Close()
	sig := probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: srv.URL + "/p/items"}}})
	if sig.Rows != 2 {
		t.Errorf("rows = %d, want 2: the verdict was taken before the data source answered (text=%d)", sig.Rows, sig.TextLen)
	}
	if sig.Unsettled {
		t.Errorf("a page that finished loading inside the wait is reported as still loading: %+v", sig)
	}
}

// A data source slower than the whole wait cannot be waited for, and must not
// be passed off as a finished page either: the verdict says the page was still
// loading and names the open request.
func TestProbeReportsAPageThatOutlastsTheWait(t *testing.T) {
	srv := fakeMendixPage(20 * time.Second)
	defer srv.Close()
	sig := probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: srv.URL + "/p/items"}}, WaitMs: 2500})
	if !sig.Unsettled {
		t.Fatalf("a page whose data never arrived was not reported as still loading: rows=%d text=%d", sig.Rows, sig.TextLen)
	}
	if len(sig.PendingRequests) == 0 || sig.PendingRequests[0] != "POST /xas/" {
		t.Errorf("pending = %v, want the open POST /xas/", sig.PendingRequests)
	}
	if fails := EvaluatePage(sig, CheckSpec{}); len(fails) == 0 {
		t.Errorf("a page measured mid-load passed the check")
	}
	if sig.DataWidgets != 1 {
		t.Errorf("dataWidgets = %d, want 1 (the list view)", sig.DataWidgets)
	}
}
