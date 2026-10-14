// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// namedItemList is a stored list of named elements: the document holding it
// and the key it is under, the pair findInWidgetArray takes.
type namedItemList struct {
	doc bson.D
	key string
}

// legacyGridItemLists returns the named-item lists of a legacy (Dojo)
// Forms$DataGrid other than its control bar, which the finder already walks:
// the grid's Columns (Forms$GridColumn) and its source's SearchBar.Items
// (Forms$*SearchField). Each item carries a stored Name, so `drop <name>` and
// `set (Caption: …) on <name>` address them like any widget. Without this both
// were unreachable and the only remedy was replacing the grid
// (mendixlabs/mxcli#1459).
//
// Gated on the $Type: a LayoutGrid row also has a `Columns` key, holding
// layout columns rather than named grid columns.
func legacyGridItemLists(wDoc bson.D) []namedItemList {
	if bsonnav.DGetString(wDoc, "$Type") != "Forms$DataGrid" {
		return nil
	}
	lists := []namedItemList{{wDoc, "Columns"}}
	if ds := bsonnav.DGetDoc(wDoc, "DataSource"); ds != nil {
		if sb := bsonnav.DGetDoc(ds, "SearchBar"); sb != nil {
			lists = append(lists, namedItemList{sb, "Items"})
		}
	}
	return lists
}
