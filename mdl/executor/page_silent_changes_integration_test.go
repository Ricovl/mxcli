// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1214: `alter page … set Editable = true on <listview>`
// printed "Altered page" and stored nothing. describe is the read the user has.
func TestAlterPage_SetListViewEditable(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entity := testModule + ".EditRow"
	page := testModule + ".EditRowsPage"
	if err := env.executeMDL(`create or modify persistent entity ` + entity + ` (Note: String(200));`); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(`create page ` + page + ` (Title: 'Rows', Layout: Atlas_Core.Atlas_Default) {
		listview lvRows (DataSource: database ` + entity + `) { textbox txtNote (Attribute: Note) }
	}`); err != nil {
		t.Fatal(err)
	}
	before, err := env.describeMDL(`describe page ` + page + `;`)
	if err != nil {
		t.Fatal(err)
	}
	// Control: the list view starts read-only, so the assertion below can fail.
	if strings.Contains(before, "Editable: true") {
		t.Fatalf("control: list view already editable before the alter:\n%s", before)
	}
	if err := env.executeMDL(`alter page ` + page + ` { set (Editable: true) on lvRows };`); err != nil {
		t.Fatal(err)
	}
	after, err := env.describeMDL(`describe page ` + page + `;`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, "listview lvRows (DataSource: database from "+entity+", Editable: true)") {
		t.Errorf("set Editable = true on a list view was not stored:\n%s", after)
	}
	// The input enum is not a list view's value: refused, not swallowed.
	if err := env.executeMDL(`alter page ` + page + ` { set (Editable: true) on txtNote };`); err == nil {
		t.Error("set Editable = true on a text box succeeded; its Editable is Always/Never")
	}
}

// ako/mxcli#528, mendixlabs/mxcli#293: a data view's footer is a region with no
// stored name. It is addressed as `<dataview>.footer`, a replacement may restate
// the names of the widgets it removes, and describe prints the footer unnamed —
// so its output re-executes onto the same page.
func TestAlterPage_DataViewFooterRegion(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0)

	entity := testModule + ".FootRow"
	page := testModule + ".FootRowPage"
	if err := env.executeMDL(`create or modify persistent entity ` + entity + ` (Note: String(200));`); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(`create page ` + page + ` (Title: 'Foot', Layout: Atlas_Core.Atlas_Default, Params: { $Row: ` + entity + ` }) {
		dataview dvMain (DataSource: $Row) {
			textbox txtNote (Attribute: Note)
			footer footerButtons { actionbutton btnSave (Caption: 'Save', Action: save_changes) }
		}
	}`); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(`alter page ` + page + ` {
		insert into dvMain.footer { actionbutton btnCancel (Caption: 'Cancel', Action: cancel_changes) }
	};`); err != nil {
		t.Fatalf("insert into dvMain.footer: %v", err)
	}
	if err := env.executeMDL(`alter page ` + page + ` {
		replace dvMain.footer with { footer {
			actionbutton btnSave (Caption: 'Save it', Action: save_changes)
			actionbutton btnClose (Caption: 'Close', Action: close_page)
		} }
	};`); err != nil {
		t.Fatalf("replace dvMain.footer restating btnSave: %v", err)
	}
	out, err := env.describeMDL(`describe page ` + page + `;`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "footer {") || strings.Contains(out, "footer1") || strings.Contains(out, "footerButtons") {
		t.Errorf("describe should print the footer unnamed:\n%s", out)
	}
	if !strings.Contains(out, "'Save it'") || !strings.Contains(out, "btnClose") || strings.Contains(out, "btnCancel") {
		t.Errorf("the footer was not replaced as a whole:\n%s", out)
	}
	// The name a script wrote on the footer, and the old `footer1`, name
	// nothing; the miss points at the address that works.
	err = env.executeMDL(`alter page ` + page + ` { drop footer1 };`)
	if err == nil || !strings.Contains(err.Error(), "dvMain.footer") {
		t.Errorf("drop footer1: %v, want a not-found naming dvMain.footer", err)
	}
	// describe's output re-executes onto the page it came from.
	if err := env.executeMDL(stripDescribeArtifacts(out)); err != nil {
		t.Fatalf("re-executing describe output: %v\n%s", err, out)
	}
	again, err := env.describeMDL(`describe page ` + page + `;`)
	if err != nil {
		t.Fatal(err)
	}
	if again != out {
		t.Errorf("describe changed after re-executing its own output:\n--- before\n%s\n--- after\n%s", out, again)
	}
}
