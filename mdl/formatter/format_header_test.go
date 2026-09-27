// SPDX-License-Identifier: Apache-2.0

package formatter

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// fmt keeps a script's language header where it is, and adds none: mdl 1 is a
// preview until beta, and only `fmt --upgrade` may add a header (ADR-0011).
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

func TestFormat_AddsNoHeaderWhileMdl1IsAPreview(t *testing.T) {
	if langver.HeaderLine() != "" {
		t.Skip("mdl 1 is frozen; fmt emitting it is decided by HeaderLine")
	}
	got := Format("create entity Shop.Customer ( Name: String(200) );\n")
	if langver.IsHeaderLine(strings.SplitN(got, "\n", 2)[0]) {
		t.Fatalf("fmt added a language header to a headerless script:\n%s", got)
	}
}
