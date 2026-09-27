// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// ako/mxcli#749: `alter page … on dg column(Name)` addresses a DataGrid 2 column
// explicitly, by the attribute or caption describe prints in it. Mendix stores
// no column name, so two columns over the same attribute share the address, and
// an address matching more than one is refused unless @n picks one.

func sel(s backend.ColumnSelector) string { return s.String() }

// The shape of Administration.Account_Overview in the PedApp fixture (Studio
// Pro-authored): two columns over FullName, one captioned ' '.
func accountOverviewGrid() bson.D {
	return buildGridWithColumns([]map[string]string{
		{"attr": "Administration.Account.FullName", "caption": "Full name"},
		{"attr": "System.User.Name", "caption": "Login"},
		{"caption": "Total"},
		{"attr": "Administration.Account.FullName", "caption": " "},
	})
}

func TestFindBsonColumn_SelectorByAttribute(t *testing.T) {
	res, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "Name"}), gridFinder(accountOverviewGrid()))
	if err != nil {
		t.Fatalf("column(Name): %v", err)
	}
	if res.index != 1 {
		t.Errorf("column(Name) resolved to column %d, want 1", res.index)
	}
}

func TestFindBsonColumn_SelectorByCaption(t *testing.T) {
	res, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Caption: "Total"}), gridFinder(accountOverviewGrid()))
	if err != nil {
		t.Fatalf("column('Total'): %v", err)
	}
	if res.index != 2 {
		t.Errorf("column('Total') resolved to column %d, want 2", res.index)
	}
	// A caption is matched as written, not sanitized: 'Full name' is the first
	// column, and `Full_name` — what the derived name would be — is not a caption.
	if _, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Caption: "Full_name"}), gridFinder(accountOverviewGrid())); err == nil {
		t.Error("column('Full_name') matched; a caption address must match the caption exactly")
	}
}

// The duplicate the issue names: two columns both describe as Attribute: FullName.
func TestFindBsonColumn_SelectorRefusesDuplicates(t *testing.T) {
	res, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "FullName"}), gridFinder(accountOverviewGrid()))
	if res != nil {
		t.Fatal("an ambiguous column(FullName) resolved to a column")
	}
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	for _, want := range []string{"ambiguous", "dg column(FullName)@1", "dg column(FullName)@2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ambiguity error should contain %q, got: %v", want, err)
		}
	}
}

func TestFindBsonColumn_SelectorOrdinalPicks(t *testing.T) {
	res, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "FullName", Ordinal: 2}), gridFinder(accountOverviewGrid()))
	if err != nil {
		t.Fatalf("column(FullName)@2: %v", err)
	}
	if res.index != 3 {
		t.Errorf("column(FullName)@2 resolved to column %d, want 3", res.index)
	}
	if _, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "FullName", Ordinal: 3}), gridFinder(accountOverviewGrid())); err == nil ||
		!strings.Contains(err.Error(), "only 2") {
		t.Errorf("column(FullName)@3 should say there are only 2 matches, got %v", err)
	}
}

func TestFindBsonColumn_SelectorMissListsAddresses(t *testing.T) {
	_, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "Email"}), gridFinder(accountOverviewGrid()))
	if err == nil {
		t.Fatal("column(Email) resolved on a grid with no Email column")
	}
	for _, want := range []string{"not found", "column(FullName)", "column(Name)", "column('Total')"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("not-found error should list %q, got: %v", want, err)
		}
	}
}

// An attribute over an association is stored as an AttributeRef document whose
// EntityRef holds the steps; describe writes it `UserRoles/Name`, and so must
// the address.
func TestFindBsonColumn_SelectorByAssociationPath(t *testing.T) {
	grid := buildGridWithColumns([]map[string]string{{"attr": "System.User.Name"}})
	// Replace the plain AttributeRef string with the stored document shape.
	cols := bsonDGetColumns(t, grid)
	colDoc := cols[1].(bson.D)
	props := colDoc[0].Value.(bson.A)
	prop := props[1].(bson.D)
	prop[1].Value = bson.D{{Key: "AttributeRef", Value: bson.D{
		{Key: "Attribute", Value: "System.UserRole.Name"},
		{Key: "EntityRef", Value: bson.D{
			{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
			{Key: "Steps", Value: bson.A{int32(2), bson.D{{Key: "Association", Value: "System.UserRoles"}}}},
		}},
	}}}
	if _, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "UserRoles/Name"}), gridFinder(grid)); err != nil {
		t.Fatalf("column(UserRoles/Name): %v", err)
	}
	if _, err := findBsonColumn(bson.D{}, "dg", sel(backend.ColumnSelector{Attribute: "Name"}), gridFinder(grid)); err == nil {
		t.Error("column(Name) matched an attribute over an association; describe writes it UserRoles/Name")
	}
}

// The generic resolver takes the selector as the second path element and must
// answer with the address as written.
func TestResolveAlterTarget_ColumnSelector(t *testing.T) {
	m := &Mutator{rawData: bson.D{}, widgetFinder: gridFinder(accountOverviewGrid())}
	got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"dg", sel(backend.ColumnSelector{Attribute: "Name"})}})
	if err != nil {
		t.Fatalf("resolve dg column(Name): %v", err)
	}
	if got.Kind != "column" || got.Name != "dg column(Name)" {
		t.Errorf("resolved to %+v", got)
	}
	if _, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"dg", sel(backend.ColumnSelector{Attribute: "FullName"})}}); err == nil {
		t.Error("resolve dg column(FullName) accepted an ambiguous address")
	}
}

func bsonDGetColumns(t *testing.T, grid bson.D) bson.A {
	t.Helper()
	for _, e := range grid {
		if e.Key != "Object" {
			continue
		}
		props := e.Value.(bson.D)[0].Value.(bson.A)
		return props[1].(bson.D)[1].Value.(bson.D)[0].Value.(bson.A)
	}
	t.Fatal("grid has no Object")
	return nil
}
