// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pausedACTDebug is get_paused_microflows as a Mendix 11.15 runtime answered it
// with MyFirstModule.ACT_Debug paused on its first activity (measured for
// ako/mxcli#1782): a String, two objects and an enumeration in scope.
const pausedACTDebug = `{"result":{"paused_microflows":[{"debug_id":"dbg-1","variables":{` +
	`"currentSession":{"id":"13229323905401380","state":"NORMAL","type":"object","entity":"System.Session"},` +
	`"Greeting":{"type":"string","value":"hello"},` +
	`"currentUser":{"id":"13792273858822669","state":"NORMAL","type":"object","entity":"System.User"},` +
	`"currentDeviceType":{"enumeration_name":"System.DeviceType","type":"enum","value":"Desktop"}},` +
	`"microflow_id":"c5b44d8c-defc-42dd-aa6c-65acc647ce17","object_name":"CreateAndChange",` +
	`"microflow_name":"MyFirstModule.ACT_Debug","object_id":"11276f01-9157-4cff-82b8-cdb78636802e"}]},"status":0}`

// debugRuntimeStub is a /debugger/ endpoint that records each request and
// answers through reply (action, params) -> body.
type debugRuntimeStub struct {
	*httptest.Server
	actions []string
	params  []map[string]any
}

func newDebugRuntimeStub(t *testing.T, reply func(action string, params map[string]any) string) *debugRuntimeStub {
	t.Helper()
	s := &debugRuntimeStub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.actions = append(s.actions, body.Action)
		s.params = append(s.params, body.Params)
		_, _ = w.Write([]byte(reply(body.Action, body.Params)))
	}))
	t.Cleanup(s.Close)
	return s
}

// runDebug runs `mxcli debug <args>` against the stub, with a cached session
// token beside a scratch project path, and returns what it printed.
func runDebug(t *testing.T, stub *debugRuntimeStub, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".mxcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mxcli", "debug-session.token"), []byte("tok-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	full := append([]string{"debug"}, args...)
	full = append(full, "--app-url", stub.URL, "-p", filepath.Join(dir, "app.mpr"))
	return captureStdout(t, func() error {
		rootCmd.SetArgs(full)
		return rootCmd.Execute()
	})
}

// `debug continue` resumes the one paused flow by its debug_id. Sent without
// one, the runtime refuses it ("Missing property: debug_id") and the flow
// stays paused, which printed "Continued." (ako/mxcli#1782).
func TestDebugContinue_ResumesThePausedFlowByDebugID(t *testing.T) {
	stub := newDebugRuntimeStub(t, func(action string, params map[string]any) string {
		switch action {
		case "get_paused_microflows":
			return pausedACTDebug
		case "poll_events":
			return `{"result":{"events":[]},"status":0}`
		case "continue":
			if _, ok := params["debug_id"]; !ok {
				return `{"result":{},"message":"Missing property: debug_id","status":2}`
			}
		}
		return `{"result":{},"status":0}`
	})
	out, err := runDebug(t, stub, "continue")
	if err != nil {
		t.Fatalf("debug continue: %v\n%s", err, out)
	}
	last := len(stub.actions) - 1
	if stub.actions[last] != "continue" || stub.params[last]["debug_id"] != "dbg-1" {
		t.Errorf("last request = %s %v, want continue {debug_id: dbg-1}", stub.actions[last], stub.params[last])
	}
	if !strings.Contains(out, "dbg-1") {
		t.Errorf("output = %q, want it to name the flow it continued", out)
	}
}
