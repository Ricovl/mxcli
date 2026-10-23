// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1326 (mendixlabs/mxcli#1453), the reporter's last row: CREATE
// accepts `group grp1 (…)` and drops the name, because an Accordion group
// stores none, so `insert into grp1 { … }` misses. The miss on a page that also
// held a data grid answered
//
//	widget "grp1" not found. DataGrid2 columns are addressed by a derived name
//	(the bound attribute, or the caption), not the name written in MDL —
//	available columns: Name (run DESCRIBE PAGE to confirm)
//
// which sends the author to the grid. The group IS addressable — `acc1 item@1`,
// `acc1 item('One')` — and the miss has to say so.

// realGrid is gridWithCaptionColumns with its `columns` declared the way the
// shipped DataGrid 2 declares it — an Object list — so the miss has to tell
// the grid's columns from an item list rather than miss them by shape.
func realGrid(name string, captions ...string) bson.D {
	g := gridWithCaptionColumns(name, captions...)
	pt := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(bsonnav.DGetDoc(g, "Type"), "ObjectType"), "PropertyTypes"))[0].(bson.D)
	vt := bsonnav.DGetDoc(pt, "ValueType")
	vt = append(bson.D{{Key: "Type", Value: "Object"}, {Key: "IsList", Value: true}}, vt...)
	bsonnav.DSet(pt, "ValueType", vt)
	return g
}

func groupAndGridPage() bson.D {
	return makeRawPage(
		accordionWithGroups(
			accordionGroup("One", nil, []string{"btnInGroup"}),
			accordionGroup("Two", nil, nil),
		),
		realGrid("dg1", "Name"),
	)
}

func TestWidgetNotFound_NamesTheItemAddresses(t *testing.T) {
	for _, tc := range []struct {
		op  string
		run func(m *Mutator) error
	}{
		{"insert into", func(m *Mutator) error {
			return m.InsertWidget("grp1", "", backend.InsertPosition("into"), []pages.Widget{&pages.DynamicText{}})
		}},
		{"set", func(m *Mutator) error { return m.SetWidgetProperty("grp1", "headerText", "x") }},
		{"drop", func(m *Mutator) error { return m.DropWidget([]backend.WidgetRef{{Widget: "grp1"}}) }},
	} {
		t.Run(tc.op, func(t *testing.T) {
			m := New(groupAndGridPage(), model.ID("u"), &stubWidgetDeps{})
			err := tc.run(m)
			if err == nil {
				t.Fatal("grp1 resolved, but no widget or item on the page is named grp1")
			}
			msg := err.Error()
			for _, want := range []string{
				`widget "grp1" not found`,
				// what would hit, spelled as the author types it
				"acc1 item@1", "acc1 item('One')", "acc1 item@2", "acc1 item('Two')",
				// and the grid's columns are still listed
				"available columns: Name",
			} {
				if !strings.Contains(msg, want) {
					t.Errorf("miss does not name %q:\n%s", want, msg)
				}
			}
			if strings.Contains(msg, "dg1 item@") {
				t.Errorf("the grid's columns are addressed by name, not as items:\n%s", msg)
			}
		})
	}
}

// Without an item list on the page, the miss stays as it was.
func TestWidgetNotFound_NoItemListNoItemHint(t *testing.T) {
	m := New(makeRawPage(realGrid("dg1", "Name")), model.ID("u"), &stubWidgetDeps{})
	err := m.SetWidgetProperty("grp1", "Caption", "x")
	if err == nil {
		t.Fatal("grp1 resolved on a page that has no grp1")
	}
	if strings.Contains(err.Error(), "item@") {
		t.Errorf("item addresses offered on a page with no item list:\n%s", err)
	}
	if !strings.Contains(err.Error(), "available columns: Name") {
		t.Errorf("columns no longer listed:\n%s", err)
	}
}
