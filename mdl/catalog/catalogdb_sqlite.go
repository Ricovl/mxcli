// SPDX-License-Identifier: Apache-2.0

//go:build !js

package catalog

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// SqliteCatalogDB wraps *sql.DB from modernc.org/sqlite.
type SqliteCatalogDB struct {
	db *sql.DB
}

// NewSqliteCatalogDB opens an in-memory SQLite database.
func NewSqliteCatalogDB() (*SqliteCatalogDB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	return &SqliteCatalogDB{db: db}, nil
}

// fileBusyTimeoutMs is how long a connection to an on-disk catalog waits for a
// lock held by another process before failing with SQLITE_BUSY. Opening a cache
// writes to it (schema upgrade, schema version), so parallel mxcli processes on
// one project contend for the write lock for a moment (ako/mxcli#951).
const fileBusyTimeoutMs = 10000

// NewSqliteCatalogDBFromFile opens a file-based SQLite database.
func NewSqliteCatalogDBFromFile(path string) (*SqliteCatalogDB, error) {
	dsn := path
	// The busy timeout goes in the DSN so it applies to every pooled connection,
	// not only the one a PRAGMA statement happens to run on. A '?' in the path
	// would be read as the start of the query string; keep the bare path then.
	if !strings.Contains(path, "?") {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(%d)", path, fileBusyTimeoutMs)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	return &SqliteCatalogDB{db: db}, nil
}

func (s *SqliteCatalogDB) Query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *SqliteCatalogDB) QueryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(query, args...)
}

func (s *SqliteCatalogDB) Exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *SqliteCatalogDB) Begin() (CatalogTx, error) {
	return s.db.Begin()
}

func (s *SqliteCatalogDB) Close() error {
	return s.db.Close()
}

// RawDB returns the underlying *sql.DB. Used only for SQLite-specific
// operations like VACUUM INTO in SaveToFile.
func (s *SqliteCatalogDB) RawDB() *sql.DB {
	return s.db
}

// WrapSqlDB wraps an existing *sql.DB as a CatalogDB.
// Used by tests that create their own in-memory databases.
func WrapSqlDB(db *sql.DB) *SqliteCatalogDB {
	return &SqliteCatalogDB{db: db}
}
