// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1461: a layout-grid row carries its own visibility
// (Forms$LayoutGridRow.ConditionalVisibilitySettings — 483 of 483 rows in
// TestApp + PedApp have the key, TestApp's Snip_DashboardContext_Timeline sets
// one). DESCRIBE printed a bare `row { … }`, and `row (Visible: …)` passed
// check and exec and was not written, so describe → exec made every such row
// always visible. A row is built by buildLayoutGridV3, not buildWidgetV3, so
// the visibility tail every widget gets never ran for it (as for a tab page,
// #1465).

func layoutGridOf(t *testing.T, body string) *ast.WidgetV3 {
	t.Helper()
	src := "create page M.P (Title: 'x', Layout: A.L, Params: ($Job: M.Job)) {\n" +
		"  dataview dv (DataSource: $Job) {\n" + body + "  }\n};\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	return prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children[0]
}

func TestLayoutGridRow_VisibleIsWritten(t *testing.T) {
	lgw := layoutGridOf(t, `
    layoutgrid lg {
      row { column (DesktopWidth: AutoFill) { dynamictext t1 (Content: 'A') } }
      row (Visible: $currentObject/IsLocal) { column (DesktopWidth: AutoFill) { dynamictext t2 (Content: 'B') } }
      row (Class: 'r3', Visible: Status in (Running, empty)) { column (DesktopWidth: AutoFill) { dynamictext t3 (Content: 'C') } }
      row (Visible: false) { column (DesktopWidth: AutoFill) { dynamictext t4 (Content: 'D') } }
    }
`)
	built, err := visibleWhenPB("M.Job").buildWidgetV3(lgw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	rows := built.(*pages.LayoutGrid).Rows
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	// Control: a row without Visible: stays without a setting.
	if cv := rows[0].ConditionalVisibility; cv != nil {
		t.Errorf("row 1: a row without Visible: got a visibility setting %+v", cv)
	}
	if cv := rows[1].ConditionalVisibility; cv == nil || cv.Expression != "$currentObject/IsLocal" {
		t.Errorf("row 2: ConditionalVisibility = %+v, want Expression $currentObject/IsLocal — the condition was dropped (row always visible)", cv)
	}
	if cv := rows[2].ConditionalVisibility; cv == nil || cv.Attribute != "M.Job.Status" || len(cv.Conditions) != 4 {
		t.Errorf("row 3: ConditionalVisibility = %+v, want the attribute condition on M.Job.Status", cv)
	}
	if rows[2].Class != "r3" {
		t.Errorf("row 3: Class = %q, want r3", rows[2].Class)
	}
	if cv := rows[3].ConditionalVisibility; cv == nil || cv.Expression != "false" {
		t.Errorf("row 4: ConditionalVisibility = %+v, want Expression false", cv)
	}
}

// A top-level `row` builds a container around a one-row grid, and the
// container carries the row's Visible: (as it carries its appearance) — it
// must not be written on the row a second time.
func TestLayoutGridRow_TopLevelRowVisibleOnContainerOnly(t *testing.T) {
	roww := layoutGridOf(t, `
    row r (Visible: $currentObject/IsLocal) { column (DesktopWidth: AutoFill) { dynamictext t1 (Content: 'A') } }
`)
	built, err := visibleWhenPB("M.Job").buildWidgetV3(roww)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	c, ok := built.(*pages.Container)
	if !ok {
		t.Fatalf("got %T, want the wrapping container", built)
	}
	if cv := c.ConditionalVisibility; cv == nil || cv.Expression != "$currentObject/IsLocal" {
		t.Errorf("container: ConditionalVisibility = %+v, want the row's condition", cv)
	}
	row := c.Widgets[0].(*pages.LayoutGrid).Rows[0]
	if row.ConditionalVisibility != nil {
		t.Errorf("row: ConditionalVisibility = %+v, want none — the container already carries it", row.ConditionalVisibility)
	}
}

// DESCRIBE reads a row's visibility back, so describe -> exec keeps it.
// The stored shape is TestApp's WorkflowCommons.Snip_DashboardContext_Timeline.
func TestDescribe_LayoutGridRowVisible(t *testing.T) {
	col := map[string]any{"$Type": "Forms$LayoutGridColumn", "Weight": int32(-1), "TabletWeight": int32(-1), "PhoneWeight": int32(-1), "Widgets": []any{int32(2)}}
	stored := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Name":  "lg",
		"Rows": []any{int32(2),
			map[string]any{"$Type": "Forms$LayoutGridRow", "SpacingBetweenColumns": true, "Columns": []any{int32(2), col}},
			map[string]any{
				"$Type": "Forms$LayoutGridRow", "SpacingBetweenColumns": true, "Columns": []any{int32(2), col},
				"ConditionalVisibilitySettings": map[string]any{
					"$Type":      "Forms$ConditionalVisibilitySettings",
					"Expression": "$currentObject/Outcome != empty",
				},
			},
		},
	}
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, stored)
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	if !strings.Contains(got, "row (Visible: $currentObject/Outcome != empty) {") {
		t.Errorf("describe output lacks the row's Visible:\n%s", got)
	}
	if strings.Count(got, "Visible:") != 1 {
		t.Errorf("only the second row has a visibility setting:\n%s", got)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"); len(errs) > 0 {
		t.Errorf("describe output does not parse: %v\n%s", errs, got)
	}
}

// Forms$LayoutGridColumn has no ConditionalVisibilitySettings (none of 732
// stored columns in TestApp + PedApp has the key; the 11.x metamodel has no
// such property), so `Visible:` on a column inside a row is dropped. check
// says so. Controls: the row's own Visible:, and a top-level `column`, whose
// wrapping container carries it, draw nothing.
func TestCheck_LayoutGridColumnVisibleDropped(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: A.L, Params: ($Job: M.Job)) {
  dataview dv (DataSource: $Job) {
    layoutgrid lg {
      row (Visible: $currentObject/IsLocal) {
        column c1 (DesktopWidth: AutoFill, Visible: $Job/IsLocal) { dynamictext t1 (Content: 'A') }
        column (DesktopWidth: AutoFill) { dynamictext t2 (Content: 'B') }
      }
    }
    column c3 (DesktopWidth: AutoFill, Visible: $currentObject/IsLocal) { dynamictext t3 (Content: 'C') }
  }
};
`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	reg, _ := NewWidgetRegistry()
	var hits []linter.Violation
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], reg) {
		if strings.Contains(v.Message, "layout-grid column") {
			hits = append(hits, v)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("got %d layout-grid column visibility violations, want 1 (c1): %+v", len(hits), hits)
	}
	if hits[0].RuleID != "MDL-WIDGET07" || hits[0].Severity != linter.SeverityWarning || !strings.Contains(hits[0].Message, "column 1 of its row") {
		t.Errorf("got %s/%v %q, want an MDL-WIDGET07 warning naming column 1", hits[0].RuleID, hits[0].Severity, hits[0].Message)
	}
}
