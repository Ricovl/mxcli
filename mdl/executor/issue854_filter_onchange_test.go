// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#854: a data grid filter's `onChange` had no mapping, so MDL could
// not write it and describe did not print it — a Studio Pro-authored filter
// action was at the mercy of every rewrite of its page.
//
// mxcli could not write the action at all, so these tests seed it: a
// Forms$MicroflowAction taken from a combo box's onChangeEvent, which MDL does
// write, transplanted into the filter's onChange.

const issue854Setup = `create enumeration MyFirstModule.Colour (Red 'Red', Blue 'Blue');
create persistent entity MyFirstModule.Chore (Title: String(100), Colour: Enumeration(MyFirstModule.Colour));
create microflow MyFirstModule.ACT_Filtered () begin end;
create page MyFirstModule.Donor (Title: 'Donor', Layout: Atlas_Core.Atlas_Default, Params: ($Chore: MyFirstModule.Chore)) {
  dataview dvChore (DataSource: $Chore) {
    combobox cbDonor (Label: 'Colour', Attribute: Colour, OnChange: call microflow MyFirstModule.ACT_Filtered())
  }
};`

const issue854Page = `create page MyFirstModule.Chores (Title: 'Chores', Layout: Atlas_Core.Atlas_Default) {
  datagrid dgChores (DataSource: database from MyFirstModule.Chore) {
    column (Attribute: Title, Caption: 'Title') {
      textfilter tfTitle
    }
  }
};`

// donorAction is the microflow call MDL stored on the donor combo box.
func donorAction(t *testing.T, exec *Executor) bson.D {
	t.Helper()
	donor := unitIDByName(t, exec, "page", "Donor")
	act := pluggableSlotAction(t, namedDoc(t, storedUnitDoc(t, exec, donor), "cbDonor"), "onChangeEvent")
	if got := bsonnav.DGetString(act, "$Type"); got != "Forms$MicroflowAction" {
		t.Fatalf("donor combo box stores %s, want Forms$MicroflowAction", got)
	}
	return act
}

// seedFilterOnChange gives tfTitle the onChange Studio Pro would store.
func seedFilterOnChange(t *testing.T, exec *Executor, id string) {
	t.Helper()
	act := donorAction(t, exec)
	doc := storedUnitDoc(t, exec, id)
	tf := namedDoc(t, doc, "tfTitle")
	slot := pluggableSlotAction(t, tf, "onChange")
	if got := bsonnav.DGetString(slot, "$Type"); got != "Forms$NoAction" {
		t.Fatalf("an mxcli-written filter's onChange holds %s, want Forms$NoAction", got)
	}
	setPluggableSlotAction(t, tf, "onChange", act)
	writeUnitDoc(t, exec, id, doc)
	if got := filterOnChangeType(t, exec, id); got != "Forms$MicroflowAction" {
		t.Fatalf("seed did not take: onChange holds %s", got)
	}
}

// setPluggableSlotAction replaces the action a pluggable widget stores under key.
func setPluggableSlotAction(t *testing.T, widget bson.D, key string, action bson.D) {
	t.Helper()
	var typeID string
	for _, pt := range bsonDocs(bsonValue(bsonDocField(bsonDocField(widget, "Type"), "ObjectType"), "PropertyTypes")) {
		if bsonString(pt, "PropertyKey") == key {
			typeID = bsonIDKey(bsonValue(pt, "$ID"))
		}
	}
	for _, prop := range bsonDocs(bsonValue(bsonDocField(widget, "Object"), "Properties")) {
		if typeID != "" && bsonIDKey(bsonValue(prop, "TypePointer")) == typeID {
			bsonnav.DSet(bsonDocField(prop, "Value"), "Action", action)
			return
		}
	}
	t.Fatalf("widget stores no property %s", key)
}

func filterOnChangeType(t *testing.T, exec *Executor, id string) string {
	t.Helper()
	tf := namedDoc(t, storedUnitDoc(t, exec, id), "tfTitle")
	return bsonnav.DGetString(pluggableSlotAction(t, tf, "onChange"), "$Type")
}

func filterOnChangeFlow(t *testing.T, exec *Executor, id string) string {
	t.Helper()
	tf := namedDoc(t, storedUnitDoc(t, exec, id), "tfTitle")
	return bsonnav.DGetString(bsonnav.DGetDoc(pluggableSlotAction(t, tf, "onChange"), "MicroflowSettings"), "Microflow")
}

// describeFilterPage is describe's output for the Chores page.
func describeFilterPage(t *testing.T, exec *Executor, out interface {
	Reset()
	String() string
}) string {
	t.Helper()
	out.Reset()
	run(t, exec, "describe page MyFirstModule.Chores")
	return out.String()
}

// withFilterType gives tfTitle a filter type, whether or not describe printed a
// property list for it.
func withFilterType(described string) string {
	if strings.Contains(described, "textfilter tfTitle (") {
		return strings.Replace(described, "textfilter tfTitle (", "textfilter tfTitle (FilterType: startsWith, ", 1)
	}
	return strings.Replace(described, "textfilter tfTitle", "textfilter tfTitle (FilterType: startsWith)", 1)
}

// The filter itself edited and rewritten from describe's output: the stored
// onChange survives. This held before the mapping already, through the
// per-property patch of ako/mxcli#832 — neither build wrote the slot, so both
// agreed on it and the stored value was kept. Stubbing that patch fails this
// test with "editing tfTitle reset its onChange to Forms$NoAction".
func TestIssue854_EditedFilterKeepsItsStoredOnChange(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854Setup)
	run(t, exec, issue854Page)
	id := unitIDByName(t, exec, "page", "Chores")
	seedFilterOnChange(t, exec, id)

	stmt := withFilterType(describeFilterPage(t, exec, out))
	run(t, exec, stmt)
	if !storedHas(t, exec, id, "startsWith") {
		t.Fatalf("the rewrite did not land, so it shows nothing about the filter:\n%s", stmt)
	}
	if got := filterOnChangeType(t, exec, id); got != "Forms$MicroflowAction" {
		t.Errorf("editing tfTitle reset its onChange to %s", got)
	}
}

// A sibling edited: the grid is patched, the filter passed through as stored.
func TestIssue854_SiblingEditKeepsTheFiltersStoredOnChange(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854Setup)
	run(t, exec, issue854Page)
	id := unitIDByName(t, exec, "page", "Chores")
	seedFilterOnChange(t, exec, id)

	described := describeFilterPage(t, exec, out)
	run(t, exec, strings.Replace(described, "Caption: 'Title'", "Caption: 'Chore title'", 1))
	if !storedHas(t, exec, id, "Chore title") {
		t.Fatal("the rewrite did not land")
	}
	if got := filterOnChangeType(t, exec, id); got != "Forms$MicroflowAction" {
		t.Errorf("rewriting the grid reset tfTitle's onChange to %s:\n%s", got, described)
	}
}

// What the mapping adds: describe says what the stored filter runs, so a
// statement can keep, retarget or remove it.
func TestIssue854_StoredFilterOnChangeIsDescribed(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854Setup)
	run(t, exec, issue854Page)
	id := unitIDByName(t, exec, "page", "Chores")
	seedFilterOnChange(t, exec, id)

	described := describeFilterPage(t, exec, out)
	if line := lineContaining(described, "textfilter tfTitle"); !strings.Contains(line, "OnChange: call microflow MyFirstModule.ACT_Filtered") {
		t.Fatalf("tfTitle is described without its onChange: %q", line)
	}
	out.Reset()
	run(t, exec, described)
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-running describe rewrote the page:\n%s", out.String())
	}
}

func TestIssue854_StatementRetargetsAndRemovesTheFiltersOnChange(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854Setup)
	run(t, exec, `create microflow MyFirstModule.ACT_Other () begin end;`)
	run(t, exec, issue854Page)
	id := unitIDByName(t, exec, "page", "Chores")
	seedFilterOnChange(t, exec, id)

	described := describeFilterPage(t, exec, out)
	run(t, exec, strings.Replace(described, "MyFirstModule.ACT_Filtered", "MyFirstModule.ACT_Other", 1))
	if got := filterOnChangeFlow(t, exec, id); got != "MyFirstModule.ACT_Other" {
		t.Errorf("retargeting tfTitle's onChange stored %q", got)
	}

	described = describeFilterPage(t, exec, out)
	line := lineContaining(described, "textfilter tfTitle")
	run(t, exec, strings.Replace(described, line, "      textfilter tfTitle", 1))
	if got := filterOnChangeType(t, exec, id); got != "Forms$NoAction" {
		t.Errorf("a statement without tfTitle's onChange kept %s", got)
	}
}
