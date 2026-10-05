// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The command resolves mxcli verbs with the real command tree, as
// loop-report does, so `docker check` and `-c list` are named the same way.
func TestCobraVerb(t *testing.T) {
	cases := map[string]string{
		"mxcli exec a.mdl -p app.mpr":       "exec",
		"mxcli -p app.mpr docker check":     "docker check",
		"mxcli diag session-report x.jsonl": "diag session-report",
		"mxcli -p app.mpr -c describe":      "",
		"mxcli syntax page":                 "syntax",
	}
	for in, want := range cases {
		if got := cobraVerb(strings.Fields(in)); got != want {
			t.Errorf("cobraVerb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSessionReports(t *testing.T) {
	fx := "sessionreport/testdata/session1.jsonl"
	var text bytes.Buffer
	if err := writeSessionReports(&text, []string{fx}, 5, true, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"session aaaaaaaa (+1 subagents)", "model calls 5 (main 3)", "-c list 1", "check 1", "failures 1"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("missing %q in:\n%s", want, text.String())
		}
	}
	if strings.Contains(text.String(), "combined") {
		t.Errorf("one transcript must not print a combined report")
	}

	var js bytes.Buffer
	if err := writeSessionReports(&js, []string{fx, fx}, 5, true, true); err != nil {
		t.Fatal(err)
	}
	var doc sessionReportJSON
	if err := json.Unmarshal(js.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(doc.Sessions) != 2 || doc.Combined == nil || doc.Combined.ModelCalls != 10 {
		t.Fatalf("json = %+v", doc)
	}
}
