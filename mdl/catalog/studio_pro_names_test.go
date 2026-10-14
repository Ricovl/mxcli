// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"slices"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1395: "mxcli describe --help and the catalog.db still use
// the old names" after ako/mxcli#755 renamed the document types to Studio Pro's
// (R10). The catalog must expose the canonical table names and ObjectType
// values; the old table names stay queryable as aliases, so a saved query or a
// user's lint rule written against them keeps answering.
func TestCatalog_StudioProDocumentNames(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	db := cat.CatalogDB()
	for _, stmt := range []string{
		`INSERT INTO rest_clients_data (Id, Name, QualifiedName, ModuleName) VALUES ('r1', 'Api', 'M.Api', 'M')`,
		`INSERT INTO rest_operations_data (Id, ServiceId, ServiceQualifiedName, Name) VALUES ('o1', 'r1', 'M.Api', 'GetPets')`,
		`INSERT INTO odata_clients_data (Id, Name, QualifiedName, ModuleName) VALUES ('c1', 'Crm', 'M.Crm', 'M')`,
		`INSERT INTO odata_services_data (Id, Name, QualifiedName, ModuleName) VALUES ('s1', 'Pub', 'M.Pub', 'M')`,
		`INSERT INTO queues_data (Id, Name, QualifiedName, ModuleName) VALUES ('q1', 'Mail', 'M.Mail', 'M')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	renamed := []struct{ canonical, old, name string }{
		{"consumed_rest_services", "rest_clients", "Api"},
		{"consumed_rest_operations", "rest_operations", "GetPets"},
		{"consumed_odata_services", "odata_clients", "Crm"},
		{"published_odata_services", "odata_services", "Pub"},
		{"task_queues", "queues", "Mail"},
	}
	tables := cat.Tables()
	for _, r := range renamed {
		if !slices.Contains(tables, "CATALOG."+strings.ToUpper(r.canonical)) {
			t.Errorf("Tables() does not list CATALOG.%s", strings.ToUpper(r.canonical))
		}
		if slices.Contains(tables, "CATALOG."+strings.ToUpper(r.old)) {
			t.Errorf("Tables() still lists the old name CATALOG.%s", strings.ToUpper(r.old))
		}
		for _, view := range []string{r.canonical, r.old} {
			res, err := cat.Query("SELECT Name FROM " + view)
			if err != nil {
				t.Errorf("SELECT FROM %s: %v", view, err)
				continue
			}
			if len(res.Rows) != 1 || res.Rows[0][0] != r.name {
				t.Errorf("SELECT Name FROM %s = %v, want [[%s]]", view, res.Rows, r.name)
			}
		}
	}

	res, err := cat.Query("SELECT QualifiedName, ObjectType FROM objects ORDER BY QualifiedName")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, row := range res.Rows {
		got[row[0].(string)] = row[1].(string)
	}
	want := map[string]string{
		"M.Api":  "CONSUMED_REST_SERVICE",
		"M.Crm":  "CONSUMED_ODATA_SERVICE",
		"M.Pub":  "PUBLISHED_ODATA_SERVICE",
		"M.Mail": "TASK_QUEUE",
	}
	for qn, w := range want {
		if got[qn] != w {
			t.Errorf("objects.ObjectType for %s = %q, want %q", qn, got[qn], w)
		}
	}
}
