// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#721 L1: a layout-grid column's widths did not survive
// describe → exec. Studio Pro stores Weight / TabletWeight / PhoneWeight as
// int64 and describe only accepted int32, so every column came back as
// `DesktopWidth: AutoFill` with no tablet or phone width; and the auto-fit
// width (-2) had no spelling at all. Re-running a described page reset every
// column to auto-fill.

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Every (Weight, TabletWeight, PhoneWeight) combination found in the PedApp and
// TestApp fixtures, as Studio Pro stores it (int64).
var fixtureColumnWeights = [][3]int64{
	{-1, -1, -1}, {-2, -2, -2}, {12, 12, 12}, {-1, 12, 12}, {-2, -1, -1},
	{4, 12, 12}, {3, 12, 12}, {3, -1, -1}, {4, -1, -1}, {8, 12, 12},
	{2, 12, 12}, {6, 12, 12}, {2, 2, 2}, {2, -1, -1}, {-1, -1, 12},
	{9, 12, 12}, {6, -1, -1}, {10, -1, -1}, {1, -1, -1}, {9, -1, -1},
	{-1, -2, -2},
}

func describeLayoutGrid(t *testing.T, cols [][3]int64) string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	var columns []any
	for _, c := range cols {
		columns = append(columns, map[string]any{
			"$Type":        "Forms$LayoutGridColumn",
			"Weight":       c[0],
			"TabletWeight": c[1],
			"PhoneWeight":  c[2],
		})
	}
	raw := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Name":  "lg",
		"Rows": []any{map[string]any{
			"$Type":   "Forms$LayoutGridRow",
			"Columns": columns,
		}},
	}
	widgets := parseRawWidget(ctx, raw)
	if len(widgets) != 1 {
		t.Fatalf("parseRawWidget: %d widgets, want 1", len(widgets))
	}
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, widgets[0], 1)
	return buf.String()
}

// Describe prints a spelling for every stored width, and building the printed
// MDL writes the stored weights back.
func TestLayoutGridColumnWeightsRoundTrip(t *testing.T) {
	mdl := describeLayoutGrid(t, fixtureColumnWeights)
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + mdl + "};"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("described layout grid does not parse: %v\n%s", errs, src)
	}
	page, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("statement is %T", prog.Statements[0])
	}
	grid := page.Widgets[0]
	row := grid.Children[0]
	if len(row.Children) != len(fixtureColumnWeights) {
		t.Fatalf("%d columns parsed, want %d\n%s", len(row.Children), len(fixtureColumnWeights), mdl)
	}
	pb := &pageBuilder{}
	for i, colAST := range row.Children {
		col, err := pb.buildLayoutGridColumnV3(colAST)
		if err != nil {
			t.Fatalf("column %d: %v", i, err)
		}
		// The writer maps an unset (0) weight to -1; apply the same so the
		// comparison is against what lands in the model.
		got := [3]int64{written(col.Weight), written(col.TabletWeight), written(col.PhoneWeight)}
		if got != fixtureColumnWeights[i] {
			t.Errorf("column %d: stored %v, re-executed describe writes %v\n%s",
				i, fixtureColumnWeights[i], got, lineOf(mdl, i))
		}
	}
}

// The auto-fit spelling is AutoFit; an explicit width prints as its number.
func TestLayoutGridColumnWeightsDescribeSpelling(t *testing.T) {
	out := describeLayoutGrid(t, [][3]int64{{4, 12, 12}, {-2, -2, -2}, {-1, -1, -1}})
	for _, want := range []string{
		"column (DesktopWidth: 4, TabletWidth: 12, PhoneWidth: 12)",
		"column (DesktopWidth: AutoFit, TabletWidth: AutoFit, PhoneWidth: AutoFit)",
		"column (DesktopWidth: AutoFill)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe output lacks %q:\n%s", want, out)
		}
	}
}

func written(w int) int64 {
	if w == 0 {
		return -1
	}
	return int64(w)
}

func lineOf(mdl string, col int) string {
	n := 0
	for _, l := range strings.Split(mdl, "\n") {
		if strings.Contains(l, "column (") {
			if n == col {
				return l
			}
			n++
		}
	}
	return fmt.Sprintf("<column %d not found>", col)
}
