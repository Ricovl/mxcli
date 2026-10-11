// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// carryLayoutGridPreviewWidth copies Forms$LayoutGridColumn.PreviewWidth from
// the stored document onto the rebuilt columns.
//
// PreviewWidth is a Studio Pro canvas value — the width a column is drawn at in
// the editor — with no MDL spelling and no runtime effect. layoutGridColumnToGen
// writes -1, which is what Studio Pro stores on 731 of 732 columns across
// ako/TestApp and PedApp, so the hardcode looked safe; the one column that
// stored 1 (ako/mxcli-ledger FINDINGS #170) was rewritten by every
// describe → exec. Carried for the reason carryStoredPageHeader gives: a value
// nobody asked to change should not change.
//
// A column has no name, so it is paired by position inside the stored grid of
// the same name: row i, column j. A row whose column count differs from the
// stored row's is not carried — its columns cannot be paired — and a column of
// a grid with no stored counterpart keeps -1. An ambiguous grid name is skipped.
//
// A missing or unreadable stored unit leaves the rebuilt values in place.
func (b *Backend) carryLayoutGridPreviewWidth(id model.ID, root element.Element) {
	if b.reader == nil || id == "" || root == nil {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return
	}
	var stored bson.D
	if err := bson.Unmarshal(raw, &stored); err != nil {
		return
	}
	carryLayoutGridPreviewWidthFrom(stored, root)
}

// carryLayoutGridPreviewWidthFrom is carryLayoutGridPreviewWidth against an
// already decoded stored document.
func carryLayoutGridPreviewWidthFrom(stored bson.D, root element.Element) {
	storedWidgets := map[string]bson.D{}
	ambiguous := map[string]bool{}
	indexNamedDocs(stored, storedWidgets, ambiguous)

	element.Walk(root, func(e element.Element) bool {
		grid, ok := e.(*genPg.LayoutGrid)
		if !ok || grid.Name() == "" || ambiguous[grid.Name()] {
			return true
		}
		sg, ok := storedWidgets[grid.Name()]
		if !ok || bsonnav.DGetString(sg, "$Type") != "Forms$LayoutGrid" {
			return true
		}
		storedRows := bsonnav.DGetArrayElements(bsonnav.DGet(sg, "Rows"))
		for i, r := range grid.RowsItems() {
			row, ok := r.(*genPg.LayoutGridRow)
			if !ok || i >= len(storedRows) {
				continue
			}
			sr, ok := storedRows[i].(bson.D)
			if !ok {
				continue
			}
			storedCols := bsonnav.DGetArrayElements(bsonnav.DGet(sr, "Columns"))
			cols := row.ColumnsItems()
			if len(cols) != len(storedCols) {
				continue
			}
			for j, c := range cols {
				col, ok := c.(*genPg.LayoutGridColumn)
				if !ok {
					continue
				}
				sc, ok := storedCols[j].(bson.D)
				if !ok {
					continue
				}
				// Only a differing value is set: setting an equal one marks the
				// column dirty and re-encodes an element that was kept (#721 L4).
				if v, ok := storedInt32(bsonnav.DGet(sc, "PreviewWidth")); ok && col.PreviewWidth() != v {
					col.SetPreviewWidth(v)
				}
			}
		}
		return true
	})
}

// storedInt32 reads a BSON integer as int32, whichever width it was stored at.
func storedInt32(v any) (int32, bool) {
	switch x := v.(type) {
	case int32:
		return x, true
	case int64:
		return int32(x), true
	case int:
		return int32(x), true
	}
	return 0, false
}
