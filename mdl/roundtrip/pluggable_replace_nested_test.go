// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1247, the nested case: a DataGrid 2 stores its columns as
// an object list, and each column's filter is a pluggable widget of its own.
// Restating TestApp's Studio Pro-authored dataGrid2_1 with one column caption
// changed fell back to the template rebuild — the filter widgets' type
// pointers aim into their OWN Type, which the graft could not re-aim — and
// silently turned itemSelectionMethod "rowClick" into "checkbox", reset
// onClickTrigger, and rebuilt every column's unmapped properties. Only the
// caption the statement changes may change, and running it again writes
// nothing.
func TestReplaceDataGridKeepsWhatTheStatementDoesNotState(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	described, err := h.describe("page Rules.BusinessRule_Overview")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	lines := strings.Split(described, "\n")
	start, end := -1, -1
	for i, l := range lines {
		if start < 0 && strings.Contains(l, "datagrid dataGrid2_1") {
			start = i
			continue
		}
		if start >= 0 {
			indent := lines[start][:len(lines[start])-len(strings.TrimLeft(lines[start], " "))]
			if l == indent+"}" {
				end = i
				break
			}
		}
	}
	if start < 0 || end < 0 {
		t.Fatalf("dataGrid2_1 not found in the description:\n%s", described)
	}
	grid := strings.Join(lines[start:end+1], "\n")
	const from, to = "Caption: 'Name'", "Caption: 'Rule name'"
	if !strings.Contains(grid, from) {
		t.Fatalf("control: the fixture's first column caption changed:\n%s", grid)
	}
	replace := "mdl 1;\nalter page Rules.BusinessRule_Overview {\n  replace dataGrid2_1 with {\n" +
		strings.Replace(grid, from, to, 1) + "\n  }\n};"

	before := h.pageUnit(t, "BusinessRule_Overview")
	storedType, storedObj := pluggableParts(t, before, "dataGrid2_1")
	if err := h.exec(replace); err != nil {
		t.Fatalf("exec: %v", err)
	}
	after := h.pageUnit(t, "BusinessRule_Overview")
	typ, obj := pluggableParts(t, after, "dataGrid2_1")
	if string(typ) != string(storedType) {
		t.Errorf("the stored Type was replaced by the template's")
	}
	diff := bsonDiff(storedObj, obj)
	if len(diff) == 0 {
		t.Fatal("the replace changed nothing — the caption was not written")
	}
	for _, d := range diff {
		if !strings.Contains(d, "Rule name") {
			t.Errorf("a property the statement does not state changed: %s", d)
		}
	}

	again := h.snapshot()
	if err := h.exec(replace); err != nil {
		t.Fatalf("second exec: %v", err)
	}
	if changed := again.diff(h.snapshot()); len(changed) > 0 {
		t.Errorf("running the replace a second time wrote:\n  %s", strings.Join(changed, "\n  "))
	}
}
