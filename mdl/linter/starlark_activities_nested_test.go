// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// activities_for(name) stays top-level only, so a rule written before the
// catalog had loop bodies keeps its counts (CONV010 would otherwise gain
// findings); activities_for(name, nested = True) adds the loop bodies with
// parent_loop_id and loop_depth (mendixlabs/mxcli#1266). The catalog is the
// real schema, so the query is checked against the columns the builder writes.
func TestActivitiesForNested(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ActivityCount, TotalActivityCount)
		 VALUES ('mf', 'MF_Loop', 'T.MF_Loop', 'T', 'MICROFLOW', 1, 3)`,
		`INSERT INTO activities_data (Id, ActivityType, ActionType, Sequence, MicroflowId, MicroflowQualifiedName,
			ModuleName, ParentLoopId, LoopDepth, RetrieveSource, ErrorHandlingType, Caption)
		 VALUES
		 ('loop',  'LoopedActivity', '',               1, 'mf', 'T.MF_Loop', 'T', '',      0, '',         'Rollback', ''),
		 ('ret',   'ActionActivity', 'RetrieveAction', 2, 'mf', 'T.MF_Loop', 'T', 'loop',  1, 'database', 'Rollback', 'Get orders'),
		 ('inner', 'LoopedActivity', '',               3, 'mf', 'T.MF_Loop', 'T', 'loop',  1, '',         'Rollback', ''),
		 ('del',   'ActionActivity', 'DeleteObjectAction', 4, 'mf', 'T.MF_Loop', 'T', 'inner', 2, '',     'Continue', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	rule := `RULE_ID = "TEST01"
RULE_NAME = "Nested"
DESCRIPTION = "probe"
CATEGORY = "quality"
SEVERITY = "info"
REQUIRES = ["full"]

def check():
    out = []
    for mf in microflows():
        for nested in [False, True]:
            for a in activities_for(mf.qualified_name, nested = nested):
                out.append(violation(message = "%s|%s|%s|%d|%s|%s|%s" % (
                    nested, a.id, a.parent_loop_id, a.loop_depth, a.retrieve_source,
                    a.error_handling_type, a.caption)))
    return out
`
	path := filepath.Join(t.TempDir(), "nested.star")
	if err := os.WriteFile(path, []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	ctx := linter.NewLintContextFromDB(db)
	var got []string
	for _, v := range r.Check(ctx) {
		got = append(got, v.Message)
	}
	if errs := ctx.QueryErrors(); len(errs) > 0 {
		t.Fatalf("query errors: %v -- the query names a column the catalog does not have", errs)
	}
	sort.Strings(got)
	want := []string{
		"False|loop||0||Rollback|",
		"True|del|inner|2||Continue|",
		"True|inner|loop|1||Rollback|",
		"True|loop||0||Rollback|",
		"True|ret|loop|1|database|Rollback|Get orders",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("activities_for rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
