// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// debugger.go is the client of the Mendix microflow debugger (see
// docs/11-proposals/PROPOSAL_microflow_debugger.md). It speaks two planes: the
// M2EE admin plane toggles the debugger on/off and reports status, and the
// app's /debugger/ endpoint runs the session — breakpoints, paused-flow
// inspection, stepping and continue.
//
// Two APIs, two auth schemes:
//
//	admin plane   POST http://<host>:<adminPort>/   X-M2EE-Authentication: base64(adminPass)
//	              actions: enable_debugger {password}, disable_debugger, get_debugger_status
//	debugger plane POST <appURL>/debugger/          X-Debugger-Authentication: base64(debugPass)
//	              body {action, session_token, params}; params is mandatory even when {}

// DebuggerOptions configures a DebuggerClient.
type DebuggerOptions struct {
	// Admin is the M2EE admin connection (Host/Port/Token/Direct) used for
	// enable/disable/status. For a `run --local` runtime: 127.0.0.1:8090, token
	// "mxcli-local-dev", Direct: true.
	Admin M2EEOptions
	// AppURL is the app base URL the /debugger/ endpoint lives under
	// (e.g. http://127.0.0.1:8080).
	AppURL string
	// DebugPass is the debugger password. It is passed to enable_debugger and
	// used as the X-Debugger-Authentication credential; the two must match.
	DebugPass string
	// TokenPath, when set, caches the session token across invocations (the CLI
	// is one-shot per command). Empty keeps the token in memory only.
	TokenPath string
	// Timeout bounds a single debugger HTTP request (default 30s). It must stay
	// above poll_events' long poll, which the runtime holds for about 25 s when
	// nothing is pending: a poll the client abandons loses the event delivered
	// into it (measured for ako/mxcli#1782; see ako/mxcli#1812).
	Timeout time.Duration
}

// DebuggerClient drives both planes of the runtime debugger.
type DebuggerClient struct {
	opts  DebuggerOptions
	http  *http.Client
	token string
}

// DebuggerStatus mirrors the get_debugger_status feedback.
type DebuggerStatus struct {
	Enabled                  bool `json:"enabled"`
	ClientConnected          bool `json:"client_connected"`
	NumberOfPausedMicroflows int  `json:"number_of_paused_microflows"`
}

// NewDebuggerClient returns a client with defaults applied.
func NewDebuggerClient(opts DebuggerOptions) *DebuggerClient {
	if opts.AppURL == "" {
		opts.AppURL = "http://127.0.0.1:8080"
	}
	if opts.DebugPass == "" {
		opts.DebugPass = "mxdebug"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	return &DebuggerClient{opts: opts, http: &http.Client{Timeout: opts.Timeout}}
}

// Token returns the current in-memory session token (may be empty).
func (c *DebuggerClient) Token() string { return c.token }

// Status returns the debugger state via the admin plane.
func (c *DebuggerClient) Status() (*DebuggerStatus, error) {
	resp, err := CallM2EE(c.opts.Admin, "get_debugger_status", nil)
	if err != nil {
		return nil, err
	}
	if msg := resp.M2EEError(); msg != "" {
		return nil, fmt.Errorf("get_debugger_status: %s", msg)
	}
	var st DebuggerStatus
	if len(resp.RawFeedback) > 0 {
		if err := json.Unmarshal(resp.RawFeedback, &st); err != nil {
			return nil, fmt.Errorf("decoding debugger status: %w", err)
		}
	}
	return &st, nil
}

// Enable turns the debugger on (admin plane). The runtime requires the password
// here; the same value must be used as the debugger-endpoint credential.
func (c *DebuggerClient) Enable() error {
	resp, err := CallM2EE(c.opts.Admin, "enable_debugger", map[string]any{"password": c.opts.DebugPass})
	if err != nil {
		return err
	}
	if msg := resp.M2EEError(); msg != "" {
		return fmt.Errorf("enable_debugger: %s", msg)
	}
	return nil
}

// Disable turns the debugger off (admin plane) and clears the cached session
// token, so a stale token can't be reused against a fresh session.
func (c *DebuggerClient) Disable() error {
	resp, err := CallM2EE(c.opts.Admin, "disable_debugger", nil)
	if err != nil {
		return err
	}
	if msg := resp.M2EEError(); msg != "" {
		return fmt.Errorf("disable_debugger: %s", msg)
	}
	c.clearToken()
	return nil
}

// StartSession opens a debug session on the /debugger/ endpoint and caches the
// returned token. It is the only debugger-plane call that carries no token
// (it mints one).
func (c *DebuggerClient) StartSession() (string, error) {
	result, err := c.post("start_session", false, map[string]any{"breakpoints": []any{}})
	if err != nil {
		return "", err
	}
	var r struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(result, &r); err != nil || r.SessionToken == "" {
		return "", fmt.Errorf("start_session returned no session_token (response: %s)", string(result))
	}
	c.token = r.SessionToken
	if err := c.saveToken(r.SessionToken); err != nil {
		return "", fmt.Errorf("caching session token: %w", err)
	}
	return r.SessionToken, nil
}

// AddBreakpoint sets a breakpoint on an activity (object_id) of a microflow or
// nanoflow. A non-empty condition is a Mendix expression that gates the pause.
// Requires an active session (run 'mxcli debug enable' first).
//
// A nanoflow breakpoint uses the nanoflow_name param, not microflow_name — the
// runtime NPEs on the wrong key (findings — nanoflow debugging).
//
// condition is always sent, as "" for an unconditional breakpoint: the runtime
// (11.13 and 11.15, measured) answers a request without it with
// {"message":"Missing property: condition","status":2} and registers nothing
// (ako/mxcli#1782).
func (c *DebuggerClient) AddBreakpoint(flowName, objectID, condition string, nanoflow bool) error {
	nameKey := "microflow_name"
	if nanoflow {
		nameKey = "nanoflow_name"
	}
	params := map[string]any{
		nameKey:     flowName,
		"object_id": objectID,
		"condition": condition,
	}
	_, err := c.post("add_breakpoint", true, params)
	return err
}

// PollEvents returns the runtime's pending debugger events. A paused NANOFLOW
// appears here (as a paused_microflow event) but NOT in PausedMicroflows.
func (c *DebuggerClient) PollEvents() (json.RawMessage, error) {
	return c.post("poll_events", true, nil)
}

// RemoveBreakpoint clears the breakpoint on an activity (object_id).
func (c *DebuggerClient) RemoveBreakpoint(objectID string) error {
	_, err := c.post("remove_breakpoint", true, map[string]any{"object_id": objectID})
	return err
}

// PausedMicroflows returns the runtime's paused-microflow state (each paused
// flow, its current activity, and in-scope variables) as the raw result object.
func (c *DebuggerClient) PausedMicroflows() (json.RawMessage, error) {
	return c.post("get_paused_microflows", true, nil)
}

// GetObject inspects one variable of a paused microflow (by its debug_id).
func (c *DebuggerClient) GetObject(debugID, variableName string) (json.RawMessage, error) {
	return c.post("get_object", true, map[string]any{
		"debug_id":      debugID,
		"variable_name": variableName,
	})
}

// GetList inspects a LIST variable of a paused flow (by its debug_id). Use this
// instead of GetObject when the variable is a list of objects.
func (c *DebuggerClient) GetList(debugID, variableName string) (json.RawMessage, error) {
	return c.post("get_list", true, map[string]any{
		"debug_id":      debugID,
		"variable_name": variableName,
	})
}

// Step advances a paused microflow one step. kind is "over", "into", or "out".
func (c *DebuggerClient) Step(kind, debugID string) error {
	var action string
	switch kind {
	case "over":
		action = "step_over"
	case "into":
		action = "step_into"
	case "out":
		action = "step_out"
	default:
		return fmt.Errorf("unknown step %q (want over|into|out)", kind)
	}
	_, err := c.post(action, true, map[string]any{"debug_id": debugID})
	return err
}

// Continue resumes one paused flow (by its debug_id). The runtime requires the
// debug_id: continue without it is refused with "Missing property: debug_id"
// and the flow stays paused (11.13 and 11.15, measured — ako/mxcli#1782).
func (c *DebuggerClient) Continue(debugID string) error {
	if debugID == "" {
		return fmt.Errorf("continue needs the paused flow's debug_id (see 'mxcli debug paused')")
	}
	_, err := c.post("continue", true, map[string]any{"debug_id": debugID})
	return err
}

// ContinueAll resumes every paused flow.
func (c *DebuggerClient) ContinueAll() error {
	_, err := c.post("continue_all", true, nil)
	return err
}

// LoadToken reads a previously cached session token into memory (best-effort:
// a missing file is not an error, since the session may not exist yet).
func (c *DebuggerClient) LoadToken() error {
	if c.opts.TokenPath == "" {
		return nil
	}
	b, err := os.ReadFile(c.opts.TokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	c.token = strings.TrimSpace(string(b))
	return nil
}

func (c *DebuggerClient) saveToken(token string) error {
	if c.opts.TokenPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.opts.TokenPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.opts.TokenPath, []byte(token), 0o600)
}

func (c *DebuggerClient) clearToken() {
	c.token = ""
	if c.opts.TokenPath != "" {
		_ = os.Remove(c.opts.TokenPath)
	}
}

// post drives the /debugger/ plane. The envelope is {action, session_token?,
// params}; params is always serialized (the runtime rejects a missing params
// with "Missing property"). Returns the raw "result" object from the response,
// or a *DebuggerError when the reply's status says the runtime refused it.
func (c *DebuggerClient) post(action string, withToken bool, params map[string]any) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	body := map[string]any{"action": action, "params": params}
	if withToken {
		if c.token == "" {
			return nil, fmt.Errorf("no debug session — run 'mxcli debug enable' first")
		}
		body["session_token"] = c.token
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling debugger request: %w", err)
	}

	url := strings.TrimRight(c.opts.AppURL, "/") + "/debugger/"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating debugger request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The debugger endpoint accepts ONLY this header form — raw password, Basic,
	// and Bearer all 401 (verified against a running runtime).
	req.Header.Set("X-Debugger-Authentication", m2eeAuthHeader(c.opts.DebugPass))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach debugger endpoint at %s -- is the app running (mxcli run --local)?", url)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("debugger auth failed (HTTP %d) -- is the debugger enabled ('mxcli debug enable') and --debug-pass correct?", resp.StatusCode)
	default:
		return nil, fmt.Errorf("debugger endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var env struct {
		Result  json.RawMessage `json:"result"`
		Status  *int            `json:"status"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(respBody, &env); err != nil {
		return nil, fmt.Errorf("decoding debugger response: %w (body: %s)", err, strings.TrimSpace(string(respBody)))
	}
	// The runtime answers a refusal with HTTP 200 too; only the envelope says so.
	// A missing status is not a refusal (see DebuggerError).
	if env.Status != nil && *env.Status != 0 {
		return nil, &DebuggerError{Action: action, Status: *env.Status, Message: env.Message}
	}
	return env.Result, nil
}

// DebuggerError is the runtime refusing a /debugger/ request. The endpoint
// answers HTTP 200 either way, so only the reply envelope tells them apart:
// success is {"result":…,"status":0}, a refusal {"result":{},"message":…,
// "status":2} — every reply measured on Mendix 11.13 and 11.15 carries a status,
// and every refusal seen was 2. Any non-zero status is treated as a refusal; a
// reply with no status at all is not, so a runtime that leaves it out still
// works. Ignoring the envelope is how `debug break` printed "Breakpoint set"
// over "Missing property: condition" (ako/mxcli#1782).
type DebuggerError struct {
	Action  string // the debugger action that was refused
	Status  int    // the envelope status (non-zero)
	Message string // the runtime's reason, verbatim; may be empty
}

func (e *DebuggerError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "no message"
	}
	s := fmt.Sprintf("the runtime refused %s (status %d): %s", e.Action, e.Status, msg)
	// A token the runtime does not know: it restarted, or another debug session
	// started since (one session at a time — starting one invalidates the last).
	if strings.Contains(e.Message, "session token") {
		s += " -- the debug session is gone (the runtime restarted, or another debugger started a session); run 'mxcli debug enable' for a new one"
	}
	return s
}
