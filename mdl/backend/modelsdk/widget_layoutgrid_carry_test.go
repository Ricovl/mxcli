// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func storedGridColumn(previewWidth int32) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$LayoutGridColumn"},
		{Key: "PreviewWidth", Value: previewWidth},
		{Key: "Widgets", Value: bson.A{int32(2)}},
	}
}

func storedGrid(name string, rows ...bson.A) bson.D {
	rowDocs := bson.A{int32(2)}
	for _, cols := range rows {
		rowDocs = append(rowDocs, bson.D{
			{Key: "$Type", Value: "Forms$LayoutGridRow"},
			{Key: "Columns", Value: append(bson.A{int32(2)}, cols...)},
		})
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$LayoutGrid"},
		{Key: "Name", Value: name},
		{Key: "Rows", Value: rowDocs},
	}
}

func builtGrid(t *testing.T, name string, colsPerRow ...int) *genPg.LayoutGrid {
	t.Helper()
	lg := &pages.LayoutGrid{BaseWidget: pages.BaseWidget{Name: name}}
	for _, n := range colsPerRow {
		row := &pages.LayoutGridRow{}
		for i := 0; i < n; i++ {
			row.Columns = append(row.Columns, &pages.LayoutGridColumn{})
		}
		lg.Rows = append(lg.Rows, row)
	}
	el, err := widgetToGen(lg)
	if err != nil {
		t.Fatal(err)
	}
	return el.(*genPg.LayoutGrid)
}

func previewWidths(g *genPg.LayoutGrid) [][]int32 {
	var out [][]int32
	for _, r := range g.RowsItems() {
		var row []int32
		for _, c := range r.(*genPg.LayoutGridRow).ColumnsItems() {
			row = append(row, c.(*genPg.LayoutGridColumn).PreviewWidth())
		}
		out = append(out, row)
	}
	return out
}

// Forms$LayoutGridColumn.PreviewWidth is a Studio Pro canvas value with no MDL
// spelling and no runtime effect. The writer hardcoded -1, so a describe → exec
// of a page whose column stored 1 rewrote it (ako/mxcli-ledger FINDINGS #170).
// The stored value is carried onto the rebuilt column at the same position of
// the same-named grid.
func TestCarryLayoutGridPreviewWidth(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2),
			storedGrid("lg1", bson.A{storedGridColumn(1), storedGridColumn(-1)}),
		}},
	}
	g := builtGrid(t, "lg1", 2)
	other := builtGrid(t, "lgNew", 2)
	root := genPg.NewDataView()
	root.SetName("dv")
	root.AddWidgets(g)
	root.AddWidgets(other)

	carryLayoutGridPreviewWidthFrom(stored, root)

	if got := previewWidths(g); got[0][0] != 1 || got[0][1] != -1 {
		t.Errorf("lg1 preview widths = %v, want [[1 -1]] carried from the stored grid", got)
	}
	// Control: a grid with no stored counterpart keeps the writer's -1.
	if got := previewWidths(other); got[0][0] != -1 || got[0][1] != -1 {
		t.Errorf("lgNew preview widths = %v; a new grid took a value from nowhere", got)
	}
}

// A row whose column count no longer matches the stored one is not carried:
// the columns cannot be paired, and a new column keeps -1.
func TestCarryLayoutGridPreviewWidth_ReshapedRowKeepsDefault(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2),
			storedGrid("lg1", bson.A{storedGridColumn(1)}),
		}},
	}
	g := builtGrid(t, "lg1", 2)
	carryLayoutGridPreviewWidthFrom(stored, g)
	if got := previewWidths(g); got[0][0] != -1 || got[0][1] != -1 {
		t.Errorf("preview widths = %v, want [[-1 -1]] for a reshaped row", got)
	}
}

// Carrying the value a decoded column already holds must leave it clean, so a
// kept element is not re-encoded (the #721 L4 rule carryNoActionExecution keeps).
func TestCarryLayoutGridPreviewWidth_EqualValueLeavesClean(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2),
			storedGrid("lg1", bson.A{storedGridColumn(-1)}),
		}},
	}
	raw, err := bson.Marshal(storedGrid("lg1", bson.A{storedGridColumn(-1)}))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	carryLayoutGridPreviewWidthFrom(stored, kept)
	element.Walk(kept, func(e element.Element) bool {
		if e.IsDirty() {
			t.Errorf("%s dirtied by carrying the value it already holds", e.TypeName())
		}
		return true
	})
}
