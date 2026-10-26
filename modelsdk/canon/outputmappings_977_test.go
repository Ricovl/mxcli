// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#977 item 3: a button's output mappings (Mendix 11.5) have no MDL, so
// a rebuild writes the empty list and describe → exec dropped every mapping
// Studio Pro stored — "Replaced page", the mapping gone, mx check silent.
// CarryOutputMappings keeps them while the button still calls the same flow.

func outputMapping(id byte, expr string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bin(id)},
		{Key: "$Type", Value: "Forms$OutputMapping"},
		{Key: "AttributeRef", Value: nil},
		{Key: "Expression", Value: expr},
		{Key: "SourceAttributeRef", Value: nil},
		{Key: "SourceVariable", Value: nil},
	}
}

// buttonPage is a page with one button named name whose action calls flow
// with the given output mappings. nanoflow selects a CallNanoflowClientAction,
// otherwise a MicroflowClientAction with MicroflowSettings.
func buttonPage(t *testing.T, base byte, name, flow string, nanoflow bool, mappings ...bson.D) []byte {
	t.Helper()
	list := bson.A{int32(3)}
	for _, m := range mappings {
		list = append(list, m)
	}
	var action bson.D
	if nanoflow {
		action = bson.D{
			{Key: "$ID", Value: bin(base + 2)},
			{Key: "$Type", Value: "Forms$CallNanoflowClientAction"},
			{Key: "Nanoflow", Value: flow},
			{Key: "OutputMappings", Value: list},
		}
	} else {
		action = bson.D{
			{Key: "$ID", Value: bin(base + 2)},
			{Key: "$Type", Value: "Forms$MicroflowClientAction"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: bin(base + 3)},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: flow},
				{Key: "OutputMappings", Value: list},
			}},
		}
	}
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Name", Value: "Go_Page"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bin(base + 1)},
			{Key: "$Type", Value: "Forms$ActionButton"},
			{Key: "Name", Value: name},
			{Key: "Action", Value: action},
		}}},
	})
}

// outputMappingExprs lists the Expression of every Forms$OutputMapping.
func outputMappingExprs(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if docLookup(n, "$Type") == "Forms$OutputMapping" {
				s, _ := docLookup(n, "Expression").(string)
				out = append(out, s)
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func TestCarryOutputMappings(t *testing.T) {
	for _, nanoflow := range []bool{false, true} {
		stored := buttonPage(t, 10, "btnGo", "M.ACT_Go", nanoflow, outputMapping(40, "'a'"), outputMapping(41, "'b'"))
		for _, tc := range []struct {
			name    string
			rebuilt []byte
			want    int
		}{
			// The reported round trip: the rebuild wrote the empty list.
			{"same button, same flow", buttonPage(t, 20, "btnGo", "M.ACT_Go", nanoflow), 2},
			// Controls: what the statement said wins.
			{"the flow changed", buttonPage(t, 20, "btnGo", "M.ACT_Other", nanoflow), 0},
			{"the statement wrote its own", buttonPage(t, 20, "btnGo", "M.ACT_Go", nanoflow, outputMapping(50, "'mine'")), 1},
			{"another button", buttonPage(t, 20, "btnOther", "M.ACT_Go", nanoflow), 0},
		} {
			got := outputMappingExprs(t, CarryOutputMappings(tc.rebuilt, stored))
			if len(got) != tc.want {
				t.Errorf("nanoflow=%v %s: output mappings %v, want %d", nanoflow, tc.name, got, tc.want)
			}
		}
		// The whole point: an unchanged button is an unchanged document.
		if _, unchanged := Reconcile(buttonPage(t, 20, "btnGo", "M.ACT_Go", nanoflow), stored); !unchanged {
			t.Errorf("nanoflow=%v: a rebuild that only dropped the output mappings was written", nanoflow)
		}
	}
}
