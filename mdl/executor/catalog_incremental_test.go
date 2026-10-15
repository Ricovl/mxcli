// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestIncrementalCatalogMatchesFullRebuild renames a microflow in its unit file
// only (as a git pull would), refreshes the catalog incrementally, and requires
// every table to equal a full rebuild's.
func TestIncrementalCatalogMatchesFullRebuild(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	units, _ := filepath.Glob(filepath.Join(filepath.Dir(ctx.MprPath), "mprcontents", "*", "*", "*.mxunit"))
	old := time.Now().Add(-time.Hour)
	for _, u := range units {
		_ = os.Chtimes(u, old, old) // not racily clean
	}
	run(t, exec, "REFRESH CATALOG FULL FORCE")
	cachePath := getCachePath(ctx)

	// A microflow with references, so stale rows would show.
	db, err := sql.Open("sqlite", cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	err = db.QueryRow(`SELECT SourceId FROM refs WHERE SourceType = 'MICROFLOW' AND SourceId <> '' LIMIT 1`).Scan(&id)
	db.Close()
	if err != nil {
		t.Fatalf("no microflow with references in the fixture: %v", err)
	}
	unit := filepath.Join(filepath.Dir(ctx.MprPath), "mprcontents", id[0:2], id[2:4], id+".mxunit")
	b, err := os.ReadFile(unit)
	if err != nil {
		t.Fatal(err)
	}
	// Same-length rename: swap the name's last letter.
	i := bytes.Index(b, []byte("\x02Name\x00"))
	if i < 0 {
		t.Fatal("unit has no Name")
	}
	b[i+10+int(b[i+6])-2] = 'Q'
	if err := os.WriteFile(unit, b, 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(-30 * time.Minute)
	_ = os.Chtimes(unit, newer, newer)

	if !refreshCatalogIncrementally(ctx, cachePath, "full") {
		t.Fatal("refresh after a microflow-only change was not incremental")
	}
	ctx.Catalog.Close()
	incremental := dumpCatalog(t, cachePath)

	run(t, exec, "REFRESH CATALOG FULL FORCE")
	full := dumpCatalog(t, cachePath)
	for table, rows := range full {
		if got := incremental[table]; strings.Join(got, "\n") != strings.Join(rows, "\n") {
			t.Errorf("table %s: incremental has %d rows, full rebuild %d", table, len(got), len(rows))
		}
	}
}

// dumpCatalog returns every table's rows, sorted, without the columns a rebuild
// renumbers or restamps.
func dumpCatalog(t *testing.T, path string) map[string][]string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables []string
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		if strings.HasPrefix(n, "sqlite_") || strings.HasPrefix(n, "strings_") || strings.HasPrefix(n, "source_") ||
			n == "catalog_meta" || n == "snapshots" || n == "projects" {
			continue
		}
		tables = append(tables, n)
	}
	rows.Close()
	skip := map[string]bool{"SnapshotId": true, "ProjectId": true, "SnapshotDate": true, "MtimeNs": true, "Digest": true}
	out := map[string][]string{}
	for _, table := range tables {
		cols, _ := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
		var keep []string
		for cols.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt any
			_ = cols.Scan(&cid, &name, &typ, &notnull, &dflt, &pk)
			if skip[name] || (name == "Id" && (table == "refs" || table == "permissions")) {
				continue
			}
			keep = append(keep, fmt.Sprintf("%q", name))
		}
		cols.Close()
		r, err := db.Query(fmt.Sprintf(`SELECT %s FROM %q`, strings.Join(keep, ","), table))
		if err != nil {
			t.Fatalf("dump %s: %v", table, err)
		}
		vals := make([]any, len(keep))
		ptrs := make([]any, len(keep))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for r.Next() {
			_ = r.Scan(ptrs...)
			out[table] = append(out[table], fmt.Sprint(vals...))
		}
		r.Close()
		sort.Strings(out[table])
	}
	return out
}

// TestIncrementalCatalog_AddedAndRemovedFlows adds a microflow in a new folder
// and drops another, straight after a full build (so the build's stamps are
// racily clean), and requires the incremental refresh to equal a full rebuild.
func TestIncrementalCatalog_AddedAndRemovedFlows(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	run(t, exec, `create microflow Administration.IncDoomed () folder 'IncOld' begin log info node 'Inc' 'doomed'; end;`)
	run(t, exec, "REFRESH CATALOG FULL FORCE")
	cachePath := getCachePath(ctx)

	run(t, exec, `create microflow Administration.IncAdded () folder 'IncFolder/Sub' begin log info node 'Inc' 'added'; end;`)
	run(t, exec, `drop microflow Administration.IncDoomed;`)

	if !refreshCatalogIncrementally(ctx, cachePath, "full") {
		t.Fatal("refresh after adding and dropping a microflow was not incremental")
	}
	ctx.Catalog.Close()
	incremental := dumpCatalog(t, cachePath)
	if !strings.Contains(strings.Join(incremental["microflows_data"], "\n"), "IncAdded") {
		t.Fatal("the added microflow is not in the refreshed catalog")
	}

	run(t, exec, "REFRESH CATALOG FULL FORCE")
	full := dumpCatalog(t, cachePath)
	for table, rows := range full {
		if got := incremental[table]; strings.Join(got, "\n") != strings.Join(rows, "\n") {
			t.Errorf("table %s: incremental has %d rows, full rebuild %d", table, len(got), len(rows))
		}
	}
}

// TestIncrementalCatalog_ChangedDomainModel adds an entity with documentation,
// an association and an access rule, and a flow that uses it, and requires
// the incremental refresh to equal a full rebuild.
func TestIncrementalCatalog_ChangedDomainModel(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	// A documented entity already in the module, so its rows exist to go stale.
	run(t, exec, `/** Base entity. */
create persistent entity Administration.IncBase (Name: String(50));`)
	run(t, exec, "REFRESH CATALOG FULL FORCE")
	cachePath := getCachePath(ctx)

	run(t, exec, `/** Connection settings. */
create persistent entity Administration.IncSettings (
  EndpointUrl: String(200),
  ApiKey: String(unlimited)
);`)
	run(t, exec, `create association Administration.IncSettings_Account from Administration.IncSettings to Administration.Account;`)
	run(t, exec, `grant read (EndpointUrl, ApiKey), write (EndpointUrl, ApiKey) on entity Administration.IncSettings to Administration.Administrator;`)
	run(t, exec, `create microflow Administration.IncSettings_Get () returns Administration.IncSettings folder 'IncSettings' begin
  retrieve $S from Administration.IncSettings first;
  return $S;
end;`)

	if !refreshCatalogIncrementally(ctx, cachePath, "full") {
		t.Fatal("refresh after changing a domain model was not incremental")
	}
	ctx.Catalog.Close()
	incremental := dumpCatalog(t, cachePath)
	if !strings.Contains(strings.Join(incremental["entities_data"], "\n"), "IncSettings") {
		t.Fatal("the added entity is not in the refreshed catalog")
	}

	run(t, exec, "REFRESH CATALOG FULL FORCE")
	full := dumpCatalog(t, cachePath)
	for table, rows := range full {
		if got := incremental[table]; strings.Join(got, "\n") != strings.Join(rows, "\n") {
			t.Errorf("table %s: incremental has %d rows, full rebuild %d", table, len(got), len(rows))
		}
	}
}
