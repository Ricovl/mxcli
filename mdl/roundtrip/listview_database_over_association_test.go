// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#721 L5. Studio Pro-authored List Views whose "Database" source is
// reached from a snippet parameter over an association: a
// Forms$ListViewXPathSource with an IndirectEntityRef, a sort bar and a search
// bar. Describe printed the association-source spelling, and executing it
// re-typed the source as a Forms$AssociationSource — an in-memory retrieve with
// no XPath, sort or search. On Snip_UserTask_Assignees the destination entity
// (Administration.Account) also became the association's own end (System.User).
var listViewDatabaseOverAssociationCases = []struct {
	target string
	unit   string // the snippet's unit ID
	from   string // what describe must print after `database from `
}{
	{"snippet WorkflowCommons.Snip_UserTask_Assignees", "36e812c0-75a4-4aa0-b57a-21cc74660918",
		"$WorkflowUserTask/System.WorkflowUserTask_Assignees/Administration.Account sort by FullName asc"},
	{"snippet WorkflowCommons.Snip_TaskAssignment_UserTask_Assignees", "4361f66b-5c2a-4c14-b8d5-6b5471c1c69f",
		"$WorkflowUserTask/System.WorkflowUserTask_Assignees/Administration.Account sort by FullName asc"},
	{"snippet WorkflowCommons.Snip_UserTask_TargetGroups", "ad69ab4d-b35e-425a-bd33-d99bde393618",
		"$WorkflowUserTask/System.WorkflowUserTask_TargetGroups/System.WorkflowGroup sort by Name asc"},
}

// The whole datasource — $Type, EntityRef steps, SourceVariable, XPath, sort
// bar and search bar, $IDs included — is written back as stored. Key order is
// not compared: a unit rewritten for another reason is re-encoded in the
// codec's order.
func TestListViewDatabaseOverAssociationKeepsItsSource(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()
	for _, c := range listViewDatabaseOverAssociationCases {
		t.Run(c.target, func(t *testing.T) {
			h.restore()
			script := h.mustDescribeMdl0(t, c.target)
			if !strings.Contains(script, "DataSource: database from "+c.from) {
				t.Fatalf("describe does not print `database from %s`:\n%s", c.from, script)
			}
			before := firstListViewSource(t, h.unitBytes(t, c.unit))
			if err := h.exec(script); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, script)
			}
			after := firstListViewSource(t, h.unitBytes(t, c.unit))
			if !sameDocument(t, before, after) {
				t.Errorf("the list view's datasource changed:\n  stored  %s\n  written %s",
					bson.Raw(before), bson.Raw(after))
			}
		})
	}
}

// Control: the association spelling describe used to print re-types the
// source, so the comparison above can fail.
func TestListViewDatabaseOverAssociationKeepsItsSource_Control(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()
	c := listViewDatabaseOverAssociationCases[2]
	script := h.mustDescribeMdl0(t, c.target)
	old := strings.Replace(script,
		"DataSource: database from "+c.from,
		"DataSource: $WorkflowUserTask/System.WorkflowUserTask_TargetGroups", 1)
	if old == script {
		t.Fatalf("the control found nothing to rewrite:\n%s", script)
	}
	before := firstListViewSource(t, h.unitBytes(t, c.unit))
	if err := h.exec(old); err != nil {
		t.Fatalf("exec: %v", err)
	}
	after := firstListViewSource(t, h.unitBytes(t, c.unit))
	if sameDocument(t, before, after) {
		t.Fatal("the association spelling kept the database source — the comparison cannot fail")
	}
	var d bson.D
	if err := bson.Unmarshal(after, &d); err != nil {
		t.Fatal(err)
	}
	if got := bsonString(d, "$Type"); got != "Forms$AssociationSource" {
		t.Errorf("the association spelling wrote %s, want Forms$AssociationSource", got)
	}
}

// sameDocument compares two BSON documents by content, ignoring key order.
func sameDocument(t *testing.T, a, b []byte) bool {
	t.Helper()
	var da, db bson.D
	if err := bson.Unmarshal(a, &da); err != nil {
		t.Fatal(err)
	}
	if err := bson.Unmarshal(b, &db); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(unordered(da), unordered(db))
}

// unordered turns every document in v into a map, so key order drops out.
func unordered(v any) any {
	switch x := v.(type) {
	case bson.D:
		m := make(map[string]any, len(x))
		for _, e := range x {
			m[e.Key] = unordered(e.Value)
		}
		return m
	case bson.A:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = unordered(e)
		}
		return out
	}
	return v
}

// firstListViewSource returns the raw DataSource of the first Forms$ListView in
// a unit, depth first.
func firstListViewSource(t *testing.T, unit []byte) []byte {
	t.Helper()
	var find func(v bson.Raw) []byte
	find = func(doc bson.Raw) []byte {
		if typ, ok := doc.Lookup("$Type").StringValueOK(); ok && typ == "Forms$ListView" {
			if ds, ok := doc.Lookup("DataSource").DocumentOK(); ok {
				return ds
			}
		}
		elems, err := doc.Elements()
		if err != nil {
			return nil
		}
		for _, e := range elems {
			v := e.Value()
			switch v.Type {
			case bson.TypeEmbeddedDocument:
				if got := find(v.Document()); got != nil {
					return got
				}
			case bson.TypeArray:
				vals, _ := v.Array().Values()
				for _, av := range vals {
					if av.Type == bson.TypeEmbeddedDocument {
						if got := find(av.Document()); got != nil {
							return got
						}
					}
				}
			}
		}
		return nil
	}
	ds := find(bson.Raw(unit))
	if ds == nil {
		t.Fatal("no list view datasource in the unit")
	}
	return ds
}

// Only the List View writer stores the path. A datasource property the widget
// engine reaches under its own key — here a combo box's
// optionsSourceAssociationDataSource — bypassed the `DataSource:` guard: exec
// reported "Created page" and the pluggable writer stored the entity alone, a
// retrieve of every row instead of the context's (describe read it back as
// `database from Rules.BusinessRule`). Every non-List View call site of the
// datasource builder must refuse it.
func TestDatabaseOverAssociationRefusedUnderANamedDatasourceKey(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()
	h.restore()
	err := h.exec(`create page Rules.L5NamedKeyProbe (
  Title: 'Probe',
  Layout: Atlas_Core.PopupLayout,
  Params: ( $RuleCategory: Rules.RuleCategory, $Rule: Rules.RuleAction )
) {
  dataview dv1 (DataSource: $Rule) {
    combobox cb1 (
      Label: 'Business rule',
      Attribute: Rules.RuleAction_BusinessRule,
      optionsSourceAssociationDataSource: database from $RuleCategory/Rules.BusinessRule_RuleCategory/Rules.BusinessRule,
      CaptionAttribute: Name
    )
  }
};`)
	if err == nil {
		t.Fatal("a combo box's named datasource accepted `database from $ctx/Assoc/Entity` — " +
			"its writer stores the entity alone, so every row would be retrieved")
	}
	if !strings.Contains(err.Error(), "list view") && !strings.Contains(err.Error(), "LIST VIEW") {
		t.Errorf("refused for another reason: %v", err)
	}
}
