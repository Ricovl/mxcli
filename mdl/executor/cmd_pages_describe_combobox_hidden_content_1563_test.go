// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1563: "describe page does not show [a combo box's custom content]",
// so the widgets in it — txtOptionName, bound to Booking.Name — were invisible
// until mx check named one. MDL cannot author custom content (MDL-WIDGET26),
// so describe cannot print it as MDL; it now says it is there, by widget name,
// which is also what the replace refusal tells the author to drop.

// comboFromTemplate is an untouched combo box: the shipped template's schema
// and default values.
func comboFromTemplate(t *testing.T) map[string]any {
	return widgetFromTemplate(t, comboBoxWidgetID, "cbBooking")
}

func TestDescribeComboBoxNamesItsHiddenCustomContent(t *testing.T) {
	w := comboFromTemplate(t)
	gridValue(t, w, "optionsSourceAssociationCustomContentType")["PrimitiveValue"] = "yes"
	gridValue(t, w, "optionsSourceAssociationCustomContent")["Widgets"] = []any{2,
		map[string]any{"$Type": "Forms$DynamicText", "Name": "txtOptionName"},
		map[string]any{"$Type": "Forms$Image"},
	}

	got := describeGrid(t, w, "")
	if len(got.HiddenContent) != 1 {
		t.Fatalf("HiddenContent = %+v, want one slot", got.HiddenContent)
	}
	hc := got.HiddenContent[0]
	if hc.Property != "optionsSourceAssociationCustomContent" || strings.Join(hc.Widgets, ",") != "txtOptionName,an unnamed image" {
		t.Errorf("HiddenContent = %+v", hc)
	}

	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got, 0)
	out := buf.String()
	for _, want := range []string{"-- NOT SHOWN:", "cbBooking", "optionsSourceAssociationCustomContent", "txtOptionName"} {
		if !strings.Contains(out, want) {
			t.Errorf("description lacks %q:\n%s", want, out)
		}
	}
	// A comment, so the description still parses.
	if _, errs := visitor.Build("create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + out + "};"); len(errs) > 0 {
		t.Errorf("description no longer parses: %v\n%s", errs, out)
	}
}

// Control: an untouched combo box describes exactly as before.
func TestDescribeComboBoxWithoutCustomContentHasNoNote(t *testing.T) {
	got := describeGrid(t, comboFromTemplate(t), "")
	if len(got.HiddenContent) != 0 {
		t.Errorf("untouched combo box reported hidden content %+v", got.HiddenContent)
	}
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got, 0)
	if strings.Contains(buf.String(), "NOT SHOWN") {
		t.Errorf("untouched combo box printed a note:\n%s", buf.String())
	}
}
