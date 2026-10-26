// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 2: "The text area's `NumberOfLines` and `Autocomplete` are
// not round-tripped." describe printed neither, nothing parsed either, and the
// writer wrote 5 lines and no Autocomplete whatever was stored — so a Studio
// Pro text area of 2 lines (4 in PedApp and TestApp) or 0 lines (2) came back
// as 5.
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func storedTextArea(t *testing.T, exec *Executor, page, widget string) (lines any, autocomplete any) {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "Name") == widget {
			lines, autocomplete = afGet(d, "NumberOfLines"), afGet(d, "Autocomplete")
		}
	})
	return lines, autocomplete
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	}
	return -1
}

const ta977Prelude = `create persistent entity MyFirstModule.Note (Body: String(unlimited));
`

func ta977Page(name, props string) string {
	return "create or modify page MyFirstModule." + name + " (Title: 'Note', Layout: Atlas_Core.Atlas_Default, Params: ($Note: MyFirstModule.Note)) {\n" +
		"  dataview dvNote (DataSource: $Note) {\n" +
		"    textarea textArea1 (Attribute: Body" + props + ")\n" +
		"  }\n};\n"
}

func TestTextAreaNumberOfLinesAndAutocomplete_Authored(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	if err := afRun(t, exec, ta977Prelude+ta977Page("Note_Edit", ", NumberOfLines: 2, Autocomplete: false")); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if lines, auto := storedTextArea(t, exec, "Note_Edit", "textArea1"); asInt(lines) != 2 || auto != false {
		t.Errorf("stored NumberOfLines %v, Autocomplete %v; want 2, false", lines, auto)
	}
	if err := afRun(t, exec, "alter page MyFirstModule.Note_Edit {\n  set (NumberOfLines: 0, Autocomplete: true) on textArea1\n};"); err != nil {
		t.Fatalf("alter page set: %v", err)
	}
	if lines, auto := storedTextArea(t, exec, "Note_Edit", "textArea1"); asInt(lines) != 0 || auto != true {
		t.Errorf("after alter set: NumberOfLines %v, Autocomplete %v; want 0, true", lines, auto)
	}
}

// The reported round trip, on a text area whose stored values are Studio
// Pro's: 0 lines and Autocomplete off.
func TestTextAreaNumberOfLinesAndAutocomplete_StudioProValuesSurviveDescribeExec(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, ta977Prelude+ta977Page("Note_View", "")); err != nil {
		t.Fatalf("exec: %v", err)
	}
	id, raw := pageUnitByName(t, exec, "Note_View")
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "Name") != "textArea1" {
			return
		}
		for i := range d {
			switch d[i].Key {
			case "NumberOfLines":
				d[i].Value = int64(0) // Studio Pro's width
			case "Autocomplete":
				d[i].Value = false
			}
		}
	})
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Note_View;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	for _, want := range []string{"NumberOfLines: 0", "Autocomplete: false"} {
		if !strings.Contains(described, want) {
			t.Errorf("describe dropped %q:\n%s", want, described)
		}
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	if lines, auto := storedTextArea(t, exec, "Note_View", "textArea1"); asInt(lines) != 0 || auto != false {
		t.Errorf("after describe → exec: NumberOfLines %v, Autocomplete %v; want 0, false", lines, auto)
	}
}

// NumberOfLines is stored as a 32-bit integer, so a larger value is refused
// rather than truncated on the way to the writer.
func TestNonNegativeIntBounds(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int
		ok   bool
	}{
		{2, 2, true}, {0, 0, true}, {"7", 7, true}, {float64(3), 3, true},
		{-1, 0, false}, {"x", 0, false}, {1.5, 0, false},
		{"2147483647", 2147483647, true}, {"2147483648", 0, false}, {int64(1) << 40, 0, false},
	} {
		got, ok := nonNegativeInt(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("nonNegativeInt(%v) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
