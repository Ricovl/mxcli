// SPDX-License-Identifier: Apache-2.0

package langver

import "testing"

func TestHeaderlessIsMdl0(t *testing.T) {
	if Default != V0 {
		t.Fatal("a script without a header must get the alpha meaning (ADR-0011), never the latest")
	}
}

// Before beta, mdl 1 is a preview and nothing emits a header.
func TestPreviewState(t *testing.T) {
	if !V1.IsPreview() {
		t.Error("mdl 1 must be a preview until beta")
	}
	if V0.IsPreview() {
		t.Error("mdl 0 is the alpha language, not a preview")
	}
	if got := HeaderLine(); got != "" {
		t.Errorf("describe/fmt must not emit a header while mdl 1 is a preview, got %q", got)
	}
}

// Flipping the switch to V1 is all beta takes: mdl 1 stops being a preview and
// becomes the header describe/fmt write.
func TestFrozenSwitch(t *testing.T) {
	if isPreview(V1, V1) {
		t.Error("mdl 1 is still a preview once frozen")
	}
	if got := headerLine(V1); got != "mdl 1;" {
		t.Errorf("frozen mdl 1: got header %q", got)
	}
	if isPreview(Latest+1, V1) {
		t.Error("an unknown version is not a preview, it is refused")
	}
}

func TestKnown(t *testing.T) {
	for v, want := range map[Version]bool{-1: false, V0: true, V1: true, Latest + 1: false} {
		if v.Known() != want {
			t.Errorf("Known(%d) = %v", v, !want)
		}
	}
}

func TestChangeApplies(t *testing.T) {
	c := Change{Code: "X", Since: V1, Old: "old", New: "new"}
	if c.Applies(V0) || !c.Applies(V1) {
		t.Fatal("a change applies from its version on, and only then")
	}
}

func TestIsHeaderLine(t *testing.T) {
	for line, want := range map[string]bool{
		"mdl 1;":                             true,
		"  MDL 1 ;  -- note":                 true,
		"mdl 1":                              false,
		"create entity mdl.X ( A: String );": false,
	} {
		if IsHeaderLine(line) != want {
			t.Errorf("IsHeaderLine(%q) = %v", line, !want)
		}
	}
}
