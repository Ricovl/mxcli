// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"testing"
)

// The widget tree, appearance and action columns (mendixlabs/mxcli#1268) on
// Studio Pro-authored pages: ako/TestApp read through the real model reader.
// Each expected value was read off the project.

// Every parent is a row of the same page or snippet, every child sits one level
// below its parent, and only a root has depth 0 — over all of TestApp's
// widgets, so a holder the walk forgot to make transparent would surface as a
// dangling parent somewhere.
func TestTestAppWidgetTreeIsConsistent(t *testing.T) {
	cat := testAppActivities(t)
	for q, what := range map[string]string{
		`SELECT count(*) FROM widgets_data c WHERE c.ParentWidgetId != '' AND NOT EXISTS
			(SELECT 1 FROM widgets_data p WHERE p.Id = c.ParentWidgetId AND p.ContainerId = c.ContainerId)`: "dangling parents",
		`SELECT count(*) FROM widgets_data c JOIN widgets_data p ON p.Id = c.ParentWidgetId
			WHERE c.Depth != p.Depth + 1`: "children not one below their parent",
		`SELECT count(*) FROM widgets_data WHERE (ParentWidgetId = '') != (Depth = 0)`: "roots not at depth 0",
	} {
		if n := fmt.Sprint(one(t, cat, q)[0]); n != "0" {
			t.Errorf("%s: %s", what, n)
		}
	}
	// CONTROL: the invariants above hold vacuously on an empty or flat table.
	if n := fmt.Sprint(one(t, cat, `SELECT count(*) FROM widgets_data WHERE Depth > 0`)[0]); n == "0" {
		t.Fatal("no nested widgets in TestApp — the tree was not built")
	}
}

// A button nine levels down, through a layout grid column, two data views, a
// tab container and a data grid 2 column: the column, the row and the tab page
// are not widgets, so the chain names only catalogued widgets.
func TestTestAppWidgetAncestorChain(t *testing.T) {
	cat := testAppActivities(t)
	res, err := cat.Query(`WITH RECURSIVE chain(id, name, depth, pid) AS (
			SELECT Id, Name, Depth, ParentWidgetId FROM widgets_data
			 WHERE ContainerQualifiedName = 'WorkflowCommons.ManageTaskAssignments' AND Name = 'actionButton5'
			UNION ALL
			SELECT w.Id, w.Name, w.Depth, w.ParentWidgetId FROM widgets_data w JOIN chain c ON w.Id = c.pid)
		SELECT name, depth FROM chain ORDER BY depth`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"layoutGrid1", "dataView1", "container1", "dataView2", "container13",
		"tabContainer1", "dataGrid23", "container5", "actionButton5"}
	var got []string
	for i, row := range res.Rows {
		got = append(got, fmt.Sprint(row[0]))
		if d := fmt.Sprint(row[1]); d != fmt.Sprint(i) {
			t.Errorf("%v at depth %s, want %d", row[0], d, i)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ancestor chain = %v\nwant             %v", got, want)
	}
}

func TestTestAppWidgetAppearanceAndAction(t *testing.T) {
	cat := testAppActivities(t)
	row := one(t, cat, `SELECT Class, Style FROM widgets_data
		WHERE ContainerQualifiedName = 'Administration.Account_Edit' AND Name = 'label4'`)
	if row[0] != "alert alert-warning" || row[1] != "width:100%;" {
		t.Errorf("Account_Edit.label4 class/style = %q / %q", row[0], row[1])
	}
	// Mendix's own Account_Overview deletes directly — the case the
	// delete-button rule flags — and a delete action has no confirmation.
	row = one(t, cat, `SELECT ActionType, HasConfirmation FROM widgets_data
		WHERE ContainerQualifiedName = 'Administration.Account_Overview' AND Name = 'actionButton4'`)
	if row[0] != "Forms$DeleteClientAction" || fmt.Sprint(row[1]) != "0" {
		t.Errorf("Account_Overview.actionButton4 = %v / %v", row[0], row[1])
	}
	// A microflow call whose confirmation is set in Studio Pro.
	row = one(t, cat, `SELECT ActionType, HasConfirmation FROM widgets_data
		WHERE ContainerQualifiedName = 'WorkflowCommons.ManageTaskAssignments' AND Name = 'actionButton4'`)
	if row[0] != "Forms$MicroflowAction" || fmt.Sprint(row[1]) != "1" {
		t.Errorf("ManageTaskAssignments.actionButton4 = %v / %v", row[0], row[1])
	}
}
