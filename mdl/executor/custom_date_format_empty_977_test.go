// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 5: an `alter set` of an empty CustomDateFormat "is not
// refused on a picker already set to Custom, because properties are applied
// one at a time. It produces CE0493" ("Date format is custom but no format
// string is specified"). The DateFormat setter refused `Custom` without a pattern, but
// clearing the pattern of a picker already Custom went through.
//
// The rule is about the widget a statement leaves behind, so it is checked
// once the whole SET is applied: clearing the pattern and leaving Custom is
// refused, while clearing it and switching to Date in the same statement is
// a valid result and is not.
package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func storedPickerFormat(t *testing.T, exec *Executor, page, widget string) (dateFormat, custom string) {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "Name") != widget {
			return
		}
		fi, _ := afGet(d, "FormattingInfo").(bson.D)
		dateFormat, _ = afGet(fi, "DateFormat").(string)
		custom, _ = afGet(fi, "CustomDateFormat").(string)
	})
	return dateFormat, custom
}

func TestAlterSetEmptyCustomDateFormatOnCustomPickerIsRefused(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	if err := afRun(t, exec, `create persistent entity MyFirstModule.Shift (Starts: DateTime);
create page MyFirstModule.Shift_Edit (Title: 'Shift', Layout: Atlas_Core.Atlas_Default, Params: ($Shift: MyFirstModule.Shift)) {
  dataview dvShift (DataSource: $Shift) {
    datepicker dpStarts (Attribute: Starts, DateFormat: Custom, CustomDateFormat: 'dd-MM-yyyy')
  }
};`); err != nil {
		t.Fatalf("exec: %v", err)
	}
	alter := func(set string) error {
		return afRun(t, exec, "alter page MyFirstModule.Shift_Edit {\n  set ("+set+") on dpStarts\n};")
	}

	for _, set := range []string{"CustomDateFormat: ''", "CustomDateFormat: '  '"} {
		err := alter(set)
		if err == nil || !strings.Contains(err.Error(), "CE0493") {
			t.Errorf("set (%s) on a Custom picker: err = %v, want a refusal naming CE0493", set, err)
		}
		if df, cdf := storedPickerFormat(t, exec, "Shift_Edit", "dpStarts"); df != "Custom" || cdf != "dd-MM-yyyy" {
			t.Fatalf("the refused statement changed the picker to %s / %q", df, cdf)
		}
	}

	// Controls: a statement whose result is valid goes through.
	if err := alter("CustomDateFormat: 'yyyy-MM-dd'"); err != nil {
		t.Errorf("a new pattern was refused: %v", err)
	}
	if err := alter("CustomDateFormat: '', DateFormat: Date"); err != nil {
		t.Errorf("clearing the pattern while leaving Custom was refused: %v", err)
	}
	if df, cdf := storedPickerFormat(t, exec, "Shift_Edit", "dpStarts"); df != "Date" || cdf != "" {
		t.Errorf("after the valid statements the picker is %s / %q, want Date / \"\"", df, cdf)
	}
}
