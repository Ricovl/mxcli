// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// `context` on an entity with associations said "Related Entities: (none
// found)". The section read refs rows whose SOURCE is an entity, but an
// association edge's source is the ASSOCIATION, so it only ever found
// generalizations. Evora: DigitalTwin.Machine, five associations.
func TestAssembleEntityContextListsAssociatedEntities(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	for _, s := range []string{
		`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, EntityType, Generalization) VALUES
			('e1', 'Machine', 'Shop.Machine', 'Shop', 'PERSISTENT', ''),
			('e2', 'Line', 'Shop.Line', 'Shop', 'PERSISTENT', ''),
			('e3', 'Incident', 'Shop.Incident', 'Shop', 'PERSISTENT', ''),
			('e4', 'Robot', 'Shop.Robot', 'Shop', 'PERSISTENT', 'Shop.Machine')`,
		`INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName, FromEntity, ToEntity) VALUES
			('as1', 'Machine_Line', 'Shop.Machine_Line', 'Shop', 'Shop.Machine', 'Shop.Line'),
			('as2', 'Incident_Machine', 'Shop.Incident_Machine', 'Shop', 'Shop.Incident', 'Shop.Machine')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	ctx, _ := newMockCtx(t)
	ctx.Catalog = cat

	var out strings.Builder
	assembleEntityContext(ctx, &out, "Shop.Machine", 2)
	got := out.String()
	related := got[strings.Index(got, "### Related Entities"):]
	if strings.Contains(related, "(none found)") {
		t.Fatalf("entity with two associations reports no related entities:\n%s", related)
	}
	for _, want := range []string{"Shop.Line", "Shop.Machine_Line", "Shop.Incident", "Shop.Incident_Machine", "Shop.Robot"} {
		if !strings.Contains(related, want) {
			t.Errorf("Related Entities missing %q:\n%s", want, related)
		}
	}
}

// `context` on a microflow listed a caller once per call activity, so a caller
// with three calls to it read as three callers.
func TestAssembleMicroflowContextListsEachCallerOnce(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	defer cat.Close()
	if _, err := cat.CatalogDB().Exec(`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ProjectId, SnapshotId) VALUES
		('MICROFLOW', '', 'Shop.Caller', 'MICROFLOW', '', 'Shop.Sub', 'call', 'p', 's'),
		('MICROFLOW', '', 'Shop.Caller', 'MICROFLOW', '', 'Shop.Sub', 'call', 'p', 's'),
		('MICROFLOW', '', 'Shop.Caller', 'MICROFLOW', '', 'Shop.Sub', 'call', 'p', 's')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx, _ := newMockCtx(t)
	ctx.Catalog = cat

	var out strings.Builder
	assembleMicroflowContext(ctx, &out, "Shop.Sub", 1)
	callers := out.String()[strings.Index(out.String(), "### Direct Callers"):]
	if n := strings.Count(callers, "- Shop.Caller"); n != 1 {
		t.Errorf("Shop.Caller listed %d times under Direct Callers, want 1:\n%s", n, callers)
	}
}

// The enumeration context read ENTITY and MICROFLOW sources of edges to the
// enumeration's own name, which never existed; its values were not looked at.
func TestAssembleEnumerationContextIncludesValueUses(t *testing.T) {
	ctx := seedRefsCatalog(t)
	var out strings.Builder
	assembleEnumerationContext(ctx, &out, "Shop.Status")
	got := out.String()
	for _, want := range []string{"- Shop.Machine\n", "- Shop.ACT_Reset\n", "- Shop.Machine_Details\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("enumeration context missing %q:\n%s", want, got)
		}
	}
}
