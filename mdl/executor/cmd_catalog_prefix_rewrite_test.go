// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1450 (mendixlabs/mxcli#1553): the CATALOG. table prefix was
// stripped by a regex over the whole query, so it was also stripped inside a
// string literal. A module named Catalog could then never be filtered on:
// `WHERE QualifiedName = 'Catalog.Product'` compared against 'Product'.
//
// The rewrite is only correct if it touches what SQLite would read as a
// schema-qualified table name and nothing else, so the table below covers each
// lexical context SQLite has: string literals (with the doubled-quote escape),
// the three quoted-identifier spellings, both comment forms, and the controls
// that must keep working — upper/lower/mixed-case prefixes, joins, subqueries,
// a query with no prefix at all, and names that merely contain "catalog".
func TestConvertCatalogTableNames(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		// Controls: the prefix itself, in every case, is still removed.
		{"upper", `SELECT * FROM CATALOG.ENTITIES`, `SELECT * FROM ENTITIES`},
		{"lower", `select * from catalog.entities`, `select * from entities`},
		{"mixed", `SELECT * FROM Catalog.Entities`, `SELECT * FROM Entities`},
		{"join of two catalog tables",
			`SELECT e.Name, a.Name FROM CATALOG.ENTITIES e JOIN catalog.attributes a ON a.EntityQualifiedName = e.QualifiedName`,
			`SELECT e.Name, a.Name FROM ENTITIES e JOIN attributes a ON a.EntityQualifiedName = e.QualifiedName`},
		{"subquery",
			`SELECT Name FROM CATALOG.MICROFLOWS WHERE QualifiedName NOT IN (SELECT TargetName FROM CATALOG.REFS)`,
			`SELECT Name FROM MICROFLOWS WHERE QualifiedName NOT IN (SELECT TargetName FROM REFS)`},
		{"no prefix", `SELECT * FROM entities WHERE ModuleName = 'Sales'`, `SELECT * FROM entities WHERE ModuleName = 'Sales'`},
		{"table-qualified column", `SELECT CATALOG.ENTITIES.Name FROM CATALOG.ENTITIES`, `SELECT ENTITIES.Name FROM ENTITIES`},
		{"catalog as a plain alias", `SELECT 1 AS catalog FROM CATALOG.MODULES`, `SELECT 1 AS catalog FROM MODULES`},

		// The issue: the prefix inside a string literal is data, not a table.
		{"literal in select list (the issue)",
			`SELECT QualifiedName, 'Catalog.Product' AS lit FROM CATALOG.ENTITIES WHERE ModuleName = 'Catalog'`,
			`SELECT QualifiedName, 'Catalog.Product' AS lit FROM ENTITIES WHERE ModuleName = 'Catalog'`},
		{"literal in where (the issue)",
			`SELECT COUNT(*) AS n FROM CATALOG.ENTITIES WHERE QualifiedName = 'Catalog.Product'`,
			`SELECT COUNT(*) AS n FROM ENTITIES WHERE QualifiedName = 'Catalog.Product'`},
		{"lower-case prefix in literal",
			`SELECT * FROM catalog.entities WHERE Documentation LIKE '%see catalog.entities%'`,
			`SELECT * FROM entities WHERE Documentation LIKE '%see catalog.entities%'`},
		{"doubled quote does not end the literal",
			`SELECT * FROM CATALOG.ENTITIES WHERE Documentation = 'it''s Catalog.Product'`,
			`SELECT * FROM ENTITIES WHERE Documentation = 'it''s Catalog.Product'`},
		{"literal ending in a doubled quote, then a prefix",
			`SELECT 'x''' AS a, 'Catalog.Product' AS b FROM CATALOG.MODULES`,
			`SELECT 'x''' AS a, 'Catalog.Product' AS b FROM MODULES`},

		// Quoted identifiers are names the user spelled exactly.
		{"double-quoted identifier", `SELECT Name AS "Catalog.Name" FROM CATALOG.ENTITIES`, `SELECT Name AS "Catalog.Name" FROM ENTITIES`},
		{"double-quoted with doubled quote", `SELECT Name AS "a""Catalog.b" FROM CATALOG.ENTITIES`, `SELECT Name AS "a""Catalog.b" FROM ENTITIES`},
		{"backtick identifier", "SELECT Name AS `Catalog.Name` FROM CATALOG.ENTITIES", "SELECT Name AS `Catalog.Name` FROM ENTITIES"},
		{"bracket identifier", `SELECT Name AS [Catalog.Name] FROM CATALOG.ENTITIES`, `SELECT Name AS [Catalog.Name] FROM ENTITIES`},

		// Comments are kept verbatim.
		{"line comment", "SELECT * FROM CATALOG.ENTITIES -- not Catalog.Product\nWHERE 1 = 1", "SELECT * FROM ENTITIES -- not Catalog.Product\nWHERE 1 = 1"},
		{"block comment", `SELECT /* Catalog.Product */ * FROM CATALOG.ENTITIES`, `SELECT /* Catalog.Product */ * FROM ENTITIES`},

		// Names that merely contain "catalog" are not the prefix.
		{"module name containing catalog, in literals",
			`SELECT * FROM CATALOG.ENTITIES WHERE QualifiedName IN ('ProductCatalog.Item', 'MyCatalog.Thing', 'my_catalog.x')`,
			`SELECT * FROM ENTITIES WHERE QualifiedName IN ('ProductCatalog.Item', 'MyCatalog.Thing', 'my_catalog.x')`},
		{"alias containing catalog", `SELECT ProductCatalog.Name FROM CATALOG.ENTITIES AS ProductCatalog`, `SELECT ProductCatalog.Name FROM ENTITIES AS ProductCatalog`},
		{"underscore alias containing catalog", `SELECT my_catalog.Name FROM CATALOG.ENTITIES my_catalog`, `SELECT my_catalog.Name FROM ENTITIES my_catalog`},

		// The grammar accepts whitespace around the dot (CATALOG DOT name are
		// three tokens); SQLite reads `CATALOG . ENTITIES` as a schema, so it
		// has to be rewritten the same way.
		{"whitespace around the dot", "SELECT * FROM CATALOG . ENTITIES e JOIN CATALOG\n.ATTRIBUTES a ON 1 = 1", "SELECT * FROM ENTITIES e JOIN ATTRIBUTES a ON 1 = 1"},

		// An unterminated literal swallows the rest, as it does for SQLite,
		// which then reports it; the rewrite must not invent a table there.
		{"unterminated literal", `SELECT 'Catalog.x FROM CATALOG.ENTITIES`, `SELECT 'Catalog.x FROM CATALOG.ENTITIES`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := convertCatalogTableNames(tc.in); got != tc.want {
				t.Errorf("convertCatalogTableNames(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// catalogModuleFixture is an in-memory catalog holding a module literally
// named Catalog beside modules whose names only contain the word, which is the
// shape the issue needs and the controls need.
func catalogModuleFixture(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	for _, m := range []string{"Catalog", "ProductCatalog", "MyCatalog"} {
		if _, err := db.Exec(`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)`, "m-"+m, m, m, m); err != nil {
			t.Fatalf("seed module: %v", err)
		}
	}
	for _, e := range [][2]string{{"Catalog", "Product"}, {"ProductCatalog", "Item"}, {"MyCatalog", "Thing"}} {
		qn := e[0] + "." + e[1]
		if _, err := db.Exec(`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, EntityType) VALUES (?, ?, ?, ?, 'PERSISTENT')`, "e-"+qn, e[1], qn, e[0]); err != nil {
			t.Fatalf("seed entity: %v", err)
		}
	}
	cat.SetBuilt(true)
	return cat
}

// runCatalogSelect parses src as MDL, exactly as -c, a script and the REPL
// do, and runs the SELECT it yields.
func runCatalogSelect(t *testing.T, ctx *ExecContext, src string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("parse %q: got %d statements, want 1", src, len(prog.Statements))
	}
	sel, ok := prog.Statements[0].(*ast.SelectStmt)
	if !ok {
		t.Fatalf("parse %q: got %T, want *ast.SelectStmt", src, prog.Statements[0])
	}
	if err := execCatalogQuery(ctx, sel.Query); err != nil {
		t.Fatalf("execCatalogQuery(%q): %v", sel.Query, err)
	}
}

// TestCatalogQueryOnModuleNamedCatalog runs the issue's two queries end to end
// (MDL parse → execCatalogQuery → SQLite → JSON) plus the controls.
func TestCatalogQueryOnModuleNamedCatalog(t *testing.T) {
	cat := catalogModuleFixture(t)
	cases := []struct {
		name, src string
		want      []map[string]string
	}{
		{"issue: literal in select list",
			`SELECT QualifiedName, 'Catalog.Product' AS lit FROM CATALOG.ENTITIES WHERE ModuleName = 'Catalog'`,
			[]map[string]string{{"QualifiedName": "Catalog.Product", "lit": "Catalog.Product"}}},
		{"issue: filter on qualified name",
			`SELECT COUNT(*) AS n FROM CATALOG.ENTITIES WHERE QualifiedName = 'Catalog.Product'`,
			[]map[string]string{{"n": "1"}}},
		{"lower-case prefix, filter on qualified name",
			`select count(*) as n from catalog.entities where QualifiedName = 'Catalog.Product'`,
			[]map[string]string{{"n": "1"}}},
		{"module names containing catalog",
			`SELECT QualifiedName FROM CATALOG.ENTITIES WHERE QualifiedName IN ('ProductCatalog.Item', 'MyCatalog.Thing') ORDER BY QualifiedName`,
			[]map[string]string{{"QualifiedName": "MyCatalog.Thing"}, {"QualifiedName": "ProductCatalog.Item"}}},
		{"join of two catalog tables",
			`SELECT e.QualifiedName AS q FROM CATALOG.ENTITIES e JOIN CATALOG.MODULES m ON m.Name = e.ModuleName WHERE m.Name = 'Catalog'`,
			[]map[string]string{{"q": "Catalog.Product"}}},
		{"whitespace around the dot",
			`SELECT COUNT(*) AS n FROM CATALOG . ENTITIES WHERE ModuleName = 'Catalog'`,
			[]map[string]string{{"n": "1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diag bytes.Buffer
			ctx, out := newMockCtx(t, withFormat(FormatJSON))
			ctx.Catalog = cat
			ctx.Diagnostics = &diag
			runCatalogSelect(t, ctx, tc.src)
			var got []map[string]string
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("output is not a JSON array of rows: %v\n%s", err, out.String())
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("%s\n got: %s\nwant: %s", tc.src, gotJSON, wantJSON)
			}
		})
	}
}

// TestCatalogQueryModeWarningReadsTheTables: the mode warning found its table
// by taking the word after the first "from " in the query text, so a literal
// containing "from" hid the real table, and only the first table of a join was
// ever checked. CATALOG.REFS is filled only by `refresh catalog full`; on this
// fast catalog a query against it must say so however it is spelled.
func TestCatalogQueryModeWarningReadsTheTables(t *testing.T) {
	cat := catalogModuleFixture(t)
	const warning = "Warning: CATALOG.REFS requires refresh catalog full"
	cases := []struct {
		name, src string
		warn      bool
	}{
		{"control: plain query", `SELECT COUNT(*) AS n FROM CATALOG.REFS`, true},
		{"control: fast table does not warn", `SELECT COUNT(*) AS n FROM CATALOG.ENTITIES`, false},
		{"literal containing from", `SELECT 'moved from Sales' AS note FROM CATALOG.REFS`, true},
		{"gated table second in a join", `SELECT COUNT(*) AS n FROM CATALOG.ENTITIES e JOIN CATALOG.REFS r ON r.TargetName = e.QualifiedName`, true},
		{"literal naming the table does not warn", `SELECT 'from refs' AS note FROM CATALOG.ENTITIES`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, out := newMockCtx(t)
			ctx.Catalog = cat
			runCatalogSelect(t, ctx, tc.src)
			if got := strings.Contains(out.String(), warning); got != tc.warn {
				t.Errorf("%s\nwarned = %v, want %v; output:\n%s", tc.src, got, tc.warn, out.String())
			}
		})
	}
}
