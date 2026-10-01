// SPDX-License-Identifier: Apache-2.0

package langver

import "testing"

func TestHeaderlessIsMdl0(t *testing.T) {
	if Default != V0 {
		t.Fatal("a script without a header must get the alpha meaning (ADR-0011), never the latest")
	}
}

// mdl 1 is frozen (ako/mxcli#714): describe and fmt write its header, and the
// interactive surfaces start in it — while a headerless script stays mdl 0.
func TestMdl1IsFrozen(t *testing.T) {
	if Frozen != V1 {
		t.Fatalf("Frozen = %v, want mdl 1", Frozen)
	}
	if got := HeaderLine(); got != "mdl 1;" {
		t.Errorf("describe/fmt header = %q, want %q", got, "mdl 1;")
	}
	if Interactive != V1 {
		t.Errorf("the REPL and -c start in %v, want the frozen mdl 1", Interactive)
	}
	if Default != V0 {
		t.Error("a headerless script must keep the alpha meaning (ADR-0011)")
	}
}

func TestHeaderFor(t *testing.T) {
	for v, want := range map[Version]string{V0: "", V1: "mdl 1;"} {
		if got := HeaderFor(v); got != want {
			t.Errorf("HeaderFor(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestParseFlag(t *testing.T) {
	for in, want := range map[string]Version{"0": V0, "1": V1, " 1 ": V1, "mdl 1": V1, "MDL 0": V0} {
		if got, err := ParseFlag(in); err != nil || got != want {
			t.Errorf("ParseFlag(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "2", "-1", "one", "1.0"} {
		if _, err := ParseFlag(in); err == nil {
			t.Errorf("ParseFlag(%q) accepted a version this mxcli does not know", in)
		}
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

func TestScanHeader(t *testing.T) {
	for src, want := range map[string]Version{
		"mdl 1;\nshow entities;":              V1,
		"MDL 1 ;":                             V1,
		"  -- a note\n/* block */\nmdl\n1\n;": V1,
		"mdl /* between */ 1 -- and here\n;":  V1,
		"show entities;":                      V0,
		"":                                    V0,
		"mdl 1":                               V0, // no `;`: not a header
		"mdl 1.5;":                            V0,
		"mdl 99;":                             V0, // unknown: the parser refuses it
		"mdlx 1;":                             V0,
		"/** doc */ mdl 1;":                   V0, // a doc comment is a token
		"create entity mdl.X ( A: String );\nmdl 1;": V0, // only the first statement
		"mdl 0;": V0,
	} {
		if got := ScanHeader(src); got != want {
			t.Errorf("ScanHeader(%q) = %v, want %v", src, got, want)
		}
	}
}

// An explicit `mdl 0;` is a header; no header is not. The REPL switches its
// session on the first and keeps it on the second.
func TestScanWrittenHeader(t *testing.T) {
	for src, want := range map[string]struct {
		v       Version
		written bool
	}{
		"mdl 0;":         {V0, true},
		"mdl 1; list x;": {V1, true},
		"list entities;": {V0, false},
		"mdl 99;":        {V0, false},
		"":               {V0, false},
	} {
		if v, w := ScanWrittenHeader(src); v != want.v || w != want.written {
			t.Errorf("ScanWrittenHeader(%q) = %v, %v; want %v, %v", src, v, w, want.v, want.written)
		}
	}
}
