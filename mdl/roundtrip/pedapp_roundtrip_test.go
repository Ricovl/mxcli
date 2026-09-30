// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// fixture is one Studio Pro-authored project the harness round-trips, with its
// own allowlist. Each allowlist may only shrink.
type fixture struct {
	name string // for messages
	dir  string // the project directory, relative to this package
	mpr  string // the .mpr file name inside dir
	// minDocs is the fewest documents the enumeration may find; far fewer means
	// the enumeration broke, and a harness that looks at nothing passes.
	minDocs       int
	knownFailures map[string]knownFailure
	// optional fixtures (a git submodule) skip with this message when absent
	// instead of failing.
	skipIfMissing string
}

// pedApp is PedApp (Mendix 11.13), authored in Studio Pro from marketplace
// modules. See testdata/pedapp/README.md for what it contains and why it must
// stay pristine. It is committed, so it is never skipped.
var pedApp = fixture{
	name:          "PedApp",
	dir:           "../../testdata/pedapp",
	mpr:           "PedApp.mpr",
	minDocs:       100,
	knownFailures: knownFailures,
}

// testApp is ako/TestApp, a Studio Pro-authored project pinned as the git
// submodule testdata/testapp. It has what PedApp lacks: workflows, OData
// clients and services, and external entities (#743).
var testApp = fixture{
	name:          "TestApp",
	dir:           "../../testdata/testapp/TestApp",
	mpr:           "TestApp.mpr",
	minDocs:       100,
	knownFailures: testAppKnownFailures,
	skipIfMissing: "the TestApp fixture is a git submodule that is not initialised; run `git submodule update --init testdata/testapp` to round-trip it",
}

// law names one way a document can fail the round trip. A document's outcome is
// the set of laws it breaks; an empty set is a pass.
type law string

const (
	// lawDescribe: `describe` itself errored or printed nothing.
	lawDescribe law = "describe"
	// lawParse: the describe output does not parse as MDL.
	lawParse law = "parse"
	// lawExec: the describe output parses but executing it fails. Anything it
	// wrote before failing is still judged by GetPut.
	lawExec law = "exec"
	// lawGetPut: executing the describe output wrote something.
	lawGetPut law = "getput"
	// lawPutGet: describing again returns something other than the first
	// describe.
	lawPutGet law = "putget"
)

// knownFailure is one entry of the allowlist: the laws a document is known to
// break today and the issue that tracks it.
type knownFailure struct {
	laws  []law
	issue string
	why   string
}

// knownFailures is the allowlist; it lives in allowlist_test.go.

// ---------------------------------------------------------------------------
// The test
// ---------------------------------------------------------------------------

func TestPedAppRoundTrip(t *testing.T) { runRoundTrip(t, pedApp) }

// TestTestAppRoundTrip round-trips ako/TestApp (the testdata/testapp
// submodule). It skips, saying so, when the submodule is not initialised.
func TestTestAppRoundTrip(t *testing.T) { runRoundTrip(t, testApp) }

func runRoundTrip(t *testing.T, fx fixture) {
	h := newFixtureHarness(t, fx)
	defer h.close()

	docs := h.documents()
	if len(docs) < fx.minDocs {
		t.Fatalf("enumerated only %d documents from %s — the enumeration is broken", len(docs), fx.name)
	}

	seen := map[string]bool{}
	ran, passed := 0, 0
	for _, d := range docs {
		key := d.key()
		seen[key] = true
		t.Run(key, func(t *testing.T) {
			got := h.roundTrip(d)
			ran++
			if len(got.broken) == 0 {
				passed++
			}
			judge(t, fx.knownFailures, key, got)
		})
	}

	// Positive control for the harness itself: if nothing passes, "everything is
	// lossy" and "the comparison never ran" look the same.
	if ran == len(docs) && passed == 0 {
		t.Errorf("no document round-tripped — the harness cannot pass, so it proves nothing")
	}
	t.Logf("%d of %d documents round-trip", passed, ran)

	for key := range fx.knownFailures {
		if !seen[key] {
			t.Errorf("the %s allowlist lists %q, which the fixture does not contain — stale entry, remove it", fx.name, key)
		}
	}
}

// TestPedAppRoundTrip_EditIsWritten is the control CLAUDE.md requires of every
// "nothing changed" assertion: a deliberately edited statement run through the
// same pipeline MUST register as a write. Without it, a snapshot that never saw
// a change would pass the GetPut law for every document.
func TestPedAppRoundTrip_EditIsWritten(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "enumeration WebActions.PictureQuality"
	described, err := h.describe(target)
	if err != nil || strings.TrimSpace(described) == "" {
		t.Fatalf("describe %s: %v (output %q)", target, err, described)
	}

	// Control 1: unchanged, it writes nothing (this enumeration round-trips).
	before := h.snapshot()
	if err := h.exec(described); err != nil {
		t.Fatalf("exec unchanged describe output: %v\n%s", err, described)
	}
	if changed := before.diff(h.snapshot()); len(changed) != 0 {
		t.Fatalf("the unchanged statement wrote %d unit(s); the control needs a document that round-trips", len(changed))
	}

	// Control 2: the same statement with one edit MUST write.
	edited := strings.Replace(described, "\n);", ",\n  RoundTripControlValue 'Round-trip control'\n);", 1)
	if edited == described {
		t.Fatalf("could not edit the describe output — its shape changed:\n%s", described)
	}
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec edited statement: %v\n%s", err, edited)
	}
	changed := before.diff(h.snapshot())
	if len(changed) == 0 {
		t.Fatalf("an edited statement wrote nothing — the snapshot comparison cannot see writes, so every GetPut pass is meaningless")
	}
	after, err := h.describe(target)
	if err != nil {
		t.Fatalf("describe after edit: %v", err)
	}
	if !strings.Contains(after, "RoundTripControlValue") {
		t.Fatalf("the edit was written but describe does not show it:\n%s", after)
	}
}

// TestPedAppRoundTrip_MoveIsWritten is the control for the other half of a
// unit: in an MPR v2 project a document's folder lives in the .mpr Unit table
// (ContainerID), not in its mprcontents file, so a statement that only moves a
// document rewrites no unit bytes. A snapshot that compared unit contents alone
// would call the move "nothing written", and a describe that dropped a folder
// clause would pass GetPut while moving every document it touched.
func TestPedAppRoundTrip_MoveIsWritten(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "constant FeedbackModule.ClientIdentifier"
	described, err := h.describe(target)
	if err != nil || strings.TrimSpace(described) == "" {
		t.Fatalf("describe %s: %v (output %q)", target, err, described)
	}
	const from, to = "folder 'Private/Resources/Constants'", "folder 'Private'"
	moved := strings.Replace(described, from, to, 1)
	if moved == described {
		t.Fatalf("describe output has no %s clause — its shape changed:\n%s", from, described)
	}

	before := h.snapshot()
	if err := h.exec(moved); err != nil {
		t.Fatalf("exec moved statement: %v\n%s", err, moved)
	}
	changed := before.diff(h.snapshot())
	if len(changed) == 0 {
		t.Fatalf("moving %s to another folder registered as nothing written — the snapshot cannot see a unit's container, so a describe that drops a folder passes GetPut", target)
	}
	t.Logf("move registered as: %s", strings.Join(changed, "; "))
}

// judge compares one document's outcome with the allowlist.
func judge(t *testing.T, allow map[string]knownFailure, key string, got outcome) {
	t.Helper()
	want, listed := allow[key]
	wantSet := map[law]bool{}
	for _, l := range want.laws {
		wantSet[l] = true
	}
	gotSet := map[law]bool{}
	for _, l := range got.broken {
		gotSet[l] = true
	}

	var regressed, fixed []string
	for l := range gotSet {
		if !wantSet[l] {
			regressed = append(regressed, string(l))
		}
	}
	for l := range wantSet {
		if !gotSet[l] {
			fixed = append(fixed, string(l))
		}
	}
	sort.Strings(regressed)
	sort.Strings(fixed)

	if len(regressed) > 0 {
		t.Errorf("%s breaks %s, which the allowlist does not expect.\n%s",
			key, strings.Join(regressed, ", "), got.detail)
	}
	if len(fixed) > 0 {
		t.Errorf("%s no longer breaks %s — strike it from knownFailures (%s: %s). The allowlist may only shrink.",
			key, strings.Join(fixed, ", "), want.issue, want.why)
	}
	if listed && len(regressed) == 0 && len(fixed) == 0 {
		t.Logf("known failure (%s): %s", want.issue, want.why)
	}
	if got.refused != "" {
		t.Logf("refused, nothing written: %s", got.refused)
	}
	t.Logf("outcome %s %v", key, got.broken)
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type harness struct {
	t       *testing.T
	fx      fixture
	fixture map[string][]byte // the committed fixture, by path relative to its root
	dir     string            // the working copy
	mpr     string
	exe     *executor.Executor
	out     *bytes.Buffer
	orig    snapshot // the fixture as committed
}

type outcome struct {
	broken  []law
	detail  string
	refused string // a deliberate refusal that wrote nothing; not a failure
}

// refusals are exec errors that are deliberate guards rather than failures:
// the statement is refused as a whole, and nothing is written. Each pattern
// must name a refusal that is right for the document, not merely a known bug —
// an "already exists" from describe printing a plain `create` is a bug (#705).
var refusals = []string{
	// Writing into a marketplace module is overwritten by the next module
	// update; the executor refuses rather than write it.
	"is a marketplace module",
}

func isRefusal(err error) bool {
	for _, r := range refusals {
		if strings.Contains(err.Error(), r) {
			return true
		}
	}
	return false
}

// newHarness is a harness on PedApp, the committed fixture.
func newHarness(t *testing.T) *harness { return newFixtureHarness(t, pedApp) }

func newFixtureHarness(t *testing.T, fx fixture) *harness {
	t.Helper()
	src, err := filepath.Abs(fx.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, fx.mpr)); err != nil {
		if fx.skipIfMissing != "" {
			t.Skipf("%s: %s (%v)", fx.name, fx.skipIfMissing, err)
		}
		t.Fatalf("fixture missing: %v", err)
	}
	h := &harness{t: t, fx: fx, dir: t.TempDir(), fixture: readFixture(t, src, fx.mpr)}
	h.mpr = filepath.Join(h.dir, fx.mpr)
	// Widget packages (in PedApp stripped to their XML definitions, see its
	// README) are all a page build needs, and are copied once since no round
	// trip writes them.
	if err := copyDir(filepath.Join(src, "widgets"), filepath.Join(h.dir, "widgets")); err != nil {
		t.Fatalf("copy widgets: %v", err)
	}
	if _, err := executor.RefreshWidgetDefinitions(h.mpr, false, nil); err != nil {
		t.Fatalf("generate widget definitions: %v", err)
	}
	h.restore()
	h.orig = h.snapshot()
	// Precondition: connecting must not itself write, or every comparison
	// below would be against a copy that is no longer the fixture.
	for rel, want := range h.fixture {
		if rel == fx.mpr {
			continue // SQLite metadata; units are compared through the reader
		}
		if got, err := os.ReadFile(filepath.Join(h.dir, rel)); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("connecting to the fixture copy changed %s", rel)
		}
	}
	return h
}

// restore (re)creates the working copy from the committed fixture and connects
// a fresh executor to it. Each document is judged against the fixture as
// committed, so one document's write must not leak into the next.
func (h *harness) restore() {
	h.t.Helper()
	h.close()
	if err := syncTree(h.fixture, h.dir); err != nil {
		h.t.Fatalf("restore working copy: %v", err)
	}
	h.out = &bytes.Buffer{}
	h.exe = executor.New(h.out)
	h.exe.SetQuiet(true)
	h.exe.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := h.exe.Execute(&ast.ConnectStmt{Path: h.mpr}); err != nil {
		h.t.Fatalf("connect to fixture copy: %v", err)
	}
}

func (h *harness) close() {
	if h.exe != nil {
		_ = h.exe.Execute(&ast.DisconnectStmt{})
		_ = h.exe.Close()
		h.exe = nil
	}
}

func (h *harness) describe(target string) (string, error) {
	h.out.Reset()
	prog, errs := visitor.Build("describe " + target + ";")
	if len(errs) > 0 {
		return "", errs[0]
	}
	for _, stmt := range prog.Statements {
		if err := h.exe.Execute(stmt); err != nil {
			return h.out.String(), err
		}
	}
	return h.out.String(), nil
}

// exec runs a script the way `mxcli exec` does after its preflight.
func (h *harness) exec(script string) error {
	h.out.Reset()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		return fmt.Errorf("parse: %w", errs[0])
	}
	return h.exe.ExecuteProgram(prog)
}

// roundTrip runs describe -> exec -> describe for one document and reports the
// laws it breaks. The working copy is restored afterwards if anything was
// written.
func (h *harness) roundTrip(d document) outcome { return h.roundTripWith(d, nil) }

// roundTripWith is roundTrip with the describe output passed through rewrite
// before it is executed; PutGet still compares against the unrewritten first
// describe. A nil rewrite executes the output unchanged.
func (h *harness) roundTripWith(d document, rewrite func(string) string) outcome {
	var o outcome
	fail := func(l law, format string, args ...any) {
		o.broken = append(o.broken, l)
		o.detail += fmt.Sprintf("[%s] ", l) + fmt.Sprintf(format, args...) + "\n"
	}

	first, err := h.describe(d.target())
	if err != nil || strings.TrimSpace(first) == "" {
		fail(lawDescribe, "describe %s: err=%v output=%q", d.target(), err, first)
		return o
	}

	script := first
	if rewrite != nil {
		script = rewrite(first)
	}
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		fail(lawParse, "%v\n--- executed script ---\n%s", errs[0], script)
		return o
	}
	if len(prog.Statements) == 0 {
		// Comments only: nothing to execute, so nothing can be written. Not a
		// failure of either law, but not evidence either.
		return o
	}

	execErr := h.exe.ExecuteProgram(prog)
	changed := h.orig.diff(h.snapshot())
	switch {
	case execErr == nil:
	case len(changed) == 0 && isRefusal(execErr):
		// ADR-0012: content MDL cannot safely write is refused, never dropped.
		// A refusal that wrote nothing keeps both laws.
		o.refused = execErr.Error()
	default:
		fail(lawExec, "%v\n--- executed script ---\n%s", execErr, script)
	}

	if len(changed) > 0 {
		fail(lawGetPut, "executing the describe output wrote %d unit(s):\n  %s\n--- executed script ---\n%s",
			len(changed), strings.Join(changed, "\n  "), script)
	}

	// PutGet. With no write this also catches a describe that is not
	// deterministic, which a diff-based workflow cannot tolerate either.
	second, err := h.describe(d.target())
	if err != nil || second != first {
		fail(lawPutGet, "second describe differs from the first (err=%v):\n%s", err, lineDiff(first, second))
	}

	// A failed statement can leave executor state behind even when it wrote
	// nothing, so reconnect after either.
	if len(changed) > 0 || execErr != nil {
		h.restore()
	}
	return o
}

// ---------------------------------------------------------------------------
// Snapshots: every unit's raw bytes, keyed by unit ID
// ---------------------------------------------------------------------------

type snapshot struct {
	units map[string][]byte // unit ID -> raw BSON
	label map[string]string // unit ID -> "$Type Name", for messages
	// place is where each unit sits: its container and containment name. In
	// MPR v2 that is a row in the .mpr Unit table, not part of the unit's
	// mprcontents file, so a statement that only moves a document to another
	// folder rewrites no unit bytes and is visible here alone.
	place map[string]string
	// files are the working copy's files outside the model: javasource/,
	// javascriptsource/, themesource/ and anything else a statement might write
	// next to the project. The fixture has none, so any file here is a write.
	files map[string][]byte
}

func (h *harness) snapshot() snapshot {
	h.t.Helper()
	r, err := mmpr.Open(h.mpr)
	if err != nil {
		h.t.Fatalf("open working copy: %v", err)
	}
	defer r.Close()
	units, err := r.ListUnits()
	if err != nil {
		h.t.Fatalf("list units: %v", err)
	}
	s := snapshot{units: map[string][]byte{}, label: map[string]string{}, place: map[string]string{}}
	for _, u := range units {
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			h.t.Fatalf("read unit %s: %v", u.ID, err)
		}
		s.units[u.ID] = b
		s.place[u.ID] = u.ContainerID + "/" + u.ContainmentName
		typ, name := typeAndName(b)
		s.label[u.ID] = strings.TrimSpace(typ + " " + name)
	}
	s.files = map[string][]byte{}
	err = filepath.Walk(h.dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(h.dir, p)
		top := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		switch {
		case top == "mprcontents" || top == "widgets" || top == ".mxcli" || strings.HasPrefix(top, h.fx.mpr):
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case info.IsDir():
			return nil
		}
		b, err := os.ReadFile(p)
		s.files[rel] = b
		return err
	})
	if err != nil {
		h.t.Fatalf("scan working copy: %v", err)
	}
	return s
}

// diff lists the units that differ between two snapshots. A unit whose bytes
// differ but whose canonical form is equal is still a write (GetPut says
// nothing is written, and ADR-0008 elides such writes), so it is reported, and
// marked as $ID churn so it is not mistaken for data loss.
func (s snapshot) diff(o snapshot) []string {
	var out []string
	for id, a := range s.units {
		b, ok := o.units[id]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("removed %s (%s)", id, s.label[id]))
		case s.place[id] != o.place[id]:
			out = append(out, fmt.Sprintf("moved %s (%s): container %s -> %s", id, s.label[id], s.place[id], o.place[id]))
			if !bytes.Equal(a, b) {
				out = append(out, fmt.Sprintf("rewrote %s (%s) while moving it", id, s.label[id]))
			}
		case !bytes.Equal(a, b):
			kind := "content changed"
			if eq, err := canon.Equal(a, b); err == nil && eq {
				kind = "$ID churn only (canonically equal)"
			} else if paths := bsonDiff(a, b); len(paths) > 0 {
				kind += ":\n      " + strings.Join(paths, "\n      ")
			}
			out = append(out, fmt.Sprintf("rewrote %s (%s): %s", id, s.label[id], kind))
		}
	}
	for id := range o.units {
		if _, ok := s.units[id]; !ok {
			out = append(out, fmt.Sprintf("added %s (%s)", id, o.label[id]))
		}
	}
	for rel, b := range o.files {
		if a, ok := s.files[rel]; !ok {
			out = append(out, fmt.Sprintf("wrote file %s (%d bytes)", rel, len(b)))
		} else if !bytes.Equal(a, b) {
			out = append(out, fmt.Sprintf("rewrote file %s", rel))
		}
	}
	for rel := range s.files {
		if _, ok := o.files[rel]; !ok {
			out = append(out, fmt.Sprintf("deleted file %s", rel))
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Enumeration: every describable document in the fixture
// ---------------------------------------------------------------------------

type document struct {
	keyword string // describe keyword, e.g. "java action"
	name    string // qualified name, or a quoted name for user roles
}

func (d document) target() string { return d.keyword + " " + d.name }
func (d document) key() string    { return d.target() }

// unitKeywords maps a document unit's storage type to its describe keyword.
// A type that is not here is not covered; TestPedAppRoundTrip logs which
// fixture types that leaves out, so the gap is visible rather than silent.
var unitKeywords = map[string]string{
	"Microflows$Microflow":                 "microflow",
	"Microflows$Nanoflow":                  "nanoflow",
	"Microflows$Rule":                      "rule",
	"Forms$Page":                           "page",
	"Forms$Snippet":                        "snippet",
	"Forms$Layout":                         "layout",
	"Forms$BuildingBlock":                  "building block",
	"Enumerations$Enumeration":             "enumeration",
	"Constants$Constant":                   "constant",
	"JavaActions$JavaAction":               "java action",
	"JavaScriptActions$JavaScriptAction":   "javascript action",
	"Workflows$Workflow":                   "workflow",
	"Rest$ConsumedODataService":            "odata client",
	"ODataPublish$PublishedODataService2":  "odata service",
	"Menus$MenuDocument":                   "menu",
	"Images$ImageCollection":               "image collection",
	"JsonStructures$JsonStructure":         "json structure",
	"ImportMappings$ImportMapping":         "import mapping",
	"ExportMappings$ExportMapping":         "export mapping",
	"Rest$ConsumedRestService":             "rest client",
	"Rest$PublishedRestService":            "published rest service",
	"ScheduledEvents$ScheduledEvent":       "scheduled event",
	"RegularExpressions$RegularExpression": "regular expression",
}

func (h *harness) documents() []document {
	h.t.Helper()
	r, err := mmpr.Open(h.mpr)
	if err != nil {
		h.t.Fatalf("open fixture copy: %v", err)
	}
	defer r.Close()

	units, err := r.ListUnits()
	if err != nil {
		h.t.Fatalf("list units: %v", err)
	}
	parent := map[string]string{}
	for _, u := range units {
		parent[u.ID] = u.ContainerID
	}
	modules := map[string]string{} // module unit ID -> name
	for _, u := range units {
		b, _ := r.GetRawUnitBytes(u.ID)
		if typ, name := typeAndName(b); typ == "Projects$ModuleImpl" {
			modules[u.ID] = name
		}
	}
	moduleOf := func(id string) string {
		for range 32 {
			if name, ok := modules[id]; ok {
				return name
			}
			p, ok := parent[id]
			if !ok || p == id {
				return ""
			}
			id = p
		}
		return ""
	}

	var docs []document
	uncovered := map[string]int{}
	for _, u := range units {
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			h.t.Fatalf("read unit %s: %v", u.ID, err)
		}
		var doc bson.D
		if err := bson.Unmarshal(b, &doc); err != nil {
			h.t.Fatalf("decode unit %s: %v", u.ID, err)
		}
		typ, name := typeAndName(b)
		mod := moduleOf(u.ID)
		switch typ {
		case "DomainModels$DomainModel":
			for _, n := range childNames(doc, "Entities") {
				docs = append(docs, document{"entity", mod + "." + n})
			}
			// An external entity is also described on its own, by `describe
			// external entity`, which prints the `from odata client` form.
			for _, n := range externalEntityNames(doc) {
				docs = append(docs, document{"external entity", mod + "." + n})
			}
			for _, n := range childNames(doc, "Associations") {
				docs = append(docs, document{"association", mod + "." + n})
			}
			for _, n := range childNames(doc, "CrossAssociations") {
				docs = append(docs, document{"association", mod + "." + n})
			}
		case "Security$ModuleSecurity":
			for _, n := range childNames(doc, "ModuleRoles") {
				docs = append(docs, document{"module role", mod + "." + n})
			}
		case "Security$ProjectSecurity":
			for _, n := range childNames(doc, "UserRoles") {
				docs = append(docs, document{"user role", "'" + n + "'"})
			}
		default:
			if kw, ok := unitKeywords[typ]; ok {
				docs = append(docs, document{kw, mod + "." + name})
			} else {
				uncovered[typ]++
			}
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].key() < docs[j].key() })

	var gaps []string
	for typ, n := range uncovered {
		gaps = append(gaps, fmt.Sprintf("%s (%d)", typ, n))
	}
	sort.Strings(gaps)
	h.t.Logf("unit types not round-tripped (no describe mapping): %s", strings.Join(gaps, ", "))
	return docs
}

func typeAndName(b []byte) (string, string) {
	var doc bson.D
	if err := bson.Unmarshal(b, &doc); err != nil {
		return "", ""
	}
	var typ, name string
	for _, e := range doc {
		switch e.Key {
		case "$Type":
			typ, _ = e.Value.(string)
		case "Name":
			name, _ = e.Value.(string)
		}
	}
	return typ, name
}

// externalEntityNames returns the Name of every entity in a domain model whose
// Source is an OData remote entity source.
func externalEntityNames(doc bson.D) []string {
	var out []string
	for _, e := range doc {
		if e.Key != "Entities" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		for _, item := range arr {
			ent, ok := item.(bson.D)
			if !ok {
				continue
			}
			var name string
			external := false
			for _, f := range ent {
				switch f.Key {
				case "Name":
					name, _ = f.Value.(string)
				case "Source":
					if src, ok := f.Value.(bson.D); ok {
						for _, sf := range src {
							if sf.Key == "$Type" && sf.Value == "Rest$ODataRemoteEntitySource" {
								external = true
							}
						}
					}
				}
			}
			if external && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// childNames returns the Name of every element in a BSON array property. Mendix
// arrays carry a leading type-marker integer, which is skipped.
func childNames(doc bson.D, prop string) []string {
	var out []string
	for _, e := range doc {
		if e.Key != prop {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			return nil
		}
		for _, item := range arr {
			if d, ok := item.(bson.D); ok {
				for _, f := range d {
					if f.Key == "Name" {
						if s, ok := f.Value.(string); ok && s != "" {
							out = append(out, s)
						}
					}
				}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func lineDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	inB := map[string]int{}
	for _, l := range bl {
		inB[l]++
	}
	inA := map[string]int{}
	for _, l := range al {
		inA[l]++
	}
	var out []string
	for _, l := range al {
		if inB[l] == 0 {
			out = append(out, "- "+l)
		} else {
			inB[l]--
		}
	}
	for _, l := range bl {
		if inA[l] == 0 {
			out = append(out, "+ "+l)
		} else {
			inA[l]--
		}
	}
	if len(out) > 40 {
		out = append(out[:40], fmt.Sprintf("... (%d more lines)", len(out)-40))
	}
	return strings.Join(out, "\n")
}

// readFixture loads the .mpr and mprcontents/ into memory once, so restoring
// the working copy never reads the (possibly slow) checkout again.
func readFixture(t *testing.T, src, mprName string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != mprName && !strings.HasPrefix(rel, "mprcontents"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(p)
		out[rel] = b
		return err
	})
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return out
}

// syncTree makes dst's .mpr and mprcontents/ equal to the fixture, rewriting
// only the files that differ and deleting the ones the fixture does not have.
func syncTree(fixture map[string][]byte, dst string) error {
	for rel, want := range fixture {
		p := filepath.Join(dst, rel)
		if got, err := os.ReadFile(p); err == nil && bytes.Equal(got, want) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, want, 0o644); err != nil {
			return err
		}
	}
	return filepath.Walk(filepath.Join(dst, "mprcontents"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dst, p)
		if _, ok := fixture[rel]; !ok {
			return os.Remove(p)
		}
		return nil
	})
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// bsonDiff names the leaf properties that differ between two units, so a
// failure says WHAT was lost rather than only that the unit changed. Element
// $IDs are skipped: a rebuild renumbers them, and canon already judged that.
func bsonDiff(a, b []byte) []string {
	var da, db bson.D
	if bson.Unmarshal(a, &da) != nil || bson.Unmarshal(b, &db) != nil {
		return nil
	}
	var out []string
	walkDiff("", da, db, &out)
	const max = 15
	if len(out) > max {
		out = append(out[:max], fmt.Sprintf("... and %d more", len(out)-max))
	}
	return out
}

func walkDiff(path string, a, b any, out *[]string) {
	switch av := a.(type) {
	case bson.D:
		bv, ok := b.(bson.D)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
			return
		}
		bm := map[string]any{}
		for _, e := range bv {
			bm[e.Key] = e.Value
		}
		am := map[string]bool{}
		for _, e := range av {
			am[e.Key] = true
			if e.Key == "$ID" {
				continue
			}
			p := path + "/" + e.Key
			if v, ok := bm[e.Key]; ok {
				walkDiff(p, e.Value, v, out)
			} else {
				*out = append(*out, fmt.Sprintf("%s: removed (was %s)", p, short(e.Value)))
			}
		}
		for _, e := range bv {
			if !am[e.Key] {
				*out = append(*out, fmt.Sprintf("%s/%s: added %s", path, e.Key, short(e.Value)))
			}
		}
	case bson.A:
		bv, ok := b.(bson.A)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
			return
		}
		if len(av) != len(bv) {
			*out = append(*out, fmt.Sprintf("%s: %d -> %d items", path, len(av), len(bv)))
		}
		for i := 0; i < len(av) && i < len(bv); i++ {
			walkDiff(fmt.Sprintf("%s[%d]%s", path, i, elementName(av[i])), av[i], bv[i], out)
		}
	default:
		if ab, isBin := a.(bson.Binary); isBin {
			// Element references are renumbered with the $IDs, so they are
			// skipped. A GUID is not a reference: it is the database's identity
			// for the element, and a new one drops its table or column.
			if bb, ok := b.(bson.Binary); ok && strings.HasSuffix(path, "/GUID") && !bytes.Equal(ab.Data, bb.Data) {
				*out = append(*out, fmt.Sprintf("%s: GUID changed (the runtime reads this as a new element and drops its data)", path))
			}
			return
		}
		if fmt.Sprint(a) != fmt.Sprint(b) {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
		}
	}
}

func elementName(v any) string {
	if d, ok := v.(bson.D); ok {
		for _, e := range d {
			if e.Key == "Name" {
				if s, ok := e.Value.(string); ok && s != "" {
					return "(" + s + ")"
				}
			}
		}
	}
	return ""
}

func short(v any) string {
	r := []rune(fmt.Sprintf("%v", v))
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return string(r)
}
