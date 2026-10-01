// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// Rehearsal U1 (ako/mxcli#714, #706): `date` as a type stopped every run of
// mxcli-ledger's domain model, and fmt --upgrade could not get past it, so the
// migration needed a hand edit. It is a deprecated alias of DateTime again, and
// the upgrade writes DateTime — what was always stored. The two lines are the
// ledger's own (01-domain-model.mdl lines 95 and 188 before its migration), and
// the expected output is what the ledger committed after its hand edit.
func TestUpgrade_DateTypeBecomesDateTime(t *testing.T) {
	src := "create or modify persistent entity Ledger.Account (\n" +
		"  /** Date of the most recent successful import */\n" +
		"  LastImport: date,\n" +
		"  SortOrder: integer default 0\n" +
		");\n" +
		"create or modify persistent entity Ledger.Transaction (\n" +
		"  TxDate: date not null error 'Transaction date is required',\n" +
		"  Amount: decimal default 0\n" +
		");\n" +
		"create microflow Ledger.F ($D: DATE) returns Date begin declare $x date = $D; return $x; end;\n"
	want := "create or modify persistent entity Ledger.Account (\n" +
		"  /** Date of the most recent successful import */\n" +
		"  LastImport: DateTime,\n" +
		"  SortOrder: integer default 0\n" +
		");\n" +
		"create or modify persistent entity Ledger.Transaction (\n" +
		"  TxDate: DateTime not null error message 'Transaction date is required',\n" +
		"  Amount: decimal default 0\n" +
		");\n" +
		"create microflow Ledger.F ($D: DATETIME) returns DateTime begin declare $x DateTime = $D; return $x; end;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if n := res.Rewritten[deprecation.DateType]; n != 5 {
		t.Errorf("Rewritten[%s] = %d, want 5 (all: %v)", deprecation.DateType, n, res.Rewritten)
	}
	if len(res.Unrewritten) != 0 {
		t.Errorf("Unrewritten = %+v, want none", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
	// And with the header: the upgraded script is valid mdl 1.
	withHeader := mustUpgrade(t, src, Options{AddHeader: true})
	if !withHeader.HeaderAdded {
		t.Errorf("the header was not added:\n%s", withHeader.Source)
	}
}
