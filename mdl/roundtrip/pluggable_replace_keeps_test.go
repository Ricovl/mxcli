// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1247: `alter page … replace <combobox> with { combobox … }`
// rebuilt the widget from its template, so every property MDL has no word for
// came back at the template's value. On TestApp's Studio Pro-authored
// Rules.BusinessRule_NewEdit, adding a sort to comboBox1's options turned its
// stored Editable "Never" into "Always" and an expression property's
// PrimitiveValue "" into "false". The replace now changes what the statement
// changes and nothing else, and running it again writes nothing.
func TestReplacePluggableKeepsWhatTheStatementDoesNotState(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	const replace = `mdl 1;
alter page Rules.BusinessRule_NewEdit {
  replace comboBox1 with {
    combobox comboBox1 (
      Label: 'Rule category',
      Attribute: Rules.BusinessRule_RuleCategory,
      DataSource: database from Rules.RuleCategory sort by Name asc,
      CaptionAttribute: Name
    )
  }
};`
	before := h.pageUnit(t, "BusinessRule_NewEdit")
	storedType, storedObj := pluggableParts(t, before, "comboBox1")
	// Control: the stored widget carries a value the template does not, so the
	// assertions below can fail.
	if got := pluggableField(t, before, "comboBox1", "Editable"); got != "Never" {
		t.Fatalf("control: comboBox1's stored Editable is %q, the fixture changed", got)
	}

	if err := h.exec(replace); err != nil {
		t.Fatalf("exec: %v", err)
	}
	after := h.pageUnit(t, "BusinessRule_NewEdit")
	typ, obj := pluggableParts(t, after, "comboBox1")
	if got := pluggableField(t, after, "comboBox1", "Editable"); got != "Never" {
		t.Errorf("Editable = %q after the replace, want the stored \"Never\"", got)
	}
	if !bytes.Equal(typ, storedType) {
		t.Errorf("the stored Type was replaced by the template's")
	}
	diff := bsonDiff(storedObj, obj)
	if len(diff) == 0 {
		t.Fatal("the replace changed nothing — the sort was not written")
	}
	for _, d := range diff {
		if !strings.Contains(d, "SortItems") {
			t.Errorf("a property the statement does not state changed: %s", d)
		}
	}

	// Twice-exec: the same statement again writes nothing.
	again := h.snapshot()
	if err := h.exec(replace); err != nil {
		t.Fatalf("second exec: %v", err)
	}
	if changed := again.diff(h.snapshot()); len(changed) > 0 {
		t.Errorf("running the replace a second time wrote:\n  %s", strings.Join(changed, "\n  "))
	}
}

// pluggableField returns a string field of the pluggable widget named name.
func pluggableField(t *testing.T, unit []byte, name, field string) string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(unit, &doc); err != nil {
		t.Fatal(err)
	}
	var got string
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case bson.D:
			isCW, n := false, ""
			for _, e := range x {
				switch e.Key {
				case "$Type":
					isCW = e.Value == "CustomWidgets$CustomWidget"
				case "Name":
					n, _ = e.Value.(string)
				}
			}
			if isCW && n == name {
				for _, e := range x {
					if e.Key == field {
						got, _ = e.Value.(string)
					}
				}
				return true
			}
			for _, e := range x {
				if walk(e.Value) {
					return true
				}
			}
		case bson.A:
			for _, e := range x {
				if walk(e) {
					return true
				}
			}
		}
		return false
	}
	if !walk(doc) {
		t.Fatalf("no pluggable widget %s", name)
	}
	return got
}
