// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets"
)

// widgetFromTemplate is an untouched stored widget: the shipped template's
// schema and the values exec writes for a property left out.
func widgetFromTemplate(t *testing.T, widgetID, name string) map[string]any {
	t.Helper()
	tmpl, err := widgets.GetTemplate(widgetID)
	if err != nil || tmpl == nil {
		t.Fatalf("%s template: %v, %v", widgetID, tmpl, err)
	}
	raw, err := json.Marshal(map[string]any{"Type": tmpl.Type, "Object": tmpl.Object})
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	normalizeTemplateJSON(w)
	w["$Type"] = "CustomWidgets$CustomWidget"
	w["Name"] = name
	return w
}

const comboBoxWidgetID = "com.mendix.widget.web.combobox.Combobox"

// mendixlabs/mxcli#1586: a combo box's expression-typed settings are written by
// exec's explicit-property pass and were never described — the combo box has
// its own reader, which knew only the keys MDL has keywords for. A describe →
// exec cycle reset `customEditabilityExpression` to `false`.
func TestDescribeComboBox_PrintsExpressionSettingsByKey(t *testing.T) {
	w := widgetFromTemplate(t, comboBoxWidgetID, "cb")
	gridValue(t, w, "customEditability")["PrimitiveValue"] = "conditionally"
	gridValue(t, w, "customEditabilityExpression")["Expression"] = "$currentObject/Locked"
	gridValue(t, w, "ariaRequired")["Expression"] = "true"

	got := describeParsed(t, w)
	for _, want := range []string{
		"customEditability: 'conditionally'",
		"customEditabilityExpression: $currentObject/Locked",
		"ariaRequired: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe lacks %q:\n%s", want, got)
		}
	}
}

// Control: an untouched combo box prints none of them. The template stores
// `readOnlyStyle: bordered` against a schema default of `text`, and exec writes
// the template's value for a property left out — so that is the value compared
// with, or every combo box mxcli wrote would print it.
func TestDescribeComboBox_UntouchedPrintsNothingByKey(t *testing.T) {
	w := widgetFromTemplate(t, comboBoxWidgetID, "cb")
	got := describeParsed(t, w)
	if strings.Contains(got, "readOnlyStyle") || strings.Contains(got, "NOT re-executable") {
		t.Errorf("untouched combo box printed settings:\n%s", got)
	}
	// The schema default, which Studio Pro stores, differs from what exec
	// writes, so it is printed.
	gridValue(t, w, "readOnlyStyle")["PrimitiveValue"] = "text"
	if got := describeParsed(t, w); !strings.Contains(got, "readOnlyStyle: 'text'") {
		t.Errorf("a stored readOnlyStyle text was not printed:\n%s", got)
	}
}

// A combo box in a mode DESCRIBE does not spell has its settings named, not
// printed into a combo box of another mode.
func TestDescribeComboBox_UndescribedModeIsNamedNotPrinted(t *testing.T) {
	w := widgetFromTemplate(t, comboBoxWidgetID, "cb")
	gridValue(t, w, "optionsSourceDatabaseCaptionType")["PrimitiveValue"] = "expression"
	got := describeParsed(t, w)
	if strings.Contains(got, "optionsSourceDatabaseCaptionType:") {
		t.Errorf("database-mode setting printed:\n%s", got)
	}
	if !strings.Contains(got, "-- NOT re-executable: optionsSourceDatabaseCaptionType") {
		t.Errorf("database-mode setting not named:\n%s", got)
	}
}

// The data grid filters share the combo box's branch and had the same loss: a
// text filter's `defaultValue` expression and its `delay`.
func TestDescribeTextFilter_PrintsSettingsByKey(t *testing.T) {
	w := widgetFromTemplate(t, "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter", "tf")
	gridValue(t, w, "defaultValue")["Expression"] = "'abc'"
	gridValue(t, w, "delay")["PrimitiveValue"] = "300"
	got := describeParsed(t, w)
	for _, want := range []string{"defaultValue: '''abc'''", "delay: 300"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe lacks %q:\n%s", want, got)
		}
	}
}
