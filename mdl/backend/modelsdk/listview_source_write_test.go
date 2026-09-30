// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A ListView database source must serialize as Forms$ListViewXPathSource on the
// modelsdk engine (previously only microflow sources were supported, forcing
// --engine legacy).
func TestListViewSourceToGen_Database(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{
		EntityName:      "LvBug.Item",
		XPathConstraint: "[Rank > 5]",
		Sorting:         []*pages.GridSort{{AttributePath: "Name", Direction: "Ascending"}},
	})
	if err != nil {
		t.Fatalf("listViewSourceToGen(database): %v", err)
	}
	src, ok := el.(*genPg.ListViewXPathSource)
	if !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
	if src.XPathConstraint() != "[Rank > 5]" {
		t.Errorf("XPathConstraint = %q, want [Rank > 5]", src.XPathConstraint())
	}
	if src.EntityRef() == nil {
		t.Error("EntityRef must be set for a database source")
	}
	// Studio Pro requires the SortBar and Search sub-elements on a ListViewXPathSource.
	if src.SortBar() == nil {
		t.Error("SortBar must be set")
	}
	if src.Search() == nil {
		t.Error("Search must be set")
	}
}

// The encoded ListView database source must carry Search.SearchRefs. Without the
// Forms$ListViewSearch TypeDefaults registration the codec drops the empty,
// never-Set PartList, and the Mendix client crashes reading searchRefs.length in
// retrieveByXPath/processResult. Regression guard for that runtime crash.
func TestListViewSourceToGen_SearchRefsEmitted(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{EntityName: "LvBug.Item"})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	search, ok := lookup(d, "Search").(bson.D)
	if !ok {
		t.Fatalf("Search missing or not a document: %T", lookup(d, "Search"))
	}
	if lookup(search, "SearchRefs") == nil {
		t.Errorf("Search.SearchRefs must be emitted; Search had keys %v", dKeys(search))
	}
	sortBar, ok := lookup(d, "SortBar").(bson.D)
	if !ok {
		t.Fatalf("SortBar missing: %T", lookup(d, "SortBar"))
	}
	if lookup(sortBar, "SortItems") == nil {
		t.Errorf("SortBar.SortItems must be emitted; SortBar had keys %v", dKeys(sortBar))
	}
}

func lookup(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func dKeys(d bson.D) []string {
	ks := make([]string, 0, len(d))
	for _, e := range d {
		ks = append(ks, e.Key)
	}
	return ks
}

// Database source with no explicit sorting still produces a valid source (empty SortBar).
func TestListViewSourceToGen_DatabaseNoSort(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{EntityName: "LvBug.Item"})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	if _, ok := el.(*genPg.ListViewXPathSource); !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
}

// Microflow source still works (regression guard).
func TestListViewSourceToGen_Microflow(t *testing.T) {
	el, err := listViewSourceToGen(&pages.MicroflowSource{Microflow: "LvBug.DS_Items"})
	if err != nil {
		t.Fatalf("listViewSourceToGen(microflow): %v", err)
	}
	if _, ok := el.(*genPg.MicroflowSource); !ok {
		t.Fatalf("type = %T, want *genPg.MicroflowSource", el)
	}
}

// ako/mxcli#721 L5: a List View database source reached over an association
// from a snippet parameter is a Forms$ListViewXPathSource whose EntityRef is an
// IndirectEntityRef and whose SourceVariable names the snippet parameter — the
// shape Studio Pro stores in TestApp's WorkflowCommons snippets. It is not a
// Forms$AssociationSource: the XPath source keeps its sort and search bars.
func TestListViewSourceToGen_DatabaseOverAssociation(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{
		EntityName: "Administration.Account",
		EntitySteps: []pages.AttributeRefStep{{
			Association: "System.WorkflowUserTask_Assignees", DestinationEntity: "Administration.Account",
		}},
		ContextVariable:    "WorkflowUserTask",
		IsSnippetParameter: true,
		Sorting:            []*pages.GridSort{{AttributePath: "Administration.Account.FullName", Direction: "Ascending"}},
	})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := lookup(d, "$Type"); got != "Forms$ListViewXPathSource" {
		t.Fatalf("$Type = %v, want Forms$ListViewXPathSource", got)
	}
	ref, ok := lookup(d, "EntityRef").(bson.D)
	if !ok {
		t.Fatalf("EntityRef = %T", lookup(d, "EntityRef"))
	}
	if got := lookup(ref, "$Type"); got != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef $Type = %v, want DomainModels$IndirectEntityRef", got)
	}
	steps, _ := lookup(ref, "Steps").(bson.A)
	if len(steps) != 2 {
		t.Fatalf("Steps = %v, want a marker and one step", steps)
	}
	step := steps[1].(bson.D)
	if lookup(step, "Association") != "System.WorkflowUserTask_Assignees" || lookup(step, "DestinationEntity") != "Administration.Account" {
		t.Errorf("step = %v", step)
	}
	sv, ok := lookup(d, "SourceVariable").(bson.D)
	if !ok || lookup(sv, "SnippetParameter") != "WorkflowUserTask" {
		t.Errorf("SourceVariable = %v, want the snippet parameter WorkflowUserTask", lookup(d, "SourceVariable"))
	}
	if lookup(d, "SortBar") == nil || lookup(d, "Search") == nil {
		t.Errorf("SortBar/Search dropped; keys %v", dKeys(d))
	}
}

// encodeD encodes a gen element and reads it back as a bson.D.
func encodeD(t *testing.T, el element.Element) bson.D {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d
}

// lvListMarker is a stored list's leading typed-array marker.
func lvListMarker(t *testing.T, d bson.D, key string) any {
	t.Helper()
	a, ok := lookup(d, key).(bson.A)
	if !ok || len(a) == 0 {
		t.Fatalf("%s = %v, want a list with a marker; keys %v", key, lookup(d, key), dKeys(d))
	}
	return a[0]
}

// Studio Pro stores every list of a List View and its XPath source with marker
// 2, empty or not, and always writes the source's SourceVariable key (null when
// the source starts nowhere). Measured on the Studio Pro-authored fixtures:
// GridSortBar.SortItems 164/164, ListViewSearch.SearchRefs 44/44,
// ListView.Templates 45/45 at marker 2; ListViewXPathSource.SourceVariable
// present on 44/44. mxcli wrote marker 3 and omitted the null key, so executing
// the description of any Studio Pro list view rewrote it (ako/mxcli#721 L5).
func TestListViewSourceToGen_StoredListShape(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{
		EntityName:       "LvBug.Item",
		Sorting:          []*pages.GridSort{{AttributePath: "LvBug.Item.Name", Direction: "Ascending"}},
		SearchAttributes: []string{"LvBug.Item.Name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := encodeD(t, el)
	if lvListMarker(t, lookup(d, "SortBar").(bson.D), "SortItems") != int32(2) {
		t.Errorf("populated SortItems marker = %v, want 2", lvListMarker(t, lookup(d, "SortBar").(bson.D), "SortItems"))
	}
	if lvListMarker(t, lookup(d, "Search").(bson.D), "SearchRefs") != int32(2) {
		t.Errorf("populated SearchRefs marker = %v, want 2", lvListMarker(t, lookup(d, "Search").(bson.D), "SearchRefs"))
	}
	hasSV := false
	for _, e := range d {
		if e.Key == "SourceVariable" {
			hasSV = e.Value == nil
		}
	}
	if !hasSV {
		t.Errorf("SourceVariable must be written as null on an unbound source; keys %v", dKeys(d))
	}

	empty := encodeD(t, mustListViewSource(t, &pages.DatabaseSource{EntityName: "LvBug.Item"}))
	if m := lvListMarker(t, lookup(empty, "Search").(bson.D), "SearchRefs"); m != int32(2) {
		t.Errorf("empty SearchRefs marker = %v, want 2", m)
	}

	lv, err := widgetToGen(&pages.ListView{DataSource: &pages.DatabaseSource{EntityName: "LvBug.Item"}})
	if err != nil {
		t.Fatal(err)
	}
	if m := lvListMarker(t, encodeD(t, lv), "Templates"); m != int32(2) {
		t.Errorf("empty Templates marker = %v, want 2", m)
	}
}

func mustListViewSource(t *testing.T, ds pages.DataSource) element.Element {
	t.Helper()
	el, err := listViewSourceToGen(ds)
	if err != nil {
		t.Fatal(err)
	}
	return el
}
