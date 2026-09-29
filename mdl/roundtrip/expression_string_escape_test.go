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

// escapeCase is one string literal as a script of one language spells it, and
// the Mendix expression it must store: the value the literal has in that
// language, written the way Studio Pro writes it — an apostrophe doubled, a
// backslash and a control character as themselves (ako/mxcli#810, #820).
type escapeCase struct {
	name   string
	lit    string // the literal in the script
	stored string // the Mendix expression the literal stores
}

// Under mdl 0 a backslash before n, r, t, a backslash or an apostrophe is an
// escape, and any other backslash is itself. Under mdl 1 a backslash is always
// itself and a doubled apostrophe the only escape (ADR-0010 R11).
var escapeCases = map[string][]escapeCase{
	"mdl 0": {
		{"backslash", `'C:\\temp\\new'`, `'C:\temp\new'`},
		{"backslash before another character", `'C:\data'`, `'C:\data'`},
		{"escaped apostrophe", `'it\'s'`, `'it''s'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"escaped line break", `'a\nb'`, "'a\nb'"},
		{"regex", `'^\d+\.\d{2}$'`, `'^\d+\.\d{2}$'`},
		{"escaped regex", `'^\\d+\\n$'`, `'^\d+\n$'`},
		{"trailing backslash", `'C:\\'`, `'C:\'`},
	},
	"mdl 1": {
		{"backslash", `'C:\temp\new'`, `'C:\temp\new'`},
		{"doubled backslash", `'C:\\temp'`, `'C:\\temp'`},
		{"trailing backslash", `'C:\'`, `'C:\'`},
		{"doubled apostrophe", `'it''s'`, `'it''s'`},
		{"backslash n", `'a\nb'`, `'a\nb'`},
		{"regex", `'^\d+\.\d{2}$'`, `'^\d+\.\d{2}$'`},
	},
}

// escapeSlots are the three ways the builder stores an expression: rendered
// from its tree (one line, spaced operators), stored as written because it
// spans lines, and stored as written because an operator is unspaced — the
// quote scanner decides that last one, so it has to see where the string ends
// (#820).
var escapeSlots = []struct {
	name  string
	write func(lit string) string
}{
	{"rendered", func(lit string) string { return lit }},
	{"spanning lines", func(lit string) string { return lit + "\n    + 'x'" }},
	{"unspaced operator", func(lit string) string { return lit + "+'x'" }},
}

const escapeModule = "StrEscape"

// escapeScript is a microflow declaring one variable per case and slot.
func escapeScript(header string, cases []escapeCase) string {
	var b strings.Builder
	b.WriteString(header)
	fmt.Fprintf(&b, "create module %s;\ncreate microflow %s.Esc () begin\n", escapeModule, escapeModule)
	for i, c := range cases {
		for j, s := range escapeSlots {
			fmt.Fprintf(&b, "  declare $v%d_%d String = %s;\n", i, j, s.write(c.lit))
		}
	}
	b.WriteString("end;\n")
	return b.String()
}

// storedInitialValues maps each variable a flow declares to the expression
// stored for its initial value.
func storedInitialValues(t *testing.T, unit []byte) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(unit, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			var name, value string
			var hasValue bool
			for _, e := range x {
				switch e.Key {
				case "VariableName":
					name, _ = e.Value.(string)
				case "InitialValue":
					value, hasValue = e.Value.(string)
				}
				walk(e.Value)
			}
			if name != "" && hasValue {
				out[name] = value
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// Each literal, in each language and each slot, stores the Mendix expression
// for its value, which is what Studio Pro stores for it. describe in that
// language writes the expression so that it reads back as the same value: the
// description, executed, writes nothing, and describes the same again.
// Reverting any one of QuoteLiteral, the transcription of an expression stored
// as written, the version-aware quote scanner or describe's mdl 0 spelling
// fails it.
func TestExpressionStringEscapesStoreWhatStudioProStores(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const target = "microflow " + escapeModule + ".Esc"
	for _, lang := range []string{"mdl 0", "mdl 1"} {
		t.Run(lang, func(t *testing.T) {
			h.restore()
			header := ""
			if lang == "mdl 1" {
				header = "mdl 1;\n"
			}
			describe := func() string {
				if header == "" {
					return h.mustDescribe(t, target)
				}
				return h.describeUnder(header, target)
			}
			cases := escapeCases[lang]
			script := escapeScript(header, cases)
			if err := h.exec(script); err != nil {
				t.Fatalf("exec: %v\n%s", err, script)
			}
			stored := storedInitialValues(t, h.flowUnit(t, "Esc"))
			for i, c := range cases {
				for j, s := range escapeSlots {
					want := s.write(c.stored)
					if got := stored[fmt.Sprintf("v%d_%d", i, j)]; got != want {
						t.Errorf("%s, %s: %s stored\n  %q\nwant\n  %q", c.name, s.name, s.write(c.lit), got, want)
					}
				}
			}

			first := describe()
			before := h.flowUnit(t, "Esc")
			if err := h.exec(header + first); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, first)
			}
			if after := h.flowUnit(t, "Esc"); !bytes.Equal(after, before) {
				t.Errorf("describe -> exec wrote the flow:\n%s\n%s", diffInitialValues(t, before, after), first)
			}
			if again := describe(); again != first {
				t.Errorf("describe -> exec -> describe changed it:\n%s", lineDiff(first, again))
			}

			// Control: read in the other language, the description spells other
			// values, so the unchanged-write comparison above can fail.
			other := "mdl 1;\n"
			if header != "" {
				other = ""
			}
			if err := h.exec(other + first); err == nil && bytes.Equal(h.flowUnit(t, "Esc"), before) {
				t.Error("the description read in the other language wrote nothing — the comparison cannot fail")
			}
		})
	}
}

func diffInitialValues(t *testing.T, before, after []byte) string {
	b, a := storedInitialValues(t, before), storedInitialValues(t, after)
	var out []string
	for k, v := range b {
		if a[k] != v {
			out = append(out, fmt.Sprintf("  %s: %q -> %q", k, v, a[k]))
		}
	}
	return strings.Join(out, "\n")
}

// fmt --upgrade --header of the mdl 0 script is an mdl 1 script that parses
// and builds the same model — including an expression stored as written that
// holds `\'`, which the upgrade used to leave in place, so that the mdl 1
// script did not parse (#820). Control: the mdl 0 script given only the
// header builds another model.
func TestUpgradeOfExpressionStringEscapes(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()
	script := escapeScript("", escapeCases["mdl 0"])
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
