// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// The REPL starts in mdl 1 since the freeze (decision 6, ako/mxcli#714), and an
// `mdl 0;` / `mdl 1;` statement switches the session for what follows. The
// probe is `show entity X`: refused by mdl 1 at parse time, and under mdl 0
// parsed and then refused for want of a connection.
func TestREPLLanguage_DefaultAndSwitch(t *testing.T) {
	const probe = "show entity M.E;\n"
	run := func(input string) (string, *REPL) {
		var out bytes.Buffer
		r := New(strings.NewReader(input), &out)
		if err := r.Run(); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return out.String(), r
	}

	out, r := run(probe)
	if r.Language() != langver.V1 {
		t.Fatalf("the REPL starts in %v, want mdl 1", r.Language())
	}
	if !strings.Contains(out, "Parse error") || !strings.Contains(out, "is not in mdl 1") {
		t.Fatalf("headerless input was not read as mdl 1:\n%s", out)
	}

	out, r = run("mdl 0;\n" + probe)
	if r.Language() != langver.V0 {
		t.Fatalf("`mdl 0;` did not switch the session: %v", r.Language())
	}
	if strings.Contains(out, "Parse error") {
		t.Fatalf("after `mdl 0;` the probe was still read as mdl 1:\n%s", out)
	}

	// And back: the switch is a session setting, not a one-statement header.
	out, r = run("mdl 0;\nmdl 1;\n" + probe)
	if r.Language() != langver.V1 || !strings.Contains(out, "is not in mdl 1") {
		t.Fatalf("`mdl 1;` did not switch the session back (%v):\n%s", r.Language(), out)
	}
}

// --mdl 0 (SetLanguage) starts the session in mdl 0.
func TestREPLLanguage_SetLanguage(t *testing.T) {
	var out bytes.Buffer
	r := New(strings.NewReader("show entity M.E;\n"), &out)
	r.SetLanguage(langver.V0)
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Parse error") {
		t.Fatalf("a session started in mdl 0 read input as mdl 1:\n%s", out.String())
	}
}

// Session commands stay the REPL's under mdl 1 (R7): `status` is typed here.
func TestREPLLanguage_SessionCommandsUnderMdl1(t *testing.T) {
	var out bytes.Buffer
	r := New(strings.NewReader("status;\nset format = json;\n"), &out)
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Parse error") {
		t.Fatalf("the mdl 1 REPL refused a session command:\n%s", out.String())
	}
}
