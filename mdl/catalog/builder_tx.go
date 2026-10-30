// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// builderTx is the transaction a build writes through. It records, for every
// row inserted, which unit the row came from (the builder's current owner),
// as rowid ranges per table. The next build carries an unchanged unit's rows
// over by those ranges instead of producing them again (ako/mxcli#1804; see
// builder_incremental.go).
type builderTx struct {
	CatalogTx
	b *Builder

	mu     sync.Mutex
	tables map[string]string // statement text -> table it inserts into ("" if none)
}

// Stmt is a prepared statement of a build. Exec records the rows an INSERT
// wrote; everything else is the *Stmt.
type Stmt struct {
	*sql.Stmt
	table string
	tx    *builderTx
}

func newBuilderTx(tx CatalogTx, b *Builder) *builderTx {
	return &builderTx{CatalogTx: tx, b: b, tables: map[string]string{}}
}

// insertInto matches the table an INSERT writes to.
var insertInto = regexp.MustCompile(`(?is)^\s*INSERT\s+(?:OR\s+\w+\s+)?INTO\s+([A-Za-z_][A-Za-z0-9_]*)`)

// insertTable is the table query inserts into, or "" when it is not an INSERT.
func (t *builderTx) insertTable(query string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	table, ok := t.tables[query]
	if !ok {
		if m := insertInto.FindStringSubmatch(query); m != nil {
			table = strings.ToLower(m[1])
		}
		t.tables[query] = table
	}
	return table
}

func (t *builderTx) Prepare(query string) (*Stmt, error) {
	s, err := t.CatalogTx.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &Stmt{Stmt: s, table: t.insertTable(query), tx: t}, nil
}

func (t *builderTx) Exec(query string, args ...any) (sql.Result, error) {
	res, err := t.CatalogTx.Exec(query, args...)
	if err == nil {
		if table := t.insertTable(query); table != "" {
			t.record(table, res)
		}
	}
	return res, err
}

func (s *Stmt) Exec(args ...any) (sql.Result, error) {
	res, err := s.Stmt.Exec(args...)
	if err == nil && s.table != "" {
		s.tx.record(s.table, res)
	}
	return res, err
}

// record notes the rows an INSERT wrote, when the transaction belongs to a
// build (a pass run on its own in a test has nobody to record for).
func (t *builderTx) record(table string, res sql.Result) {
	if t.b != nil && t.b.rows != nil {
		t.b.rows.record(table, res)
	}
}

// rowRange is a run of consecutive rowids of one table written for one unit.
type rowRange struct {
	owner       string
	first, last int64
}

// rowOwners is what a build records about the rows it wrote: which unit each
// one came from, and which units the per-unit passes visited at all.
type rowOwners struct {
	owner  string                // the unit the rows being written now come from; "" for project-wide rows
	ranges map[string][]rowRange // by table
	owned  map[string]bool       // every unit a per-unit pass visited, in scope or not
	err    error                 // the first row written for a unit this build skipped
}

func newRowOwners() *rowOwners {
	return &rowOwners{ranges: map[string][]rowRange{}, owned: map[string]bool{}}
}

// skippedOwner is the owner while a pass is between documents it does not
// rebuild. A row written then belongs to no unit the build can name, which is
// a bug in that pass: the build records it and falls back to a full one.
const skippedOwner = "\x00skipped"

func (o *rowOwners) record(table string, res sql.Result) {
	if strings.HasPrefix(table, "catalog_") {
		return // the build's own bookkeeping
	}
	n, err := res.RowsAffected()
	if err != nil || n <= 0 {
		return
	}
	last, err := res.LastInsertId()
	if err != nil {
		return
	}
	if o.owner == skippedOwner {
		if o.err == nil {
			o.err = fmt.Errorf("a row of %s was written while its pass was skipping a document", table)
		}
		return
	}
	o.add(table, o.owner, last-n+1, last)
}

// add appends a range, extending the table's last one when it continues it.
func (o *rowOwners) add(table, owner string, first, last int64) {
	rs := o.ranges[table]
	if k := len(rs) - 1; k >= 0 && rs[k].owner == owner && rs[k].last+1 == first {
		rs[k].last = last
		return
	}
	o.ranges[table] = append(rs, rowRange{owner, first, last})
}
