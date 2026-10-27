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

// `playwright check` on a page that shows the sign-in form waited out the whole
// --wait before answering, then reported the page as still loading on top of
// the sign-in form. Measured on 11.13.0 against a secured page with no session:
//
//	WAIT   not settled after 30.2s: still waiting for POST /xas/
//	FAIL   still loading when measured, … (raise --wait if it is only slow)
//	FAIL   the sign-in form is showing, not the page (…)
//
// at --wait 30s (33s wall time; 8s at --wait 5s). The client keeps a POST /xas/
// open behind the sign-in form, so the probe's "no request open" condition never
// held. The sign-in form is a verdict in itself, and raising --wait is the
// wrong advice for it.

// fakeSignInPage serves the sign-in form the client shows for a page that needs
// a session, with a POST /xas/ that does not answer while the form is up.
func fakeSignInPage() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/p/secret", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head><title>Mendix</title></head><body>
<form><h2>Sign in</h2><input id="usernameInput"><input id="passwordInput" type="password">
<button id="loginButton">Sign in</button></form>
<script>fetch('/xas/', {method: 'POST', body: '{"action":"get_session_data"}'});</script>
</body></html>`)
	})
	mux.HandleFunc("/xas/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-time.After(60 * time.Second):
		case <-r.Context().Done():
		}
	})
	return httptest.NewServer(mux)
}

func TestProbeDoesNotWaitOutTheWaitOnTheSignInForm(t *testing.T) {
	srv := fakeSignInPage()
	defer srv.Close()
	const waitMs = 20000
	start := time.Now()
	sig := probeOrSkip(t, ProbeOptions{Pages: []PageTarget{{URL: srv.URL + "/p/secret"}}, WaitMs: waitMs})
	took := time.Since(start)

	if !sig.LoginForm {
		t.Fatalf("the sign-in form was not recognised: %+v", sig)
	}
	if sig.Unsettled || sig.WaitedMs >= waitMs {
		t.Errorf("the probe waited out --wait on the sign-in form (waited %dms of %dms, unsettled=%v)",
			sig.WaitedMs, waitMs, sig.Unsettled)
	}
	// Browser start-up is most of the remaining time; a wait served in full
	// would put this above 20s.
	if took > 12*time.Second {
		t.Errorf("checking a sign-in form took %s with --wait %dms", took.Round(time.Second), waitMs)
	}
}

// The verdict for the sign-in form names the sign-in form, not loading: even a
// form measured while unsettled (a client still busy behind it) is not cured by
// a longer wait.
func TestEvaluatePage_SignInFormIsNotStillLoading(t *testing.T) {
	sig := healthy()
	sig.LoginForm, sig.Unsettled, sig.PendingRequests = true, true, []string{"POST /xas/"}
	fails := EvaluatePage(sig, CheckSpec{})
	if len(fails) != 1 || !strings.Contains(fails[0], "sign-in form") {
		t.Errorf("want the sign-in form as the one failure, got %q", fails)
	}
	// Control: an unsettled page that is not the sign-in form still fails as
	// still loading.
	sig.LoginForm = false
	if fails := EvaluatePage(sig, CheckSpec{}); len(fails) == 0 || !strings.Contains(fails[0], "still loading") {
		t.Errorf("control: an unsettled page no longer fails as still loading: %q", fails)
	}
}
