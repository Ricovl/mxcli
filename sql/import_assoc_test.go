// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // in-process stand-in for the Mendix PostgreSQL database
)

// The fixtures below mirror what the Mendix runtime creates, measured with psql
// on databases booted by Mendix 10.24.24, 11.12.2, 11.13.0 and 11.14.0 (ako/mxcli#1454,
// upstream mendixlabs/mxcli#1557): mendixsystem$association.storage_format is a
// smallint, 1 for an association stored as a foreign-key column on the owner's
// table and 2 for one stored in a junction table. No measured version stored
// the strings "Column"/"Table" the import compared against, so every LINK, of
// either kind, linked nothing while the import reported success.

const (
	colAssoc = "MyFirstModule.EmployeeCol_Department"
	tblAssoc = "MyFirstModule.EmployeeTbl_Department"
)

// openSQLite opens a fresh on-disk SQLite database and runs the fixture
// statements against it.
func openSQLite(t *testing.T, name string, stmts ...string) *Connection {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), name+".db"))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	// One connection, so the information_schema ATTACH is visible to every
	// statement the pipeline issues.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	return &Connection{DB: db, Alias: name}
}

// mendixTarget is a Mendix app database with the domain model of the
// measurement: Department, EmployeeCol -> Department stored as a column, and
// EmployeeTbl -> Department stored in a junction table. colFormat and tblFormat
// are the SQL literals stored in storage_format for the two associations; the
// column is declared without a type so a literal keeps the type it was written
// with, as it would come back from a driver.
func mendixTarget(t *testing.T, colFormat, tblFormat string) *Connection {
	return openSQLite(t, "mendix",
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.columns (table_name TEXT, column_name TEXT)`,
		`CREATE TABLE mendixsystem$entity (id TEXT PRIMARY KEY, entity_name TEXT)`,
		`CREATE TABLE mendixsystem$entityidentifier (id TEXT PRIMARY KEY, short_id INTEGER, object_sequence INTEGER)`,
		`CREATE TABLE mendixsystem$association (id TEXT, association_name TEXT, table_name TEXT,
			parent_column_name TEXT, child_column_name TEXT, storage_format)`,
		`INSERT INTO mendixsystem$entity VALUES
			('e-dep', 'MyFirstModule.Department'),
			('e-col', 'MyFirstModule.EmployeeCol'),
			('e-tbl', 'MyFirstModule.EmployeeTbl')`,
		`INSERT INTO mendixsystem$entityidentifier VALUES ('e-dep', 1, 3), ('e-col', 2, 0), ('e-tbl', 3, 0)`,
		// The two rows exactly as Mendix 11.14.0 wrote them.
		`INSERT INTO mendixsystem$association VALUES
			('a-col', '`+colAssoc+`', 'myfirstmodule$employeecol', 'id',
			 'myfirstmodule$employeecol_department', `+colFormat+`),
			('a-tbl', '`+tblAssoc+`', 'myfirstmodule$employeetbl_department', 'myfirstmodule$employeetblid',
			 'myfirstmodule$departmentid', `+tblFormat+`)`,
		`CREATE TABLE "myfirstmodule$department" (id INTEGER PRIMARY KEY, name TEXT)`,
		`INSERT INTO "myfirstmodule$department" VALUES (101, 'Sales'), (102, 'Engineering'), (103, 'Finance')`,
		`CREATE TABLE "myfirstmodule$employeecol" (id INTEGER PRIMARY KEY, name TEXT,
			"myfirstmodule$employeecol_department" INTEGER)`,
		`CREATE TABLE "myfirstmodule$employeetbl" (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE "myfirstmodule$employeetbl_department" (
			"myfirstmodule$employeetblid" INTEGER NOT NULL, "myfirstmodule$departmentid" INTEGER NOT NULL)`,
	)
}

// sourceDB is the external database the rows are imported from. Eve's
// department does not exist, so one row per import must stay unlinked.
func sourceDB(t *testing.T) *Connection {
	return openSQLite(t, "source",
		`CREATE TABLE employees (name TEXT, dept TEXT)`,
		`INSERT INTO employees VALUES ('Alice', 'Sales'), ('Bob', 'Engineering'),
			('Carol', 'Finance'), ('Dave', 'Sales'), ('Eve', 'NoSuchDept')`,
	)
}

// linkFromSystemTable resolves `link (dept to <assoc> on Name)` the way
// mdl/executor.resolveOneLink does once mendixsystem$association has a row for
// the association: the storage format and the FK column / junction table come
// from that row, the junction's column names from the naming convention.
func linkFromSystemTable(t *testing.T, conn *Connection, assoc, parentEntity string) *AssocInfo {
	t.Helper()
	ctx := context.Background()
	sys, err := LookupAssociationInfo(ctx, conn, assoc)
	if err != nil {
		t.Fatalf("LookupAssociationInfo(%s): %v", assoc, err)
	}
	if sys == nil {
		t.Fatalf("LookupAssociationInfo(%s): no row", assoc)
	}
	cache, err := BuildLookupCache(ctx, conn, "myfirstmodule$department", "name")
	if err != nil {
		t.Fatalf("BuildLookupCache: %v", err)
	}
	info := &AssocInfo{
		SourceColumn:    "dept",
		LookupAttr:      "Name",
		AssociationName: assoc,
		ChildEntity:     "MyFirstModule.Department",
		StorageFormat:   sys.StorageFormat,
		LookupCache:     cache,
	}
	if sys.StorageFormat == "Column" {
		info.FKColumnName = sys.ChildColumnName
	} else {
		info.JunctionTable = sys.TableName
		info.ParentColName = JunctionColumnFromConvention(parentEntity)
		info.ChildColName = JunctionColumnFromConvention("MyFirstModule.Department")
	}
	return info
}

func importEmployees(t *testing.T, source, target *Connection, entity string, link *AssocInfo) (*ImportResult, error) {
	t.Helper()
	table, err := EntityToTableName(entity)
	if err != nil {
		t.Fatal(err)
	}
	return ExecuteImport(context.Background(), &ImportConfig{
		SourceConn:  source,
		TargetConn:  target,
		SourceQuery: "SELECT name, dept FROM employees",
		TargetTable: table,
		EntityName:  entity,
		ColumnMap:   []ColumnMapping{{SourceName: "name", TargetName: "name"}},
		Assocs:      []*AssocInfo{link},
	}, nil)
}

// linkedDepartments returns employee name -> linked department name ("" when
// the association is empty), read back with SQL from wherever the link lives.
func linkedDepartments(t *testing.T, conn *Connection, query string) map[string]string {
	t.Helper()
	rows, err := conn.DB.Query(query)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var name string
		var dept sql.NullString
		if err := rows.Scan(&name, &dept); err != nil {
			t.Fatal(err)
		}
		got[name] = dept.String
	}
	return got
}

var wantDepartments = map[string]string{
	"Alice": "Sales", "Bob": "Engineering", "Carol": "Finance", "Dave": "Sales", "Eve": "",
}

func assertLinked(t *testing.T, what string, got map[string]string, res *ImportResult, assoc string) {
	t.Helper()
	for name, want := range wantDepartments {
		if got[name] != want {
			t.Errorf("%s: %s linked to %q, want %q (all: %v)", what, name, got[name], want, got)
		}
	}
	if res.LinksCreated[assoc] != 4 || res.LinksMissed[assoc] != 1 {
		t.Errorf("%s: reported %d linked / %d missed, want 4 / 1",
			what, res.LinksCreated[assoc], res.LinksMissed[assoc])
	}
}

// TestImportLink_PopulatesBothStorageFormats is the regression guard for
// ako/mxcli#1454: with the system rows as Mendix writes them (storage_format 1
// and 2), a LINK must fill the owner's FK column for a column-stored
// association and the junction table for a table-stored one.
func TestImportLink_PopulatesBothStorageFormats(t *testing.T) {
	source := sourceDB(t)
	target := mendixTarget(t, "1", "2")

	t.Run("column storage (storage_format 1)", func(t *testing.T) {
		link := linkFromSystemTable(t, target, colAssoc, "MyFirstModule.EmployeeCol")
		res, err := importEmployees(t, source, target, "MyFirstModule.EmployeeCol", link)
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		got := linkedDepartments(t, target, `SELECT e.name, d.name FROM "myfirstmodule$employeecol" e
			LEFT JOIN "myfirstmodule$department" d ON d.id = e."myfirstmodule$employeecol_department"`)
		assertLinked(t, "FK column", got, res, colAssoc)
	})

	t.Run("table storage (storage_format 2)", func(t *testing.T) {
		link := linkFromSystemTable(t, target, tblAssoc, "MyFirstModule.EmployeeTbl")
		res, err := importEmployees(t, source, target, "MyFirstModule.EmployeeTbl", link)
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		got := linkedDepartments(t, target, `SELECT e.name, d.name FROM "myfirstmodule$employeetbl" e
			LEFT JOIN "myfirstmodule$employeetbl_department" j ON j."myfirstmodule$employeetblid" = e.id
			LEFT JOIN "myfirstmodule$department" d ON d.id = j."myfirstmodule$departmentid"`)
		assertLinked(t, "junction table", got, res, tblAssoc)
	})
}

// TestLookupAssociationInfo_StorageFormat pins how each storage_format value
// decodes. The measured encoding is the smallint; the decimal text of it and the
// model's own names decode the same way. Every other value — NULL included —
// is an error naming the association, because guessing would put the link in
// the wrong place, or nowhere.
func TestLookupAssociationInfo_StorageFormat(t *testing.T) {
	cases := []struct {
		name   string
		stored string // SQL literal written to storage_format
		want   string // decoded format; "" means an error is expected
	}{
		{"smallint 1, measured on 10.24 to 11.14", "1", "Column"},
		{"smallint 2, measured on 10.24 to 11.14", "2", "Table"},
		{"text 1", "'1'", "Column"},
		{"text 2", "'2'", "Table"},
		{"model name Column", "'Column'", "Column"},
		{"model name, other case", "'table'", "Table"},
		{"NULL", "NULL", ""},
		{"smallint 0", "0", ""},
		{"smallint 3", "3", ""},
		{"unknown name", "'Junction'", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target := mendixTarget(t, c.stored, "2")
			info, err := LookupAssociationInfo(context.Background(), target, colAssoc)
			if c.want == "" {
				if err == nil {
					t.Fatalf("storage_format %s decoded to %q, want an error", c.stored, info.StorageFormat)
				}
				if !strings.Contains(err.Error(), colAssoc) || !strings.Contains(err.Error(), "storage_format") {
					t.Errorf("error does not name the association and the column: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("storage_format %s: %v", c.stored, err)
			}
			if info.StorageFormat != c.want {
				t.Errorf("storage_format %s decoded to %q, want %q", c.stored, info.StorageFormat, c.want)
			}
		})
	}
}

// TestExecuteImport_RefusesUnrecognisedStorageFormat: a LINK whose storage is
// neither a column nor a junction table is skipped by both write paths, so the
// import must refuse it before the first row is written rather than insert the
// rows unlinked and report success. "1" is exactly what the pre-fix resolver
// handed over for a column-stored association.
func TestExecuteImport_RefusesUnrecognisedStorageFormat(t *testing.T) {
	for _, format := range []string{"1", "2", "", "Junction"} {
		t.Run("format "+format, func(t *testing.T) {
			source := sourceDB(t)
			target := mendixTarget(t, "1", "2")
			link := &AssocInfo{
				SourceColumn:    "dept",
				LookupAttr:      "Name",
				AssociationName: colAssoc,
				ChildEntity:     "MyFirstModule.Department",
				StorageFormat:   format,
				FKColumnName:    "myfirstmodule$employeecol_department",
				LookupCache:     map[string]int64{"Sales": 101},
			}
			_, err := importEmployees(t, source, target, "MyFirstModule.EmployeeCol", link)
			if err == nil || !strings.Contains(err.Error(), "storage format") {
				t.Fatalf("import with storage format %q: err = %v, want a storage format error", format, err)
			}
			if !strings.Contains(err.Error(), colAssoc) {
				t.Errorf("error does not name the association: %v", err)
			}
			var n int
			if err := target.DB.QueryRow(`SELECT COUNT(*) FROM "myfirstmodule$employeecol"`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%d rows written before the refusal, want 0", n)
			}
		})
	}
}
