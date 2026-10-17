// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1520 (ako/mxcli#1402): DESCRIBE PAGE dropped a Data grid 2
// column that has no attribute and an empty header — the usual actions column,
// `column (Caption: ”, ShowContentAs: customContent) { actionbutton … }` —
// together with every widget inside it. The column reader kept a column only
// when it had an attribute or a caption.
//
// The round trip is the damage: re-executing the unchanged output is reported
// "Unchanged page" (the stored pluggable passes through), but the first real
// edit to the described text replaces the page, and the column and its buttons
// are gone from the BSON — check, exec and mx check clean at both ends.
package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// buildDataGridWithUntitledActionsColumn is a DataGrid2 with two columns: an
// attribute column, and an actions column whose header is an empty template,
// with no attribute and one button as custom content.
func buildDataGridWithUntitledActionsColumn() map[string]any {
	const (
		idColumns       = "type-id-columns"
		idHeader        = "type-id-header"
		idAttribute     = "type-id-attribute"
		idShowContentAs = "type-id-showcontentas"
		idContent       = "type-id-content"
	)
	colProp := func(typePointer string, value map[string]any) map[string]any {
		return map[string]any{"TypePointer": typePointer, "Value": value}
	}
	header := func(text string) map[string]any {
		items := []any{}
		if text != "" {
			items = append(items, map[string]any{"Text": text})
		}
		return map[string]any{"TextTemplate": map[string]any{"Template": map[string]any{"Items": items}}}
	}
	return map[string]any{
		"Name": "dgTest",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.datagrid.Datagrid",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{
						"$ID": idColumns, "PropertyKey": "columns",
						"ValueType": map[string]any{
							"ObjectType": map[string]any{
								"PropertyTypes": []any{
									map[string]any{"$ID": idHeader, "PropertyKey": "header"},
									map[string]any{"$ID": idAttribute, "PropertyKey": "attribute"},
									map[string]any{"$ID": idShowContentAs, "PropertyKey": "showContentAs"},
									map[string]any{"$ID": idContent, "PropertyKey": "content"},
								},
							},
						},
					},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"TypePointer": idColumns,
					"Value": map[string]any{
						"Objects": []any{
							map[string]any{"Properties": []any{
								colProp(idAttribute, map[string]any{"AttributeRef": map[string]any{"Attribute": "MyModule.Item.Name"}}),
								colProp(idHeader, header("Name")),
								colProp(idShowContentAs, map[string]any{"PrimitiveValue": "attribute"}),
							}},
							map[string]any{"Properties": []any{
								colProp(idContent, map[string]any{
									"Widgets": []any{
										map[string]any{"$Type": "Forms$CheckBox", "Name": "cbSelect"},
									},
								}),
								colProp(idHeader, header("")),
								colProp(idShowContentAs, map[string]any{"PrimitiveValue": "customContent"}),
							}},
						},
					},
				},
			},
		},
	}
}

// The read half: the untitled, attribute-less column is a column.
func TestDataGrid2Column_KeepsUntitledCustomContentColumn(t *testing.T) {
	cols := extractDataGrid2Columns(nil, buildDataGridWithUntitledActionsColumn())
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d: %+v — the actions column with an empty header "+
			"and no attribute was dropped (mendixlabs/mxcli#1520)", len(cols), cols)
	}
	col := cols[1]
	if col.ShowContentAs != "customContent" {
		t.Errorf("ShowContentAs = %q, want customContent", col.ShowContentAs)
	}
	if len(col.ContentWidgets) != 1 || col.ContentWidgets[0].Name != "cbSelect" {
		t.Errorf("content widgets = %+v, want one widget named cbSelect", col.ContentWidgets)
	}
}

// The emit half: the column is printed with an explicit empty caption and its
// content, so describe -> exec writes the same column back.
func TestDataGrid2Column_EmitsUntitledCustomContentColumn(t *testing.T) {
	cols := extractDataGrid2Columns(nil, buildDataGridWithUntitledActionsColumn())
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type:            "CustomWidgets$CustomWidget",
		RenderMode:      "datagrid2",
		Name:            "dgTest",
		WidgetID:        "com.mendix.widget.web.datagrid.Datagrid",
		DataGridColumns: cols,
	}, 0)
	out := buf.String()
	if !strings.Contains(out, "column (Caption: '', ShowContentAs: customContent) {") {
		t.Errorf("untitled actions column missing from DESCRIBE output:\n%s", out)
	}
	if !strings.Contains(out, "cbSelect") {
		t.Errorf("the actions column's widget is missing from DESCRIBE output:\n%s", out)
	}
}

// End to end on the Studio Pro-authored PedApp fixture: create the page,
// describe it, make one real edit to the described text (a caption of ANOTHER
// column), execute it — the actions column and its button must still be in the
// stored BSON.
func TestDescribePage_UntitledActionsColumnRoundTrips(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	src := `create page MyFirstModule.ActionsCol (Title: 'Actions', Layout: Atlas_Core.Atlas_Default) {
  datagrid dgAccounts (DataSource: database from Administration.Account) {
    column (Attribute: FullName, Caption: 'Full name')
    column (Attribute: Name, Caption: 'Login')
    column (Caption: '', ShowContentAs: customContent) {
      actionbutton btnEdit (Caption: 'Edit', Action: microflow Administration.NewAccount)
    }
  }
};`
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if !rawPageContains(t, exec, "MyFirstModule.ActionsCol", "btnEdit") {
		t.Fatal("precondition: btnEdit not stored by create page")
	}

	described := describeOn(t, exec, out, "MyFirstModule.ActionsCol")
	if !strings.Contains(described, "column (Caption: '', ShowContentAs: customContent) {") ||
		!strings.Contains(described, "actionbutton btnEdit") {
		t.Fatalf("describe dropped the untitled actions column (mendixlabs/mxcli#1520):\n%s", described)
	}

	edited := strings.Replace(described, "Caption: 'Login'", "Caption: 'User name'", 1)
	if edited == described {
		t.Fatalf("fixture: no Login caption to edit in:\n%s", described)
	}
	if err := afRun(t, exec, edited); err != nil {
		t.Fatalf("re-exec edited describe output: %v\n%s", err, edited)
	}
	if !rawPageContains(t, exec, "MyFirstModule.ActionsCol", "User name") {
		t.Fatal("the edit did not land — the test proves nothing without it")
	}
	if !rawPageContains(t, exec, "MyFirstModule.ActionsCol", "btnEdit") {
		t.Errorf("describe -> edit -> exec deleted the actions column's button from the BSON")
	}
	after := describeOn(t, exec, out, "MyFirstModule.ActionsCol")
	if n := strings.Count(after, "\n    column ("); n != 3 {
		t.Errorf("after the round trip the grid has %d columns, want 3:\n%s", n, after)
	}
}

func rawPageContains(t *testing.T, exec *Executor, qn, needle string) bool {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	info, err := ctx.Backend.GetRawUnitByName("page", qn)
	if err != nil || info == nil {
		t.Fatalf("raw page %s: %v", qn, err)
	}
	return bytes.Contains(info.Contents, []byte(needle))
}
