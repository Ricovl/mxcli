// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1647: TestApp's comboBox1 (database source, multi-selection)
// described as
//
//	combobox comboBox1 (DataSource: database from System.WorkflowGroup …, …)
//
// with a note that optionsSourceDatabaseCaptionAttribute was not reproduced and
// nothing about the item selection. `DataSource:` is the association mode's
// clause, so exec built an association-mode combo box with no association.

// databaseComboBox is the shipped template set up the way Studio Pro stores
// comboBox1.
func databaseComboBox(t *testing.T) map[string]any {
	w := widgetFromTemplate(t, comboBoxWidgetID, "comboBox1")
	gridValue(t, w, "source")["PrimitiveValue"] = "database"
	// Studio Pro leaves the association mode's type behind when the source is
	// switched to database; it means nothing then.
	gridValue(t, w, "optionsSourceType")["PrimitiveValue"] = "association"
	gridValue(t, w, "optionsSourceDatabaseDataSource")["DataSource"] = map[string]any{
		"$Type":           "CustomWidgets$CustomWidgetXPathSource",
		"EntityRef":       map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "System.WorkflowGroup"},
		"XPathConstraint": "[Name != empty]",
	}
	gridValue(t, w, "optionsSourceDatabaseItemSelection")["Selection"] = "Multi"
	gridValue(t, w, "optionsSourceDatabaseCaptionAttribute")["AttributeRef"] = map[string]any{
		"$Type": "DomainModels$AttributeRef", "Attribute": "System.WorkflowGroup.Name",
	}
	return w
}

func describeComboBoxText(t *testing.T, w map[string]any) string {
	t.Helper()
	got := describeGrid(t, w, "System.User")
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got, 0)
	return buf.String()
}

func TestDescribeDatabaseComboBoxSpellsItsMode(t *testing.T) {
	out := describeComboBoxText(t, databaseComboBox(t))
	for _, want := range []string{
		"optionsSourceDatabaseDataSource: database from System.WorkflowGroup where [Name != empty]",
		"optionsSourceDatabaseItemSelection: 'Multi'",
		"optionsSourceDatabaseCaptionAttribute: Name",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("description lacks %q:\n%s", want, out)
		}
	}
	// Not the association mode's clause, and nothing claimed as lost.
	for _, bad := range []string{"DataSource: database", "NOT re-executable"} {
		if strings.Contains(strings.ReplaceAll(out, "optionsSourceDatabaseDataSource: database", ""), bad) {
			t.Errorf("description has %q:\n%s", bad, out)
		}
	}
	if _, errs := visitor.Build("create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + out + "};"); len(errs) > 0 {
		t.Errorf("description does not parse: %v\n%s", errs, out)
	}
}

// The default selection is not printed, as no default is.
func TestDescribeDatabaseComboBoxOmitsTheDefaultSelection(t *testing.T) {
	w := databaseComboBox(t)
	gridValue(t, w, "optionsSourceDatabaseItemSelection")["Selection"] = "Single"
	if out := describeComboBoxText(t, w); strings.Contains(out, "optionsSourceDatabaseItemSelection") {
		t.Errorf("default selection printed:\n%s", out)
	}
}

// Control: a static-source combo box's settings stay withheld — that mode is
// still not spelled, and printing its keys into another mode would bind them
// against the wrong entity.
func TestDescribeStaticComboBoxStillWithholdsItsSettings(t *testing.T) {
	w := widgetFromTemplate(t, comboBoxWidgetID, "cbStatic")
	gridValue(t, w, "source")["PrimitiveValue"] = "static"
	gridValue(t, w, "optionsSourceDatabaseCaptionAttribute")["AttributeRef"] = map[string]any{
		"$Type": "DomainModels$AttributeRef", "Attribute": "System.WorkflowGroup.Name",
	}
	out := describeComboBoxText(t, w)
	if strings.Contains(out, "optionsSourceDatabaseCaptionAttribute: Name") {
		t.Errorf("a database-mode key printed on a static combo box:\n%s", out)
	}
}
