// SPDX-License-Identifier: Apache-2.0

package catalog

import "database/sql"

// CatalogDB abstracts the database layer used by Catalog and LintContext.
// The primary implementation wraps modernc.org/sqlite for native builds;
// a js/wasm build will supply a host-backed implementation.
type CatalogDB interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	Begin() (CatalogTx, error)
	Close() error
}

// CatalogTx abstracts a database transaction. Builder uses this for bulk
// inserts with prepared statements.
type CatalogTx interface {
	Prepare(query string) (*sql.Stmt, error)
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	// Query reads a multi-row result inside the transaction — the member-ref
	// pass needs the name sets earlier passes wrote (attributes, associations,
	// enumerations) and the XPath constraints buildXPathExpressions recorded.
	Query(query string, args ...any) (*sql.Rows, error)
	Commit() error
	Rollback() error
}
