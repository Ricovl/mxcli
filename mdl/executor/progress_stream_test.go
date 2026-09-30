// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// Progress is commentary about a run. In text mode it is part of what a person
// reads and stays on Output; in JSON mode Output is a payload and progress moves
// to the diagnostics stream. The end-to-end version of this, across every query
// subcommand, is cmd/mxcli's TestJSONFlagKeepsStdoutPureJSON.
func TestProgressFollowsTheOutputFormat(t *testing.T) {
	var out, diag bytes.Buffer

	text := &ExecContext{Output: &out, Diagnostics: &diag, Format: FormatTable}
	if text.progress() != &out {
		t.Error("text mode: progress must stay on Output, where a person has always read it")
	}

	js := &ExecContext{Output: &out, Diagnostics: &diag, Format: FormatJSON}
	if js.progress() != &diag {
		t.Error("JSON mode: progress must go to Diagnostics, not into the payload")
	}
}

// An empty answer must be parseable in JSON mode: "[]", with the sentence on the
// diagnostics stream. In text mode it is the sentence, as before.
func TestWriteEmptyResult(t *testing.T) {
	var out, diag bytes.Buffer
	ctx := &ExecContext{Output: &out, Diagnostics: &diag, Format: FormatJSON}
	if err := writeEmptyResult(ctx, []string{"SourceName"}, "(no references found)"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("JSON mode payload = %q, want []", got)
	}
	if !strings.Contains(diag.String(), "(no references found)") {
		t.Errorf("JSON mode: the message should still reach diagnostics, got %q", diag.String())
	}

	out.Reset()
	diag.Reset()
	ctx.Format = FormatTable
	if err := writeEmptyResult(ctx, nil, "(no references found)"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "(no references found)" {
		t.Errorf("text mode output = %q, want the sentence unchanged", got)
	}
	if diag.Len() != 0 {
		t.Errorf("text mode wrote to diagnostics: %q", diag.String())
	}
}
