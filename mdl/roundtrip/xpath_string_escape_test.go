// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// xpathEscapeCases are string literals as a script of one language spells
// them, and the literal they store: the value in Mendix's spelling. A
// bracketed XPath reads its strings by the same rule as an expression
// (ako/mxcli#822, #825).
var xpathEscapeCases = map[string][]escapeCase{
	"mdl 0": {
		{"backslash", `'C:\\temp'`, `'C:\temp'`},
		{"trailing backslash", `'C:\\'`, `'C:\'`},
		{"escaped apostrophe", `'it\'s'`, `'it''s'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"escaped line break", `'a\nb'`, "'a\nb'"},
	},
	"mdl 1": {
		{"backslash", `'C:\temp'`, `'C:\temp'`},
		{"trailing backslash", `'C:\'`, `'C:\'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"line break", "'a\nb'", "'a\nb'"},
		{"backslash n", `'a\nb'`, `'a\nb'`},
	},
}

const xpathModule = "XEsc"

// tagged puts a marker at the start of a literal's value, so that the stored
// field that holds it can be found among all of the module's strings.
func tagged(lit, tag string) string { return "'" + tag + " " + lit[1:] }

// xpathKindsScript writes each literal into every bracketed XPath a script
// can hold outside a retrieve's where (TestRetrieveXPathStringEscapes covers
// that): an access rule, short and too long for a line, a page datasource, a
// workflow user task's targeting, and — the expression stored as written next
// to a retrieve's constraint — its limit and offset.
func xpathKindsScript(header string, cases []escapeCase) (string, map[string]string) {
	var b strings.Builder
	want := map[string]string{} // tag -> the stored literal
	b.WriteString(header)
	m := xpathModule
	fmt.Fprintf(&b, "create module %s;\ncreate module role %s.User;\n", m, m)
	fmt.Fprintf(&b, "create persistent entity %s.E (Name: String(200));\n", m)
	for i, c := range cases {
		short, long := fmt.Sprintf("a%d", i), fmt.Sprintf("l%d", i)
		fmt.Fprintf(&b, "grant read * on entity %s.E to %s.User where [Name = %s];\n", m, m, tagged(c.lit, short))
		fmt.Fprintf(&b, "grant read * on entity %s.E to %s.User where [Name = %s or Name = 'a value long enough to break the rule over lines'];\n",
			m, m, tagged(c.lit, long))
		want[short], want[long] = tagged(c.stored, short), tagged(c.stored, long)
	}
	fmt.Fprintf(&b, "create page %s.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n", m)
	for i, c := range cases {
		tag := fmt.Sprintf("p%d", i)
		fmt.Fprintf(&b, "  listview lv%d (datasource: database from %s.E where [Name = %s]) {\n    dynamictext t%d (content: 'x')\n  }\n",
			i, m, tagged(c.lit, tag), i)
		want[tag] = tagged(c.stored, tag)
	}
	b.WriteString("};\n")
	fmt.Fprintf(&b, "create workflow %s.WF\n  parameter $WorkflowContext: %s.E\nbegin\n", m, m)
	for i, c := range cases {
		tag := fmt.Sprintf("w%d", i)
		fmt.Fprintf(&b, "  user task T%d 'T%d'\n    targeting users xpath [Name = %s]\n    outcomes 'Done' { };\n", i, i, tagged(c.lit, tag))
		want[tag] = tagged(c.stored, tag)
	}
	b.WriteString("end workflow;\n")
	fmt.Fprintf(&b, "create microflow %s.L () begin\n", m)
	for i, c := range cases {
		lt, ot := fmt.Sprintf("n%d", i), fmt.Sprintf("o%d", i)
		fmt.Fprintf(&b, "  retrieve $r%d from %s.E limit length(%s) offset length(%s);\n", i, m, tagged(c.lit, lt), tagged(c.lit, ot))
		want[lt], want[ot] = tagged(c.stored, lt), tagged(c.stored, ot)
	}
	b.WriteString("end;\n")
	return b.String(), want
}

// moduleStrings is every string field of every unit, with the units that hold
// the marker.
func moduleStrings(t *testing.T, units map[string][]byte) (strs []string, holding map[string][]byte) {
	t.Helper()
	holding = map[string][]byte{}
	for id, u := range units {
		if !bytes.Contains(u, []byte(xpathModule)) {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(u, &doc); err != nil {
			t.Fatal(err)
		}
		var walk func(v any)
		walk = func(v any) {
			switch x := v.(type) {
			case bson.D:
				for _, e := range x {
					walk(e.Value)
				}
			case bson.A:
				for _, e := range x {
					walk(e)
				}
			case string:
				strs = append(strs, x)
			}
		}
		walk(doc)
		holding[id] = u
	}
	return strs, holding
}

// A string in any bracketed XPath — and in the expressions stored as written
// beside a retrieve's — stores its value in Mendix's spelling in either
// language, and describe in that language writes each back so that it reads as
// that value: the descriptions, executed, write nothing, and describe the
// same again. Under mdl 0 an access rule and a workflow targeting used to be
// stored as written, so `'C:\\temp'` stored two backslashes and `'it\'s'` an
// escape Mendix does not have; a page datasource stored the value but described
// it as stored, so `'C:\temp'` came back a tab (ako/mxcli#825).
func TestXPathKindsStringEscapes(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	targets := []string{"entity " + xpathModule + ".E", "page " + xpathModule + ".P",
		"workflow " + xpathModule + ".WF", "microflow " + xpathModule + ".L"}
	for _, lang := range []string{"mdl 0", "mdl 1"} {
		t.Run(lang, func(t *testing.T) {
			h.restore()
			header := ""
			if lang == "mdl 1" {
				header = "mdl 1;\n"
			}
			describe := func() string {
				var out []string
				for _, target := range targets {
					if header == "" {
						out = append(out, h.mustDescribeMdl0(t, target))
					} else {
						out = append(out, h.describeUnder(header, target))
					}
				}
				return strings.Join(out, "\n")
			}
			script, want := xpathKindsScript(header, xpathEscapeCases[lang])
			if err := h.exec(script); err != nil {
				t.Fatalf("exec: %v\n%s", err, script)
			}
			strs, before := moduleStrings(t, h.snapshot().units)
			all := strings.Join(strs, "\n\x00\n")
			for tag, lit := range want {
				if !strings.Contains(all, lit) {
					var holding []string
					for _, s := range strs {
						if strings.Contains(s, "'"+tag+" ") {
							holding = append(holding, fmt.Sprintf("%q", s))
						}
					}
					t.Errorf("%s: want a stored field holding %q, have %s", tag, lit, strings.Join(holding, ", "))
				}
			}

			first := describe()
			if err := h.exec(header + first); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, first)
			}
			_, after := moduleStrings(t, h.snapshot().units)
			for id, b := range before {
				if !bytes.Equal(after[id], b) {
					typ, name := typeAndName(b)
					t.Errorf("describe -> exec wrote %s %s", typ, name)
				}
			}
			if again := describe(); again != first {
				t.Errorf("describe -> exec -> describe changed it:\n%s", lineDiff(first, again))
			}

			// Control: read in the other language, the description spells
			// other values, so the unchanged-write comparison above can fail.
			other := "mdl 1;\n"
			if header != "" {
				other = ""
			}
			if err := h.exec(other + withoutHeader(first)); err == nil {
				_, control := moduleStrings(t, h.snapshot().units)
				same := true
				for id, b := range before {
					same = same && bytes.Equal(control[id], b)
				}
				if same {
					t.Error("the description read in the other language wrote nothing — the comparison cannot fail")
				}
			}
		})
	}
}

// fmt --upgrade --header of the mdl 0 script is an mdl 1 script that builds
// the same model: an XPath string is requoted in place, and under mdl 1 the
// requoted literal stores what the mdl 0 one stored. Control: the mdl 0 script
// given only the header builds another model.
func TestUpgradeOfXPathStringEscapes(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()
	script, _ := xpathKindsScript("", xpathEscapeCases["mdl 0"])
	res, err := upgrade.Upgrade(script, upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if _, errs := visitor.Build(res.Source); len(errs) > 0 {
		t.Fatalf("the upgraded script does not parse: %v\n%s", errs[0], res.Source)
	}
	errA, errB, diff := executeBoth(t, a, b, script, res.Source)
	if errA != nil || errB != nil {
		t.Fatalf("execute: %v / %v\n%s", errA, errB, res.Source)
	}
	if len(diff) > 0 {
		t.Errorf("the upgraded script builds another model:\n  %s\n%s", strings.Join(diff, "\n  "), res.Source)
	}
	if _, _, diff := executeBoth(t, a, b, script, "mdl 1;\n"+script); len(diff) == 0 {
		t.Fatal("the script under the header alone built the same model — the comparison cannot fail")
	}
}
