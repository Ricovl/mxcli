// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// Measured on Evora Factory Management, `impact DigitalTwin.Machine` printed
// FactoryManagement.ProductionLine_Reset | retrieve twice (the microflow has two
// retrieve activities, one edge each) and a summary of "MICROFLOW: 9" over six
// distinct microflows, in an order that changed between runs. `refs` repeated
// the same row. The fixture reproduces each shape.

func seedRefsCatalog(t *testing.T) *ExecContext {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	db := cat.CatalogDB()
	seed := []string{
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ProjectId, SnapshotId) VALUES
			('MICROFLOW', '', 'Shop.ACT_Reset', 'ENTITY', '', 'Shop.Machine', 'retrieve', 'p', 's'),
			('MICROFLOW', '', 'Shop.ACT_Reset', 'ENTITY', '', 'Shop.Machine', 'retrieve', 'p', 's'),
			('MICROFLOW', '', 'Shop.ACT_Reset', 'ENTITY', '', 'Shop.Machine', 'delete', 'p', 's'),
			('MICROFLOW', '', 'Shop.ACT_Count', 'ENTITY', '', 'Shop.Machine', 'retrieve', 'p', 's'),
			('PAGE', '', 'Shop.Machine_Details', 'ENTITY', '', 'Shop.Machine', 'datasource', 'p', 's'),
			('PAGE', '', 'Shop.Machine_Details', 'ENTITY', '', 'Shop.Machine', 'parameter', 'p', 's'),
			('ASSOCIATION', '', 'Shop.Machine_Line', 'ENTITY', '', 'Shop.Machine', 'associate', 'p', 's'),
			('ENTITY', '', 'Shop.Machine', 'ENUMERATION', '', 'Shop.Status', 'type', 'p', 's'),
			('PAGE', '', 'Shop.Machine_Details', 'ENUMERATION_VALUE', '', 'Shop.Status.Critical', 'value', 'p', 's'),
			('MICROFLOW', '', 'Shop.ACT_Reset', 'ENUMERATION_VALUE', '', 'Shop.Status.Critical', 'value', 'p', 's')`,
		`INSERT INTO attributes_data (Id, Name, EntityId, EntityQualifiedName, ModuleName, DataType) VALUES
			('a1', 'Unused', 'e1', 'Shop.Machine', 'Shop', 'String')`,
		`INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName) VALUES
			('en1', 'Status', 'Shop.Status', 'Shop')`,
		`INSERT INTO enumeration_values_data (Id, EnumerationId, EnumerationQualifiedName, ModuleName, Name) VALUES
			('v1', 'en1', 'Shop.Status', 'Shop', 'Critical'),
			('v2', 'en1', 'Shop.Status', 'Shop', 'Retired')`,
	}
	for _, s := range seed {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	ctx, _ := newMockCtx(t)
	ctx.Catalog = cat
	return ctx
}

func runRefsCmd(t *testing.T, fn func(*ExecContext, string) error, target string) string {
	t.Helper()
	ctx := seedRefsCatalog(t)
	if err := fn(ctx, target); err != nil {
		t.Fatalf("run: %v", err)
	}
	return ctx.Output.(interface{ String() string }).String()
}

func rowsMentioning(out, s string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, s) {
			n++
		}
	}
	return n
}

func TestShowReferencesListsEachEdgeOnce(t *testing.T) {
	out := runRefsCmd(t, showReferences, "Shop.Machine")
	// ACT_Reset: retrieve + delete — two edges, not three rows.
	if n := rowsMentioning(out, "Shop.ACT_Reset"); n != 2 {
		t.Errorf("Shop.ACT_Reset appears in %d rows, want 2 (retrieve, delete):\n%s", n, out)
	}
	if !strings.Contains(out, "Found 6 reference(s)") {
		t.Errorf("want 6 distinct references:\n%s", out)
	}
}

func TestShowImpactCountsElementsNotRows(t *testing.T) {
	out := runRefsCmd(t, showImpact, "Shop.Machine")
	for _, want := range []string{
		"  ASSOCIATION: 1\n  MICROFLOW: 2\n  PAGE: 1\n", // distinct elements, types in a fixed order
		"Found 4 affected element(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("impact output missing %q:\n%s", want, out)
		}
	}
	if n := rowsMentioning(out, "Shop.ACT_Reset"); n != 2 {
		t.Errorf("Shop.ACT_Reset appears in %d rows, want 2:\n%s", n, out)
	}
}

// The summary was built by ranging over a map, so two runs of the same
// command could print the types in different orders.
func TestShowImpactIsDeterministic(t *testing.T) {
	first := runRefsCmd(t, showImpact, "Shop.Machine")
	for i := 0; i < 20; i++ {
		if again := runRefsCmd(t, showImpact, "Shop.Machine"); again != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first, again)
		}
	}
}

// An enumeration is used through its values as much as through its type; an
// impact that listed only the attribute typed as it would miss the page and
// microflow that break when a value is removed.
func TestShowImpactOfEnumerationIncludesItsValues(t *testing.T) {
	out := runRefsCmd(t, showImpact, "Shop.Status")
	for _, want := range []string{"Shop.Machine", "Shop.Machine_Details", "Shop.ACT_Reset", "Shop.Status.Critical"} {
		if !strings.Contains(out, want) {
			t.Errorf("enumeration impact missing %q:\n%s", want, out)
		}
	}
}

// With no edge, the catalog cannot say an attribute is unused — only that none
// of the sites it resolves uses it. Saying "not referenced" is what makes an
// agent delete a live attribute.
func TestShowImpactOfUnreferencedAttributeSaysWhatWasChecked(t *testing.T) {
	out := runRefsCmd(t, showImpact, "Shop.Machine.Unused")
	if strings.Contains(out, "is not referenced") {
		t.Errorf("claims the attribute is not referenced:\n%s", out)
	}
	for _, want := range []string{"expression", "search 'Unused'"} {
		if !strings.Contains(out, want) {
			t.Errorf("no-reference message missing %q:\n%s", want, out)
		}
	}

	refs := runRefsCmd(t, showReferences, "Shop.Status.Retired")
	if !strings.Contains(refs, "search 'Retired'") {
		t.Errorf("enumeration value no-reference message should name what was not checked:\n%s", refs)
	}
}
