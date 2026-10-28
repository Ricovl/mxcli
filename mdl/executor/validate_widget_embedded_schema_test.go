// SPDX-License-Identifier: Apache-2.0

// `mxcli check` without a project must not call a Data grid 2 setting "not
// recognized and silently dropped" when the widget declares it.
//
//	datagrid dgAttr (DataSource: database Shop.Ord, configurationStorageType: attribute,
//	                 configurationAttribute: Config)
//
// `exec` stores both, and `check -p` accepts them. `check` alone warned
// MDL-WIDGET07 for each — and refused an Expression-typed one (rowClass) as
// MDL-WIDGET42, an ERROR — because DATAGRID has no embedded definition and the
// check fell back on the vocabulary of the built-in widgets.
//
// The keys here come from the Data grid 2 template mxcli embeds, not from a
// list typed for the test.
package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// noProjectFindings is what `mxcli check script.mdl` (no -p) reports for the
// widget rules in question.
func noProjectFindings(t *testing.T, script string) []linter.Violation {
	t.Helper()
	var out []linter.Violation
	for _, v := range ValidateProgram(parseMDL(t, script), "") {
		if v.RuleID == "MDL-WIDGET07" || v.RuleID == "MDL-WIDGET42" || v.RuleID == nativeWidgetPropDropped.Code {
			out = append(out, v)
		}
	}
	return out
}

func describeFindings(vs []linter.Violation) string {
	var lines []string
	for _, v := range vs {
		lines = append(lines, fmt.Sprintf("[%s %s] %s", v.RuleID, v.Severity, v.Message))
	}
	return strings.Join(lines, "\n  ")
}

// The personalization settings the issue is about, in the three shapes a script
// uses them (exec stores every one).
func TestCheckWithoutProject_DataGridDeclaredKeysAreNotReportedAsDropped(t *testing.T) {
	script := `create page Shop.Ord_Overview ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dg (DataSource: database Shop.Ord, PageSize: 25, Pagination: virtualScrolling,
               itemSelection: Single, refreshInterval: 30, columnsHidable: false, showEmptyPlaceholder: custom) {
    column (Attribute: Code, Caption: 'Code', Sortable: true) { textfilter tf }
  }
};
create page Shop.Ord_Local ( Title: 'Local', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgLocal (DataSource: database Shop.Ord, configurationStorageType: localStorage, storeFiltersInPersonalization: true) {
    column (Attribute: Code) { textfilter tf }
  }
};
create page Shop.Ord_Attr ( Title: 'Attr', Layout: Atlas_Core.Atlas_Default, Params: ( $Pref: Shop.GridPref ) ) {
  dataview dvPref (DataSource: $Pref) {
    datagrid dgAttr (DataSource: database Shop.Ord, configurationStorageType: attribute,
                     configurationAttribute: Config, storeFiltersInPersonalization: true) {
      column (Attribute: Code) { textfilter tf2 }
    }
  }
};`
	if got := noProjectFindings(t, script); len(got) != 0 {
		t.Errorf("check without a project reported declared Data grid 2 keys:\n  %s", describeFindings(got))
	}
}

// Expression-typed keys, grid and column: an error before, because the check
// could not tell the key takes one.
func TestCheckWithoutProject_DataGridExpressionKeysTakeAnExpression(t *testing.T) {
	script := `create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dg (DataSource: database Shop.Ord, rowClass: if $currentObject/Open then 'open' else 'closed') {
    column (Attribute: Code, columnClass: if $currentObject/Open then 'a' else 'b') { textfilter tf }
  }
};`
	if got := noProjectFindings(t, script); len(got) != 0 {
		t.Errorf("check without a project refused an expression on an Expression-typed key:\n  %s", describeFindings(got))
	}
}

// The column settings that have no MDL keyword.
func TestCheckWithoutProject_DataGridColumnDeclaredKeysAreNotReportedAsDropped(t *testing.T) {
	script := `create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dg (DataSource: database Shop.Ord) {
    column (Attribute: Code, ShowContentAs: customContent, ColumnWidth: autoFit, minWidth: manual,
            minWidthLimit: 150, allowEventPropagation: false, exportValue: 'x') {
      dynamictext t (Content: 'x')
    }
  }
};`
	if got := noProjectFindings(t, script); len(got) != 0 {
		t.Errorf("check without a project reported declared column keys:\n  %s", describeFindings(got))
	}
}

// A misspelled key is still reported — the point of the rule — and says what is
// true: it cannot be verified without a project, and it names the key it
// probably meant.
func TestCheckWithoutProject_DataGridMisspelledKeyStillWarns(t *testing.T) {
	script := `create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dg (DataSource: database Shop.Ord, configurationStorageTyp: localStorage) {
    column (Attribute: Code, minWidthLimt: 150) { textfilter tf }
  }
};`
	got := noProjectFindings(t, script)
	if len(got) != 2 {
		t.Fatalf("want 2 findings (the grid key and the column key), got %d:\n  %s", len(got), describeFindings(got))
	}
	for _, v := range got {
		if v.RuleID != "MDL-WIDGET07" || v.Severity != linter.SeverityWarning {
			t.Errorf("want a MDL-WIDGET07 warning, got %s", describeFindings([]linter.Violation{v}))
		}
	}
	grid, column := got[0].Message, got[1].Message
	if !strings.Contains(grid, "`configurationStorageTyp`") || !strings.Contains(grid, "did you mean `configurationStorageType`") {
		t.Errorf("grid finding does not name the key and its likely spelling:\n  %s", grid)
	}
	if !strings.Contains(column, "`minWidthLimt`") || !strings.Contains(column, "did you mean `minWidthLimit`") {
		t.Errorf("column finding does not name the key and its likely spelling:\n  %s", column)
	}
	for _, msg := range []string{grid, column} {
		if !strings.Contains(msg, "without a project") {
			t.Errorf("finding does not say it cannot be verified without a project:\n  %s", msg)
		}
	}
}

// The embedded definition is a baseline, not the installed widget: a key it does
// not declare may be one a newer widget version added. The message must not
// claim the value is lost as a fact; it says the key cannot be verified.
func TestCheckWithoutProject_UndeclaredKeyIsNotClaimedToBeDropped(t *testing.T) {
	got := noProjectFindings(t, `create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dg (DataSource: database Shop.Ord, aSettingFromANewerWidget: true)
};`)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d:\n  %s", len(got), describeFindings(got))
	}
	msg := got[0].Message
	if !strings.Contains(msg, "cannot be verified without a project") {
		t.Errorf("message does not say the key cannot be verified:\n  %s", msg)
	}
	if strings.Contains(msg, "is not recognized and will be silently dropped") {
		t.Errorf("message states as fact that the value is dropped:\n  %s", msg)
	}
}

// A built-in widget has no embedded template: it is judged by the properties
// its own builder writes (ako/mxcli#1698), not by a data grid's schema.
func TestCheckWithoutProject_BuiltInWidgetUnknownKeyIsUnchanged(t *testing.T) {
	got := noProjectFindings(t, `create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {
  textbox tb (Attribute: Code, columnsHidable: false)
};`)
	if len(got) != 1 || got[0].RuleID != nativeWidgetPropDropped.Code ||
		!strings.Contains(got[0].Message, "`columnsHidable` is not a property mxcli writes on a textbox") {
		t.Errorf("want the text box's own verdict (%s), got:\n  %s", nativeWidgetPropDropped.Code, describeFindings(got))
	}
}

// Derived from the schema, not typed: every scalar key the embedded Data grid 2
// template declares, written as the value its kind takes, draws neither rule.
// A key added to the template is covered the day it is added.
func TestCheckWithoutProject_EveryDeclaredDataGridKeyIsAccepted(t *testing.T) {
	schema := loadEmbeddedSchema(embeddedDataGridID, "")
	if schema == nil || len(schema.Names) < 30 {
		t.Fatalf("Data grid 2 schema = %+v, want the template's ~40 keys", schema)
	}
	values := map[string]string{
		"Boolean": "true", "Integer": "1", "Decimal": "1", "String": "'x'", "Enumeration": "'x'",
		"TextTemplate": "'x'", "Expression": "if true then 'a' else 'b'", "Attribute": "Code",
	}
	var props []string
	for _, key := range schema.Names {
		kind := schema.types[strings.ToLower(key)]
		v, ok := values[kind]
		if !ok {
			continue // datasource, selection, action, widgets, object: other rules and other syntax
		}
		props = append(props, key+": "+v)
	}
	if len(props) < 25 {
		t.Fatalf("only %d scalar keys in the template", len(props))
	}
	script := "create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {\n  datagrid dg (DataSource: database Shop.Ord, " +
		strings.Join(props, ", ") + ")\n};"
	if got := noProjectFindings(t, script); len(got) != 0 {
		t.Errorf("a key the template declares was reported:\n  %s", describeFindings(got))
	}
}

// The column item type, the same way.
func TestCheckWithoutProject_EveryDeclaredDataGridColumnKeyIsAccepted(t *testing.T) {
	schema := loadEmbeddedSchema(embeddedDataGridID, "columns")
	if schema == nil || len(schema.Names) < 15 {
		t.Fatalf("column schema = %+v, want the template's ~20 keys", schema)
	}
	values := map[string]string{
		"Boolean": "true", "Integer": "1", "Decimal": "1", "String": "'x'", "Enumeration": "'x'",
		"TextTemplate": "'x'", "Expression": "if true then 'a' else 'b'", "Attribute": "Code",
	}
	var props []string
	for _, key := range schema.Names {
		if v, ok := values[schema.types[strings.ToLower(key)]]; ok {
			props = append(props, key+": "+v)
		}
	}
	script := "create page Shop.P ( Title: 'P', Layout: Atlas_Core.Atlas_Default ) {\n  datagrid dg (DataSource: database Shop.Ord) {\n    column (" +
		strings.Join(props, ", ") + ")\n  }\n};"
	if got := noProjectFindings(t, script); len(got) != 0 {
		t.Errorf("a column key the template declares was reported:\n  %s", describeFindings(got))
	}
}

const embeddedDataGridID = "com.mendix.widget.web.datagrid.Datagrid"
