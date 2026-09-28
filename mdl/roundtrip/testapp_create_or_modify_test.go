// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"regexp"
	"testing"
)

// deferredVerbKinds are the document kinds whose describe still prints a plain
// `create` (#743): until a `create or modify` rewrite carries everything
// describe cannot print, the plain verb refuses on an existing document rather
// than silently losing Studio Pro-authored content (ADR-0012: carry or refuse).
//
// TestTestAppCreateOrModifyProbe is the evidence for switching a kind: it runs
// each such document's describe output with the verb rewritten to `create or
// modify`, on the Studio Pro-authored TestApp, and holds it to the same laws.
// A kind may switch its describe verb once none of its documents is listed in
// createOrModifyProbeKnownFailures; the entry is then struck, and the main
// round trip (TestTestAppRoundTrip) covers it from then on.
//
// `odata service` switched in #743: its probe passed on all three TestApp
// services once the rewrite carried ExportLevel, PageSize, the entity-set order
// and CanBeEmpty. `workflow`, `odata client` and `external entity` switched
// once theirs passed too: the names of a workflow's implicit activities, empty
// outcome flows and the empty EventSubProcesses list; a client's icon,
// UseQuerySegment and the keys Studio Pro stores empty; an entity's false
// generalization flags. No kind is deferred now; the probe stays for the next
// kind whose describe has to keep a plain `create`.
var deferredVerbKinds = map[string]bool{}

// createOrModifyProbeKnownFailures lives in testapp_allowlist_test.go.

// describe writes a consumed OData service under its Studio Pro name (R10,
// #755); the harness still addresses it by the kind word "odata client".
var leadingCreate = regexp.MustCompile(`(?m)^create (workflow|consumed odata service|external entity) `)

var leadingCreateOrModify = regexp.MustCompile(`(?m)^create or modify (workflow|consumed odata service|external entity) `)

// asCreateOrModify rewrites each top-level plain `create <kind>` of a deferred
// kind to `create or modify <kind>`.
func asCreateOrModify(script string) string {
	return leadingCreate.ReplaceAllString(script, "create or modify $1 ")
}

func TestTestAppCreateOrModifyProbe(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	if len(deferredVerbKinds) == 0 {
		if len(createOrModifyProbeKnownFailures) > 0 {
			t.Errorf("createOrModifyProbeKnownFailures lists %d document(s), but no kind is deferred — remove them", len(createOrModifyProbeKnownFailures))
		}
		t.Skip("no kind's describe prints a plain `create`; TestTestAppRoundTrip covers every kind")
	}
	probed := 0
	seen := map[string]bool{}
	for _, d := range h.documents() {
		if !deferredVerbKinds[d.keyword] {
			continue
		}
		key := d.key()
		seen[key] = true
		t.Run(key, func(t *testing.T) {
			first, err := h.describe(d.target())
			if err != nil {
				t.Fatalf("describe %s: %v", key, err)
			}
			if !leadingCreate.MatchString(first) {
				if leadingCreateOrModify.MatchString(first) {
					t.Fatalf("describe %s already prints `create or modify` — remove %q from deferredVerbKinds; TestTestAppRoundTrip covers it now", key, d.keyword)
				}
				t.Fatalf("describe %s printed no `create %s` to probe:\n%s", key, d.keyword, first)
			}
			probed++
			judge(t, createOrModifyProbeKnownFailures, key, h.roundTripWith(d, asCreateOrModify))
		})
	}
	// A probe that looked at nothing proves nothing: TestApp has documents of
	// every deferred kind.
	if probed == 0 {
		t.Errorf("probed no documents — the enumeration of %v is broken", deferredVerbKinds)
	}
	for key := range createOrModifyProbeKnownFailures {
		if !seen[key] {
			t.Errorf("createOrModifyProbeKnownFailures lists %q, which TestApp does not contain — stale entry, remove it", key)
		}
	}
}

// The rewrite is the probe's only lever, so it is pinned: it must turn the
// statement's own verb and nothing inside it.
func TestAsCreateOrModify(t *testing.T) {
	in := "-- Workflow: M.W\n\ncreate workflow M.W\n  display 'create workflow x'\nbegin\nend workflow;\n"
	want := "-- Workflow: M.W\n\ncreate or modify workflow M.W\n  display 'create workflow x'\nbegin\nend workflow;\n"
	if got := asCreateOrModify(in); got != want {
		t.Errorf("asCreateOrModify:\n%s\nwant\n%s", got, want)
	}
	if got := asCreateOrModify("create or modify consumed odata service M.S (\n);\n"); got != "create or modify consumed odata service M.S (\n);\n" {
		t.Errorf("an existing `create or modify` was rewritten: %q", got)
	}
}
