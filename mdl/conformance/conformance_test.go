// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"reflect"
	"strings"
	"testing"
)

func classes(fs []Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Class)
	}
	return out
}

// The gate's two findings, each against a control that differs only in the
// spelling: a deprecated spelling is reported under its registry code, and its
// canonical form is not reported at all.
func TestCheck_DeprecatedSpellingAgainstCanonicalControl(t *testing.T) {
	for _, tc := range []struct {
		name, old, canonical, code string
	}{
		{"statement", "show entities in M;", "list entities in M;", "MDL-DEPR002"},
		{"microflow activity", "$H = head($L);", "$H = head $L;", "MDL-DEPR003"},
		{"page widget", "actionbutton b (Caption: 'Out', Action: sign_out)",
			"actionbutton b (Caption: 'Out', Action: sign out)", "MDL-DEPR020"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classes(Check(Unit{Source: "doc.md", Line: 10, Text: tc.old}))
			if !reflect.DeepEqual(got, []string{tc.code}) {
				t.Errorf("old spelling %q: findings %v, want [%s]", tc.old, got, tc.code)
			}
			if got := Check(Unit{Source: "doc.md", Line: 10, Text: tc.canonical}); len(got) != 0 {
				t.Errorf("canonical control %q: findings %v, want none", tc.canonical, got)
			}
		})
	}
}

// A line number points into the source, whichever context the block parsed in.
func TestCheck_LineMapsThroughTheWrapper(t *testing.T) {
	block := "-- a comment\n$A = 1;\n$H = head($L);"
	got := Check(Unit{Source: "doc.md", Line: 40, Text: block})
	if len(got) != 1 || got[0].Line != 42 {
		t.Fatalf("got %v, want one finding on line 42", got)
	}
}

// A template next to a real statement does not hide the statement: the block
// is split on blank lines when it does not parse whole.
func TestCheck_TemplateChunkDoesNotHideItsNeighbour(t *testing.T) {
	block := "create entity <Module>.<Name> (...);\n\nshow entities in M;"
	got := Check(Unit{Source: "doc.md", Line: 1, Text: block})
	if want := []string{ClassSyntax, "MDL-DEPR002"}; !reflect.DeepEqual(classes(got), want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got[1].Line != 3 {
		t.Errorf("deprecation reported on line %d, want 3", got[1].Line)
	}
}

// A whole script is parsed as it stands, not in a wrapper.
func TestCheck_ScriptIsNotWrapped(t *testing.T) {
	got := Check(Unit{Source: "x.mdl", Line: 1, Text: "$H = head($L);", Script: true})
	if !reflect.DeepEqual(classes(got), []string{ClassSyntax}) {
		t.Fatalf("got %v, want a syntax finding: a microflow activity is not a script", got)
	}
	if got := Check(Unit{Source: "x.mdl", Line: 1, Text: "show entities in M;\n", Script: true}); !reflect.DeepEqual(classes(got), []string{"MDL-DEPR002"}) {
		t.Fatalf("got %v, want [MDL-DEPR002]", got)
	}
}

func TestMarkdownUnits(t *testing.T) {
	doc := strings.Join([]string{
		"# Title",               // 1
		"```mdl",                // 2
		"list entities in M;",   // 3
		"```",                   // 4
		"```bash",               // 5
		"```sql",                // 6: inside a bash fence, not an opening
		"```",                   // 7
		"- item",                // 8
		"  ```sql",              // 9
		"  show entities in M;", // 10
		"  ```",                 // 11
		"```text",               // 12
		"show entities in M;",   // 13
		"```",                   // 14
	}, "\n")
	units := MarkdownUnits("d.md", doc)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2: %+v", len(units), units)
	}
	if units[0].Line != 3 || units[0].Text != "list entities in M;" {
		t.Errorf("first unit %+v", units[0])
	}
	if units[1].Line != 10 || units[1].Text != "show entities in M;" {
		t.Errorf("indented unit %+v, want dedented text on line 10", units[1])
	}
}

func TestCompareAndShrink(t *testing.T) {
	a := Key{"a.md", "MDL-DEPR002"}
	b := Key{"b.md", ClassSyntax}
	c := Key{"c.md", ClassSyntax}
	allowed := Tally{a: 2, b: 3}
	measured := Tally{a: 3, b: 1, c: 1}

	d := Compare(measured, allowed)
	if !reflect.DeepEqual(d.Grown, []Key{a, c}) {
		t.Errorf("grown %v, want [a c]: a count above its ceiling and a key with none", d.Grown)
	}
	if !reflect.DeepEqual(d.Shrinkable, []Key{b}) {
		t.Errorf("shrinkable %v, want [b]", d.Shrinkable)
	}
	// Shrink lowers b and neither raises a nor adds c.
	if got := Shrink(measured, allowed); !reflect.DeepEqual(got, Tally{a: 2, b: 1}) {
		t.Errorf("shrink %v", got)
	}
	// A key measured at zero is deleted.
	if got := Shrink(Tally{}, allowed); len(got) != 0 {
		t.Errorf("shrink to nothing: %v", got)
	}
	// Control: equal counts drift in neither direction.
	if d := Compare(allowed, allowed); len(d.Grown)+len(d.Shrinkable) != 0 {
		t.Errorf("identical tallies drift: %+v", d)
	}
}

func TestAllowlistRoundTrip(t *testing.T) {
	in := Tally{{"docs-site/src/a b.md", ClassSyntax}: 4, {"syntax:page", "MDL-DEPR020"}: 1}
	got, err := ParseAllowlist(strings.NewReader(FormatAllowlist(in)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip: %v, want %v", got, in)
	}
	for _, bad := range []string{"3 syntax a.md\n", "0\tsyntax\ta.md\n", "1\tsyntax\ta.md\n1\tsyntax\ta.md\n"} {
		if _, err := ParseAllowlist(strings.NewReader(bad)); err == nil {
			t.Errorf("ParseAllowlist(%q) accepted a malformed list", bad)
		}
	}
}
