// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 3: "A button's `OutputMappings` are not round-tripped."
// The empty list Studio Pro stores on every call has been written since
// 2026-10-10 (widget_write_mapping_markers_test.go). A NON-empty list — Mendix
// 11.5's mapping of a flow's result back onto the page — has no MDL, so
// describe → exec dropped it: "Replaced page", the mapping gone, mx check
// silent. The stored mappings are now carried while the button calls the same
// flow, and describe says the button has them.
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const om977Prelude = `create microflow MyFirstModule.ACT_Go () begin end;
create microflow MyFirstModule.ACT_Other () begin end;
`

func om977Page(flow string) string {
	return "create or modify page MyFirstModule.Go_Page (Title: 'Go', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  actionbutton btnGo (Caption: 'Go', Action: call microflow MyFirstModule." + flow + ")\n};\n"
}

// storedOutputMappings returns the Expression of every stored Forms$OutputMapping
// on the page.
func storedOutputMappings(t *testing.T, exec *Executor, page string) []string {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "$Type") == "Forms$OutputMapping" {
			s, _ := afGet(d, "Expression").(string)
			out = append(out, s)
		}
	})
	return out
}

// addOutputMapping BSON-patches one Forms$OutputMapping onto every
// OutputMappings list of the page — the Studio Pro 11.5 shape per the
// metamodel (Expression, AttributeRef, SourceAttributeRef, SourceVariable).
func addOutputMapping(t *testing.T, exec *Executor, page string) {
	t.Helper()
	id, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		for i := range d {
			if d[i].Key != "OutputMappings" {
				continue
			}
			owner := afGet(d, "$ID").(bson.Binary)
			data := append([]byte(nil), owner.Data...)
			data[len(data)-1] ^= 0x5A // a distinct element id
			d[i].Value = append(d[i].Value.(bson.A), bson.D{
				{Key: "$ID", Value: bson.Binary{Subtype: owner.Subtype, Data: data}},
				{Key: "$Type", Value: "Forms$OutputMapping"},
				{Key: "AttributeRef", Value: nil},
				{Key: "Expression", Value: "'mapped'"},
				{Key: "SourceAttributeRef", Value: nil},
				{Key: "SourceVariable", Value: nil},
			})
		}
	})
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}
}

func TestButtonOutputMappingsSurviveDescribeExec(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, "mdl 1;\n"+om977Prelude+om977Page("ACT_Go")); err != nil {
		t.Fatalf("exec: %v", err)
	}
	addOutputMapping(t, exec, "Go_Page")

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Go_Page;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, "-- btnGo: 1 output mapping") {
		t.Errorf("describe does not say the button has output mappings MDL cannot show:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	if got := storedOutputMappings(t, exec, "Go_Page"); len(got) != 1 || got[0] != "'mapped'" {
		t.Errorf("after describe → exec the output mappings are %q, want ['mapped']", got)
	}

	// Control: a button that now calls another microflow does not keep
	// mappings that read the old one's result.
	if err := afRun(t, exec, "mdl 1;\n"+om977Page("ACT_Other")); err != nil {
		t.Fatalf("exec with another microflow: %v", err)
	}
	if got := storedOutputMappings(t, exec, "Go_Page"); len(got) != 0 {
		t.Errorf("mappings for ACT_Go were carried onto a call of ACT_Other: %q", got)
	}
}
