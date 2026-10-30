// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"reflect"
	"sort"
	"testing"
)

// fakeResult is the sql.Result of an INSERT that wrote n rows ending at last.
type fakeResult struct{ last, n int64 }

func (r fakeResult) LastInsertId() (int64, error) { return r.last, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.n, nil }

func TestRowOwners_RecordsRangesPerOwner(t *testing.T) {
	o := newRowOwners()
	o.owner = "a"
	o.record("refs", fakeResult{1, 1})
	o.record("refs", fakeResult{3, 2}) // an INSERT ... SELECT of rows 2-3
	o.owner = "b"
	o.record("refs", fakeResult{4, 1})
	o.owner = "a"
	o.record("refs", fakeResult{5, 1})
	o.record("refs", fakeResult{5, 0})          // ignored by INSERT OR IGNORE
	o.record("catalog_units", fakeResult{9, 1}) // the build's bookkeeping

	want := []rowRange{{"a", 1, 3}, {"b", 4, 4}, {"a", 5, 5}}
	if got := o.ranges["refs"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ranges = %+v, want %+v", got, want)
	}
	if _, ok := o.ranges["catalog_units"]; ok {
		t.Error("the build's own bookkeeping was recorded as rows of a unit")
	}
	if o.err != nil {
		t.Errorf("unexpected error: %v", o.err)
	}
}

// A row written while a pass skips a document belongs to no unit the build
// can name; the build must notice, or the next one carries a row nobody
// rebuilds.
func TestRowOwners_RowForASkippedDocumentIsAnError(t *testing.T) {
	b := &Builder{scope: map[string]bool{"in": true}}
	if !b.own("in") {
		t.Fatal("a unit in scope was skipped")
	}
	if b.own("out") {
		t.Fatal("a unit out of scope was not skipped")
	}
	b.rows.record("refs", fakeResult{1, 1})
	if b.rows.err == nil {
		t.Fatal("a row written for a skipped document was not reported")
	}
	if !b.rows.owned["in"] || !b.rows.owned["out"] {
		t.Error("own did not record the units the pass visited")
	}
}

func TestBuilderTx_InsertTable(t *testing.T) {
	tx := newBuilderTx(nil, nil)
	for q, want := range map[string]string{
		"INSERT INTO refs (A) VALUES (?)":               "refs",
		"\n\t\tinsert or ignore into Contract_X values": "contract_x",
		"INSERT OR REPLACE INTO source_units VALUES":    "source_units",
		"UPDATE contract_entities_data SET X = 1":       "",
		"SELECT 1": "",
	} {
		if got := tx.insertTable(q); got != want {
			t.Errorf("insertTable(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestDependentUnits(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, s := range []string{
		`CREATE TABLE refs (SourceName TEXT, TargetName TEXT, RefKind TEXT)`,
		`CREATE TABLE catalog_unit_names (UnitId TEXT, QualifiedName TEXT)`,
		// M.Sub specializes M.Base; Other.Sub2 specializes M.Sub.
		`INSERT INTO refs VALUES ('M.Sub', 'M.Base', 'generalize')`,
		`INSERT INTO refs VALUES ('Other.Sub2', 'M.Sub', 'generalize')`,
		// A flow naming a member of the changed entity, one naming an
		// inherited member, and one naming something else.
		`INSERT INTO refs VALUES ('M.FlowA', 'M.Base.Name', 'member')`,
		`INSERT INTO refs VALUES ('X.FlowB', 'Other.Sub2.Name', 'member')`,
		`INSERT INTO refs VALUES ('X.FlowC', 'M.Basement', 'call')`,
		`INSERT INTO catalog_unit_names VALUES ('dm-m', 'M.Base'), ('dm-m', 'M.Sub'), ('dm-other', 'Other.Sub2'),
			('flow-a', 'M.FlowA'), ('flow-b', 'X.FlowB'), ('flow-c', 'X.FlowC')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := dependentUnits(db.Query, []string{"M.Base"})
	if err != nil {
		t.Fatal(err)
	}
	var units []string
	for u := range got {
		units = append(units, u)
	}
	sort.Strings(units)
	// M.Basement shares a prefix but is not a member: FlowC is not one.
	want := []string{"dm-m", "dm-other", "flow-a", "flow-b"}
	if !reflect.DeepEqual(units, want) {
		t.Errorf("dependents = %v, want %v", units, want)
	}
}
