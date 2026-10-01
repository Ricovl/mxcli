// SPDX-License-Identifier: Apache-2.0

package formatter

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// fmt keeps a script's language header where it is. Only `fmt --upgrade` adds
// one (ADR-0011): a headerless script is mdl 0, and adding the header would
// change its meaning — mdl 1 being frozen does not change that.
func TestFormat_LanguageHeaderRoundTrips(t *testing.T) {
	src := "mdl 1;\ncreate entity Shop.Customer ( Name: String(200) );\n"
	got := Format(src)
	if !strings.HasPrefix(got, "mdl 1;\n") {
		t.Fatalf("header not kept as the first line:\n%s", got)
	}
	if again := Format(got); again != got {
		t.Fatalf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

func TestFormat_AddsNoHeaderToAHeaderlessScript(t *testing.T) {
	if langver.HeaderLine() == "" {
		t.Fatal("control: mdl 1 is frozen, so there is a header fmt could add")
	}
	got := Format("create entity Shop.Customer ( Name: String(200) );\n")
	if langver.IsHeaderLine(strings.SplitN(got, "\n", 2)[0]) {
		t.Fatalf("fmt added a language header to a headerless script:\n%s", got)
	}
}

// Concatenated describe output repeats the header; fmt keeps each one.
func TestFormat_RepeatedHeaderIsKept(t *testing.T) {
	src := "mdl 1;\ncreate entity Shop.A ( Name: String(200) );\nmdl 1;\ncreate entity Shop.B ( Name: String(200) );\n"
	if got := Format(src); strings.Count(got, "mdl 1;") != 2 {
		t.Fatalf("a repeated header was not kept:\n%s", got)
	}
}
