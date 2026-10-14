// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// seedTwoHits gives the fixture catalog two more rows that contain the same
// word, so a search for it has a known number of hits.
func seedTwoHits(t *testing.T) func(catalog.CatalogDB) {
	return func(db catalog.CatalogDB) {
		t.Helper()
		for _, q := range []string{
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Sales.Limit', 'CONSTANT', 'Upper bound for the quuxwidget', 'caption', 'Sales')`,
			`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, ModuleName)
				VALUES ('Sales.Other', 'CONSTANT', 'Lower bound for the quuxwidget', 'caption', 'Sales')`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("seed: %v\n%s", err, q)
			}
		}
	}
}

// A caller that gates on "no match" needs the number of hits, not the output it
// just printed. It is the same count in every format.
//
// One fixture per search: a second search on the same context made the
// TempDir cleanup fail on Windows (catalog.db still in use), so each case
// builds its own.
func TestSearchWith_ReturnsTheNumberOfHits(t *testing.T) {
	for _, format := range []string{"table", "names", "json"} {
		for query, want := range map[string]int{"quuxwidget": 2, "zzqqxxnomatch": 0} {
			t.Run(format+"/"+query, func(t *testing.T) {
				ctx, out, _ := searchCacheFixtureSeeded(t, "source", nil, seedTwoHits(t))
				n, err := searchWith(ctx, query, format, SearchOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if n != want {
					t.Errorf("hits = %d, want %d\n%s", n, want, out.String())
				}
			})
		}
	}
}

// A query the index cannot run is not "nothing found". The SEARCH statement has
// always answered it with "No matches found."; a gate on the absence of hits
// asks for the error instead.
func TestSearchWith_StrictReportsAQueryTheIndexRejects(t *testing.T) {
	const bad = `quux"widget`

	for _, format := range []string{"table", "json"} {
		ctx, _, _ := searchCacheFixtureSeeded(t, "source", nil, seedTwoHits(t))
		if n, err := searchWith(ctx, bad, format, SearchOptions{}); err != nil || n != 0 {
			t.Fatalf("%s default: hits = %d, err = %v; want 0, nil (unchanged behaviour)", format, n, err)
		}

		ctx, _, _ = searchCacheFixtureSeeded(t, "source", nil, seedTwoHits(t))
		if _, err := searchWith(ctx, bad, format, SearchOptions{Strict: true}); err == nil {
			t.Fatalf("%s strict: want the index's error for a query it cannot run, got nil", format)
		}
	}
}
