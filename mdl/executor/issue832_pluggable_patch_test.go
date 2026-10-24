// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#832: a pluggable widget the statement changes was rebuilt whole
// from the template, so every property it did not mention lost what Studio Pro
// had stored. And a widget the statement did NOT change was rebuilt too
// whenever describe printed it imperfectly: on ako/TestApp, re-running
// `describe page WorkflowCommons.TaskInbox` unchanged rebuilt both data grids,
// turning the Assignee(s) column's Administration.Account.FullName into
// System.User.FullName (CE1613) and the row click into a NoAction. A layout
// rewrite kept no stored pluggable widget at all.
//
// mxcli writes the template's value on every property it creates, so these
// tests seed what a Studio Pro-authored widget holds; without the seed a
// rebuild resetting it is undetectable. (A second translation is not a seed:
// canon.CarryTranslations already restores those.)

const issue832Setup = `create enumeration MyFirstModule.Colour (Red 'Red', Blue 'Blue');
create persistent entity MyFirstModule.Thing (Colour: Enumeration(MyFirstModule.Colour), Name: String(100));`

const issue832ComboPage = `create or modify page MyFirstModule.ThingEdit (Title: 'Thing', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvThing (DataSource: $Thing) {
    combobox cbColour (Label: 'Colour', Attribute: Colour, emptyOptionText: 'Pick one')
  }
};`

const issue832GridSetup = `create persistent entity MyFirstModule.Chore (Title: String(100));
create association MyFirstModule.Chore_Assignee from MyFirstModule.Chore to System.User;
create page MyFirstModule.Chores (Title: 'Chores', Layout: Atlas_Core.Atlas_Default) {
  datagrid dgChores (DataSource: database from MyFirstModule.Chore) {
    column (Attribute: Title, Caption: 'Title')
    column (Attribute: Chore_Assignee/Name, Caption: 'Assignee')
  }
};`

const issue832Layout = `create or modify layout MyFirstModule.MyLayout (layouttype: 'Responsive') {
  image imgLogo (ImageType: 'imageUrl', ImageUrl: 'https://example.com/logo.png')
  placeholder Main
};`

// objectType is a stored pluggable widget's top-level object type.
func objectType(widget bson.D) bson.D {
	return bsonDocField(bsonDocField(widget, "Type"), "ObjectType")
}

// nestedObjectType is the object type of an object-list property of ot.
func nestedObjectType(t *testing.T, ot bson.D, key string) bson.D {
	t.Helper()
	for _, pt := range bsonDocs(bsonValue(ot, "PropertyTypes")) {
		if bsonString(pt, "PropertyKey") == key {
			return bsonDocField(bsonDocField(pt, "ValueType"), "ObjectType")
		}
	}
	t.Fatalf("no object-list property %s", key)
	return nil
}

// propValue is the value object obj stores for the property key of ot.
func propValue(t *testing.T, ot, obj bson.D, key string) bson.D {
	t.Helper()
	ids := propertyKeysByTypeID(bson.D{{Key: "ObjectType", Value: ot}})
	for _, prop := range bsonDocs(bsonValue(obj, "Properties")) {
		if ids[bsonIDKey(bsonValue(prop, "TypePointer"))] == key {
			return bsonDocField(prop, "Value")
		}
	}
	t.Fatalf("no stored property %s", key)
	return nil
}

func widgetProp(t *testing.T, widget bson.D, key string) bson.D {
	t.Helper()
	return propValue(t, objectType(widget), bsonDocField(widget, "Object"), key)
}

// openUserTask is a row or click action MDL has no spelling for, as Studio Pro
// stores it.
func openUserTask() bson.D {
	return bson.D{
		{Key: "$ID", Value: bson.Binary{Data: []byte("0123456789abcdef")}},
		{Key: "$Type", Value: "Forms$OpenUserTaskClientAction"},
		{Key: "AssignOnOpen", Value: true},
		{Key: "DisabledDuringExecution", Value: true},
		{Key: "OpenWhenAssigned", Value: false},
	}
}

// describedThenEdited is describe's own output for the document, with one
// edit — what a user runs to change a Studio Pro page.
func describedThenEdited(t *testing.T, exec *Executor, out interface {
	Reset()
	String() string
}, what, old, new string) string {
	t.Helper()
	out.Reset()
	run(t, exec, "describe "+what)
	described := out.String()
	if !strings.Contains(described, old) {
		t.Fatalf("describe output has no %q:\n%s", old, described)
	}
	return strings.Replace(described, old, new, 1)
}

func TestIssue832_EditedComboBoxKeepsAPropertyItDidNotName(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832Setup)
	run(t, exec, issue832ComboPage)
	id := unitIDByName(t, exec, "page", "ThingEdit")

	// Studio Pro stores no primitive value on the expression property; the
	// template writes "false".
	doc := storedUnitDoc(t, exec, id)
	bsonnav.DSet(widgetProp(t, namedDoc(t, doc, "cbColour"), "customEditabilityExpression"), "PrimitiveValue", "")
	writeUnitDoc(t, exec, id, doc)

	run(t, exec, describedThenEdited(t, exec, out, "page MyFirstModule.ThingEdit", "Label: 'Colour'", "Label: 'Colour (changed)'"))
	if !storedHas(t, exec, id, "Colour (changed)") {
		t.Fatal("the rewrite did not land, so it shows nothing about the patch")
	}
	cb := namedDoc(t, storedUnitDoc(t, exec, id), "cbColour")
	if got := bsonnav.DGetString(widgetProp(t, cb, "customEditabilityExpression"), "PrimitiveValue"); got != "" {
		t.Errorf("changing cbColour's label rewrote customEditabilityExpression to %q", got)
	}
}

// choreColumns reads the stored grid's column object type and columns.
func choreColumns(t *testing.T, doc bson.D) (bson.D, []bson.D) {
	t.Helper()
	grid := namedDoc(t, doc, "dgChores")
	objs := bsonDocs(bsonValue(widgetProp(t, grid, "columns"), "Objects"))
	if len(objs) != 2 {
		t.Fatalf("grid stores %d columns", len(objs))
	}
	return nestedObjectType(t, objectType(grid), "columns"), objs
}

func columnAttribute(t *testing.T, ot, col bson.D) bson.D {
	t.Helper()
	return bsonDocField(propValue(t, ot, col, "attribute"), "AttributeRef")
}

// seedStudioProGrid gives dgChores what Studio Pro stores and describe cannot
// say: the Assignee column bound to the specialization's attribute through the
// association (describe prints it as `Chore_Assignee/FullName`, which resolves
// against System.User), and a row click MDL has no spelling for.
func seedStudioProGrid(t *testing.T, exec *Executor, id string) {
	t.Helper()
	doc := storedUnitDoc(t, exec, id)
	ot, cols := choreColumns(t, doc)
	ref := columnAttribute(t, ot, cols[1])
	bsonnav.DSet(ref, "Attribute", "Administration.Account.FullName")
	steps := bsonDocs(bsonValue(bsonDocField(ref, "EntityRef"), "Steps"))
	if len(steps) != 1 {
		t.Fatalf("the Assignee column's attribute has %d steps", len(steps))
	}
	bsonnav.DSet(steps[0], "DestinationEntity", "Administration.Account")
	bsonnav.DSet(widgetProp(t, namedDoc(t, doc, "dgChores"), "onClick"), "Action", openUserTask())
	writeUnitDoc(t, exec, id, doc)
}

// checkStudioProGrid reports what of the seed a rewrite lost.
func checkStudioProGrid(t *testing.T, exec *Executor, id, what string) {
	t.Helper()
	doc := storedUnitDoc(t, exec, id)
	ot, cols := choreColumns(t, doc)
	if got := bsonnav.DGetString(columnAttribute(t, ot, cols[1]), "Attribute"); got != "Administration.Account.FullName" {
		t.Errorf("%s rebound the Assignee column to %s", what, got)
	}
	if got := bsonnav.DGetString(bsonDocField(widgetProp(t, namedDoc(t, doc, "dgChores"), "onClick"), "Action"), "$Type"); got != "Forms$OpenUserTaskClientAction" {
		t.Errorf("%s reset the grid's row click to %s", what, got)
	}
}

func TestIssue832_EditedDataGridKeepsWhatItDidNotChange(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832GridSetup)
	id := unitIDByName(t, exec, "page", "Chores")
	seedStudioProGrid(t, exec, id)

	run(t, exec, describedThenEdited(t, exec, out, "page MyFirstModule.Chores", "Caption: 'Title'", "Caption: 'Chore'"))
	if !storedHas(t, exec, id, "Chore") {
		t.Fatal("the rewrite did not land, so it shows nothing about the patch")
	}
	checkStudioProGrid(t, exec, id, "changing the Title column's caption")
}

// Re-running describe's output unchanged keeps the grid as stored, even though
// describe prints the specialized attribute as the generalization's.
func TestIssue832_UnchangedDataGridIsNotRebuilt(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832GridSetup)
	id := unitIDByName(t, exec, "page", "Chores")
	seedStudioProGrid(t, exec, id)

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Chores")
	described := out.String()
	out.Reset()
	run(t, exec, described)
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-running describe unchanged rewrote the page:\n%s", out.String())
	}
	checkStudioProGrid(t, exec, id, "re-running describe unchanged")
}

// A data view switched to another entity binds the same attribute name to
// another attribute: the stored binding must not be kept (the reason the
// unchanged-widget passthrough compares model references).
func TestIssue832_ContextChangeRebindsTheAttribute(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue832Setup)
	run(t, exec, `create persistent entity MyFirstModule.Other (Colour: Enumeration(MyFirstModule.Colour));`)
	run(t, exec, issue832ComboPage)
	id := unitIDByName(t, exec, "page", "ThingEdit")

	toOther := strings.NewReplacer("$Thing: MyFirstModule.Thing", "$Other: MyFirstModule.Other", "DataSource: $Thing", "DataSource: $Other")
	for _, tc := range []struct{ stmt, entity string }{
		{toOther.Replace(issue832ComboPage), "MyFirstModule.Other"},                                                          // the same combo box text
		{strings.Replace(issue832ComboPage, "Label: 'Colour'", "Label: 'Colour (changed)'", 1), "MyFirstModule.Thing"},       // back, and changed
		{toOther.Replace(strings.Replace(issue832ComboPage, "Label: 'Colour'", "Label: 'Other'", 1)), "MyFirstModule.Other"}, // over, and changed
	} {
		run(t, exec, tc.stmt)
		cb := namedDoc(t, storedUnitDoc(t, exec, id), "cbColour")
		got := bsonnav.DGetString(bsonDocField(widgetProp(t, cb, "attributeEnumeration"), "AttributeRef"), "Attribute")
		if got != tc.entity+".Colour" {
			t.Errorf("bound to %s, want %s.Colour", got, tc.entity)
		}
	}
}

func TestIssue832_LayoutRewriteKeepsItsPluggableWidgets(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832Layout)
	ls, err := exec.backend.ListLayouts()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, l := range ls {
		if l.Name == "MyLayout" {
			id = string(l.ID)
		}
	}
	if id == "" {
		t.Fatal("no layout MyLayout")
	}
	click := func() string {
		img := namedDoc(t, storedUnitDoc(t, exec, id), "imgLogo")
		return bsonnav.DGetString(bsonDocField(widgetProp(t, img, "onClick"), "Action"), "$Type")
	}
	doc := storedUnitDoc(t, exec, id)
	bsonnav.DSet(widgetProp(t, namedDoc(t, doc, "imgLogo"), "onClick"), "Action", openUserTask())
	writeUnitDoc(t, exec, id, doc)

	// Unchanged: the stored widget is kept, so nothing is written.
	out.Reset()
	run(t, exec, issue832Layout)
	if !strings.Contains(out.String(), "Unchanged layout") {
		t.Errorf("re-running the layout unchanged rewrote it:\n%s", out.String())
	}
	if got := click(); got != "Forms$OpenUserTaskClientAction" {
		t.Errorf("re-running the layout unchanged reset imgLogo's click to %s", got)
	}

	// Changed: the property it names is written, the one it does not is kept.
	run(t, exec, strings.Replace(issue832Layout, "logo.png", "logo2.png", 1))
	if !storedHas(t, exec, id, "logo2.png") {
		t.Fatal("the rewrite did not land, so it shows nothing about the patch")
	}
	if got := click(); got != "Forms$OpenUserTaskClientAction" {
		t.Errorf("changing imgLogo's url reset its click to %s", got)
	}
}

// A column added: the list is the statement's, the rest of the grid as stored.
func TestIssue832_AddedColumnKeepsTheGridsOtherProperties(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832GridSetup)
	id := unitIDByName(t, exec, "page", "Chores")
	doc := storedUnitDoc(t, exec, id)
	bsonnav.DSet(widgetProp(t, namedDoc(t, doc, "dgChores"), "onClick"), "Action", openUserTask())
	writeUnitDoc(t, exec, id, doc)

	run(t, exec, describedThenEdited(t, exec, out, "page MyFirstModule.Chores",
		"column (Attribute: Title, Caption: 'Title')", "column (Attribute: Title, Caption: 'Title')\n    column (Attribute: Title, Caption: 'Again')"))
	grid := namedDoc(t, storedUnitDoc(t, exec, id), "dgChores")
	if n := len(bsonDocs(bsonValue(widgetProp(t, grid, "columns"), "Objects"))); n != 3 {
		t.Fatalf("the grid stores %d columns, want the 3 the statement writes", n)
	}
	if got := bsonnav.DGetString(bsonDocField(widgetProp(t, grid, "onClick"), "Action"), "$Type"); got != "Forms$OpenUserTaskClientAction" {
		t.Errorf("adding a column reset the grid's row click to %s", got)
	}
}

// propType and propOf build a one-level widget type and object for the merge.
func propType(id, key, typ string, nested ...bson.D) bson.D {
	vt := bson.D{{Key: "$ID", Value: hcBin("vt-" + id)}, {Key: "Type", Value: typ}}
	if len(nested) > 0 {
		vt = append(vt, bson.E{Key: "ObjectType", Value: nested[0]})
	}
	return bson.D{{Key: "$ID", Value: hcBin(id)}, {Key: "PropertyKey", Value: key}, {Key: "ValueType", Value: vt}}
}

func objType(id string, pts ...bson.D) bson.D {
	a := bson.A{int32(2)}
	for _, p := range pts {
		a = append(a, p)
	}
	return bson.D{{Key: "$ID", Value: hcBin(id)}, {Key: "PropertyTypes", Value: a}}
}

// propOf is a property of the type typeID; its value points at that type's
// value type, as a built value does.
func propOf(typeID string, value bson.D) bson.D {
	v := append(bson.D{{Key: "TypePointer", Value: hcBin("vt-" + typeID)}}, value...)
	return bson.D{{Key: "TypePointer", Value: hcBin(typeID)}, {Key: "Value", Value: v}}
}

func objOf(props ...bson.D) bson.D {
	a := bson.A{int32(2)}
	for _, p := range props {
		a = append(a, p)
	}
	return bson.D{{Key: "Properties", Value: a}}
}

func prim(v string) bson.D {
	return bson.D{{Key: "PrimitiveValue", Value: v}}
}

func TestPluggableMergeAcrossSchemaVersions(t *testing.T) {
	// The stored schema has a property the template lacks, and the template
	// one the stored schema lacks.
	sOT := objType("s", propType("s-caption", "caption", "String"), propType("s-old", "legacy", "String"))
	tOT := func(prefix string) bson.D {
		return objType(prefix, propType(prefix+"-caption", "caption", "String"), propType(prefix+"-new", "added", "String"))
	}
	stored := objOf(propOf("s-caption", prim("A")), propOf("s-old", prim("kept")))
	build := func(prefix, caption, added string) bson.D {
		return objOf(propOf(prefix+"-caption", prim(caption)), propOf(prefix+"-new", prim(added)))
	}
	merge := func(a, b bson.D) (bson.D, bool) {
		m := &pluggableMerge{ids: map[string]any{}}
		m.mapTypes(sOT, tOT("a"))
		return m.object(sOT, tOT("a"), tOT("b"), stored, a, b, nil)
	}

	// The statement changes the caption: written; the stored-only property
	// kept; the template-only one, unchanged, not added.
	got, ok := merge(build("a", "B", "x"), build("b", "A", "x"))
	if !ok {
		t.Fatal("the merge was abandoned")
	}
	props := propertiesByKey(sOT, got)
	if v := bsonString(bsonDocField(props["caption"], "Value"), "PrimitiveValue"); v != "B" {
		t.Errorf("caption = %q, want the statement's B", v)
	}
	if p := bsonIDKey(bsonValue(bsonDocField(props["caption"], "Value"), "TypePointer")); p != "vt-s-caption" {
		t.Errorf("the written caption points at %q, not at the stored schema's value type", p)
	}
	if v := bsonString(bsonDocField(props["legacy"], "Value"), "PrimitiveValue"); v != "kept" {
		t.Errorf("the stored-only property is %q, want kept", v)
	}
	if len(bsonDocs(bsonValue(got, "Properties"))) != 2 {
		t.Errorf("the merge wrote a property the stored schema does not have")
	}

	// The statement changes a property the stored schema has no slot for.
	if _, ok := merge(build("a", "A", "y"), build("b", "A", "x")); ok {
		t.Error("a change the stored schema cannot hold was merged instead of abandoning the patch")
	}
}
