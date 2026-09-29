// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// TestTestAppExportLevelSurvivesRoundTrip guards ako/mxcli#816, the per-document
// half of #801: a document whose ExportLevel is API must still be API after its
// own describe output is executed.
//
// The rewrite converters build a fresh gen document and set ExportLevel to a
// constant ("Hidden"), and a property the rebuild SETS beats the stored bytes.
// MDL has no spelling for most documents' export level, so describe cannot print
// it and the executed script cannot restore it: the rewrite silently removes the
// document from the module's API. Nothing reports it — a Hidden document is
// valid, the build is clean — until the module is exported as a package.
//
// THE SUBJECT HAS TO BE API. Every document in TestApp and PedApp is Hidden, the
// value the converters write, so on the fixture as committed the loss is
// invisible (this is the same trap as a GUID test on mxcli-authored content).
// Each subject is therefore set to API on the working copy first — the value a
// marketplace module's public documents carry — and then round-tripped.
//
// A document in a marketplace module is refused as a whole and proves nothing,
// so the next document of the kind is tried.
//
// CONTROL: TestTestAppExportLevelSurvivesRoundTrip_Control runs the same
// pipeline with an edit that must be written, and checks the unit was in fact
// rewritten — without it, an exec that never wrote would pass here.
func TestTestAppExportLevelSurvivesRoundTrip(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	byKind := map[string][]exportLevelSubject{}
	for _, s := range h.exportLevelSubjects() {
		byKind[s.keyword] = append(byKind[s.keyword], s)
	}
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	if len(kinds) < 10 {
		t.Fatalf("found only %d document kinds with an ExportLevel — the enumeration is broken", len(kinds))
	}

	for _, v := range exportLevelVariants {
		t.Run(v.name, func(t *testing.T) {
			for _, kind := range kinds {
				t.Run(kind, func(t *testing.T) {
					var tried []string
					for _, s := range byKind[kind] {
						got, after, measured, note := h.roundTripExportLevelUnit(t, s, v.rewrite)
						if measured && v.mustWrite && bytes.Equal(after, h.apiUnit(t, s.unit)) {
							measured, note = false, "the edit was not written"
						}
						if !measured {
							tried = append(tried, s.target()+": "+note)
							continue
						}
						if got != "API" {
							t.Errorf("%s: exec of the describe output of %s turned ExportLevel API into %q", v.name, s.target(), got)
						}
						t.Logf("%s: ExportLevel %s after the round trip", s.target(), got)
						return
					}
					t.Skipf("no %s in TestApp could be measured:\n  %s", kind, strings.Join(tried, "\n  "))
				})
			}
		})
	}
}

// exportLevelVariants are the two ways a stored document reaches its rewrite.
//
//   - described: the describe output, unchanged. Where the executor elides an
//     unchanged statement the document is never rebuilt, so this alone would
//     pass a converter that still writes the constant.
//   - edited: a documentation comment is prepended, which every kind with a
//     comment spelling must write, so the converter runs. A kind whose edit is
//     not written is skipped for this variant, saying so.
var exportLevelVariants = []struct {
	name      string
	rewrite   func(string) string
	mustWrite bool
}{
	{name: "described"},
	{name: "edited", rewrite: withDocComment, mustWrite: true},
}

// withDocComment prepends a documentation comment to the script's first
// top-level create statement.
func withDocComment(script string) string {
	lines := strings.Split(script, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "create ") {
			continue
		}
		// The doc comment precedes the statement's annotations (@position …).
		keep := i
		for keep > 0 && strings.HasPrefix(lines[keep-1], "@") {
			keep--
		}
		// A comment describe already printed for the statement is replaced.
		cut := keep
		if cut > 0 && strings.TrimSpace(lines[cut-1]) == "*/" {
			for cut > 0 && !strings.HasPrefix(strings.TrimSpace(lines[cut-1]), "/**") {
				cut--
			}
			cut--
		}
		out := append([]string{}, lines[:cut]...)
		out = append(out, "/** issue 816 */")
		out = append(out, lines[keep:]...)
		return strings.Join(out, "\n")
	}
	return script
}

// TestTestAppExportLevelSurvivesRoundTrip_Control proves the edited variant
// reaches the write: the edited describe output of a microflow must be written
// with the new documentation. Without it, a variant that never wrote would keep
// ExportLevel API trivially.
func TestTestAppExportLevelSurvivesRoundTrip_Control(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	for _, s := range h.exportLevelSubjects() {
		if s.keyword != "microflow" {
			continue
		}
		_, after, measured, _ := h.roundTripExportLevelUnit(t, s, withDocComment)
		if !measured {
			continue
		}
		var d bson.D
		if err := bson.Unmarshal(after, &d); err != nil {
			t.Fatal(err)
		}
		if doc := bsonString(d, "Documentation"); !strings.Contains(doc, "issue 816") {
			t.Fatalf("the edited describe output of %s was not written (Documentation %q) — the edited variant does not reach the write", s.target(), doc)
		}
		return
	}
	t.Fatal("no microflow in TestApp could be round-tripped with an edit")
}

type exportLevelSubject struct {
	document
	unit string // the unit whose ExportLevel is measured
}

// exportLevelSubjects lists every document of a describable kind whose unit
// stores an ExportLevel, together with that unit. A view entity's export level
// lives on its OQL source document, which shares the entity's name.
func (h *harness) exportLevelSubjects() []exportLevelSubject {
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
	modules := map[string]string{}
	for _, u := range units {
		b, _ := r.GetRawUnitBytes(u.ID)
		if typ, name := typeAndName(b); typ == "Projects$ModuleImpl" {
			modules[u.ID] = name
		}
	}
	var out []exportLevelSubject
	for _, u := range units {
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			h.t.Fatalf("read unit %s: %v", u.ID, err)
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			continue
		}
		if bsonString(d, "ExportLevel") == "" {
			continue
		}
		mod := modules[u.ContainerID]
		for p := u.ContainerID; mod == "" && p != ""; {
			var next string
			for _, v := range units {
				if v.ID == p {
					next = v.ContainerID
					break
				}
			}
			if next == p {
				break
			}
			p = next
			mod = modules[p]
		}
		typ, name := typeAndName(b)
		kw, ok := unitKeywords[typ]
		if typ == "DomainModels$ViewEntitySourceDocument" {
			kw, ok = "entity", true
		}
		if !ok || mod == "" {
			continue
		}
		out = append(out, exportLevelSubject{document{kw, mod + "." + name}, u.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// roundTripExportLevel sets the subject's stored ExportLevel to API on a fresh
// working copy, executes its own describe output (through rewrite, when given)
// and returns the ExportLevel stored afterwards. measured is false when the
// round trip could not run to the write, with note saying why.
func (h *harness) roundTripExportLevel(t *testing.T, s exportLevelSubject, rewrite func(string) string) (got string, measured bool, note string) {
	got, _, measured, note = h.roundTripExportLevelUnit(t, s, rewrite)
	return got, measured, note
}

// roundTripExportLevelUnit is roundTripExportLevel that also returns the unit
// as stored after the exec (nil when not measured) and whether it was written.
func (h *harness) roundTripExportLevelUnit(t *testing.T, s exportLevelSubject, rewrite func(string) string) (got string, after []byte, measured bool, note string) {
	t.Helper()
	rel := unitPath(s.unit)
	orig := h.fixture[rel]
	h.fixture[rel] = h.apiUnit(t, s.unit)
	defer func() {
		h.fixture[rel] = orig
		h.restore()
	}()
	h.restore()

	script, err := h.describe(s.target())
	if err != nil || strings.TrimSpace(script) == "" {
		return "", nil, false, fmt.Sprintf("describe failed: %v", err)
	}
	if rewrite != nil {
		script = rewrite(script)
	}
	if err := h.exec(script); err != nil {
		if isRefusal(err) {
			return "", nil, false, "refused"
		}
		return "", nil, false, fmt.Sprintf("exec failed: %v", firstLine(err.Error()))
	}
	after = h.unitBytes(t, s.unit)
	var ad bson.D
	if err := bson.Unmarshal(after, &ad); err != nil {
		t.Fatal(err)
	}
	return bsonString(ad, "ExportLevel"), after, true, ""
}

// apiUnit is the committed unit with its ExportLevel set to API.
func (h *harness) apiUnit(t *testing.T, id string) []byte {
	t.Helper()
	rel := unitPath(id)
	orig, ok := h.fixture[rel]
	if !ok {
		t.Fatalf("unit %s is not at %s", id, rel)
	}
	var d bson.D
	if err := bson.Unmarshal(orig, &d); err != nil {
		t.Fatal(err)
	}
	for i := range d {
		if d[i].Key == "ExportLevel" {
			d[i].Value = "API"
		}
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (h *harness) unitBytes(t *testing.T, id string) []byte {
	t.Helper()
	r, err := mmpr.Open(h.mpr)
	if err != nil {
		t.Fatalf("open working copy: %v", err)
	}
	defer r.Close()
	b, err := r.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("read unit %s: %v", id, err)
	}
	return b
}

// unitPath is where MPR v2 stores a unit: mprcontents/<2>/<2>/<id>.mxunit.
func unitPath(id string) string {
	return filepath.Join("mprcontents", id[0:2], id[2:4], id+".mxunit")
}

func bsonString(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}
