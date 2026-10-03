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

// total_activity_count is the catalog's TotalActivityCount (loop bodies
// included, mendixlabs/mxcli#1266) on the struct microflows() yields for every
// flow flavour (ako/mxcli#963). A flow with a loop has total > activity_count;
// the control without a loop has them equal.
func TestStarlarkTotalActivityCount(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	if _, err := db.Exec(`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ActivityCount, TotalActivityCount)
		VALUES ('mf', 'MF_Loop', 'T.MF_Loop', 'T', 'MICROFLOW', 2, 5),
		       ('nf', 'NF_Loop', 'T.NF_Loop', 'T', 'NANOFLOW', 1, 3),
		       ('ru', 'RU_Loop', 'T.RU_Loop', 'T', 'RULE', 1, 2),
		       ('fl', 'MF_Flat', 'T.MF_Flat', 'T', 'MICROFLOW', 4, 4)`); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	rule := `RULE_ID = "TEST01"
RULE_NAME = "Total"
DESCRIPTION = "probe"
CATEGORY = "quality"
SEVERITY = "info"

def check():
    return [violation(message = "%s|%d|%d" % (mf.name, mf.activity_count, mf.total_activity_count))
            for mf in microflows()]
`
	path := filepath.Join(t.TempDir(), "total.star")
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
		t.Fatalf("query errors: %v", errs)
	}
	sort.Strings(got)
	want := []string{"MF_Flat|4|4", "MF_Loop|2|5", "NF_Loop|1|3", "RU_Loop|1|2"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
