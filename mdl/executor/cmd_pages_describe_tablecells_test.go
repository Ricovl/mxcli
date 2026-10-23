// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1453 (ako/mxcli#1326): ALTER PAGE addresses a legacy table
// cell as `table1 cell(row, col)`, but describe printed the table as one
// unauthorable line, so neither the address nor which cell held what could be
// read back. The cells are listed under it, 1-based, in the converted shape a
// Mendix 11.13 table is stored in (Forms$DbTableCell, 0-based indexes).
func TestDescribeLegacyTable_ListsCellAddresses(t *testing.T) {
	cell := func(row, col int32, widgets ...any) map[string]any {
		return map[string]any{
			"$Type": "Forms$DbTableCell", "TopRowIndex": row, "LeftColumnIndex": col,
			"Width": int32(1), "Height": int32(1), "IsHeader": false,
			"Widgets": append([]any{int32(2)}, widgets...),
		}
	}
	table := map[string]any{
		"$Type": "Forms$Table",
		"Name":  "table1",
		"Cells": []any{int32(2),
			cell(0, 0, map[string]any{"$Type": "Forms$DynamicText", "Name": "tInCell"}),
			cell(0, 1),
			cell(1, 0, map[string]any{"$Type": "Forms$DynamicText", "Name": "tA"},
				map[string]any{"$Type": "Forms$DynamicText", "Name": "tB"}),
		},
	}
	got := describeStoredWidget(t, table)
	for _, want := range []string{
		"-- Forms$Table (table1)",
		"--   cell(1,1): tInCell",
		"--   cell(1,2): (empty)",
		"--   cell(2,1): tA, tB",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("description lacks %q:\n%s", want, got)
		}
	}
}
