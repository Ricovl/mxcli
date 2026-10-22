// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A member position inside a widget that reference checking walked past: an
// XPath constraint's members (mendixlabs/mxcli#1049). It passed check, passed
// exec, and failed the build with CE1613.

func pageWith(widgets ...*ast.WidgetV3) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.CreatePageStmtV3{
			Name:    ast.QualifiedName{Module: "Shop", Name: "P"},
			Widgets: widgets,
		},
	}}
}

// ---------------------------------------------------------------------------
// #1049 — XPath constraint members
// ---------------------------------------------------------------------------

func dbGrid(entityQN, where string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "datagrid", Name: "g",
		Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{
				Type: "database", Reference: entityQN, Where: where,
			},
		},
	}
}

func TestValidateXPathMembers_ReportsAMemberThatIsNeither(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Order", "[Shop.Order_Status = 'Open']")))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	for _, want := range []string{"Shop.Order_Status", "Shop.Order", "CE1613"} {
		if !strings.Contains(errs[0].Error(), want) {
			t.Errorf("error should mention %q: %v", want, errs[0])
		}
	}
}

func TestValidateXPathMembers_ReportsABareAttributeThatIsMissing(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Order", "[Nonsense = 'x']")))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Nonsense") {
		t.Fatalf("a bare attribute that does not exist went unreported: %v", errs)
	}
}

// CONTROL: every constraint that resolves must stay silent. Each of these is a
// shape a real page uses.
func TestValidateXPathMembers_AcceptsWhatResolves(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, entity, where string }{
		{"an attribute the entity declares", "Shop.Order", "[Status = 'Open']"},
		{"an attribute INHERITED from the generalization", "Shop.Order", "[Code = 'x']"},
		{"an association hop, then the target's attribute",
			"Shop.Order", "[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
		{"no constraint at all", "Shop.Order", ""},
		// ako/mxcli#874: stored as `Status`, so it is checked as `Status`.
		// Checked as written it was a step that resolves to nothing, and the
		// re-run of a script that had just created the page was refused.
		{"an attribute qualified with the entity", "Shop.Order", "[Shop.Order.Status = 'Open']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid(tc.entity, tc.where))); len(errs) != 0 {
				t.Errorf("reported a constraint that resolves: %v", errs)
			}
		})
	}
}

// THE CONTROL that a real false positive produced. An entity the project does
// not have makes the whole constraint unanswerable — reporting from it called a
// perfectly good association missing, on a page written against a module the
// script creates or run against the wrong app.
func TestValidateXPathMembers_SilentWhenTheBaseEntityIsUnknown(t *testing.T) {
	ctx := memberFixture(t)
	for _, where := range []string{
		"[Shop.Order_Customer/Shop.Customer/Name = 'x']", // a REAL association
		"[Anything = 'x']",
		"[Made.Up_Thing = 'x']",
	} {
		if errs := validateXPathMembers(ctx, pageWith(
			dbGrid("Nowhere.Missing", where))); len(errs) != 0 {
			t.Errorf("reported against an entity the project does not have: %v", errs)
		}
	}
}

// THE REGRESSION this check shipped with, reported against a real app
// (ako/mxcli-sudoku FINDINGS #57). An association a specialization has only
// through its GENERALIZATION was called missing on a page mxbuild builds at 0
// errors — measured on 11.13.0: `Administration.Account` (extends System.User)
// constrained on `System.UserRoles` (declared from System.User).
//
// It landed because a helper documented as "returns false when it cannot
// establish the answer" gained a second caller that read false as evidence. The
// entity's own attributes were fine throughout, because that path walks the
// chain; only the association path compared two names.
func TestValidateXPathMembers_FollowsGeneralizationForAssociations(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, where string }{
		{"an inherited association as a bare hop", "[not(Shop.Base_Region)]"},
		{"an inherited association, then the target's attribute",
			"[Shop.Base_Region/Shop.Region/RegionName != '']"},
		// Same defect in the other spelling: an association that LEAVES its
		// module is stored in CrossAssociations, which the lookup never read.
		{"a cross-module association", "[Shop.Order_Bin/Warehouse.Bin/BinCode != '']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid("Shop.Order", tc.where))); len(errs) != 0 {
				t.Errorf("reported an association the entity really has: %v", errs)
			}
		})
	}
}

// THE CONTROL for the fix above, and the reason it is a chain walk rather than
// "stop reporting associations". Each of these must still be reported, or the
// rule has been disabled rather than corrected.
func TestValidateXPathMembers_StillReportsAnAssociationTheEntityLacks(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, entity, where string }{
		// Real association, wrong entity: Order_Customer runs Order↔Customer and
		// Region is neither, nor a specialization of either.
		{"an association of some other entity", "Shop.Region",
			"[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
		// No association of that name in a module that WAS read.
		{"a name no association has", "Shop.Order", "[Shop.Order_Status = 'Open']"},
		// The generalization direction is one-way: Base does not get Order's.
		{"a SPECIALIZATION's association, from the generalization", "Shop.Base",
			"[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid(tc.entity, tc.where))); len(errs) != 1 {
				t.Errorf("did not report an association the entity lacks: %v", errs)
			}
		})
	}
}

// The other axis of could-not-establish, which the base-entity guard does not
// cover. Shop.Imported's generalization lives in a module the fixture refuses to
// read, so an association it might inherit is UNKNOWABLE — and the entity itself
// is known, so the existing guard lets it through to be reported.
//
// Collapse assocUnknown into a reportable state and this fires.
func TestValidateXPathMembers_SilentWhenTheChainCannotBeWalked(t *testing.T) {
	ctx := memberFixture(t)
	if errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Imported", "[Shop.Order_Customer/Shop.Customer/Name = 'x']"))); len(errs) != 0 {
		t.Errorf("reported past a generalization the backend could not read: %v", errs)
	}

	// CONTROL: the silence is about the CHAIN, not about the entity. A name no
	// association has needs no chain walk to rule out, and is still reported.
	if errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Imported", "[Shop.Nothing_Here = 'x']"))); len(errs) != 1 {
		t.Errorf("a name that exists nowhere went unreported: %v", errs)
	}
}

// Resolving the hop is not only about what is reported: it types the far end, so
// the steps AFTER an inherited association are checked at all. Before the chain
// walk the hop yielded nothing and everything past it was silent.
func TestResolveAssociationFrom_TypesTheFarEndThroughTheChain(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ assoc, from, want string }{
		{"Shop.Base_Region", "Shop.Order", "Shop.Region"}, // inherited, forward
		{"Shop.Base_Region", "Shop.Region", "Shop.Base"},  // and from the far end
		{"Shop.Order_Bin", "Shop.Order", "Warehouse.Bin"}, // cross-module
		{"Shop.Order_Customer", "Shop.Order", "Shop.Customer"},
	} {
		got, res := resolveAssociationFrom(ctx, tc.assoc, tc.from)
		if res != assocResolved || got != tc.want {
			t.Errorf("%s from %s = (%q, %v), want (%q, assocResolved)",
				tc.assoc, tc.from, got, res, tc.want)
		}
	}

	// And the three ways it does not resolve stay distinguishable — the whole
	// point of the type. A boolean here is what produced the false positive.
	for _, tc := range []struct {
		assoc, from string
		want        assocResolution
	}{
		{"Shop.Order_Status", "Shop.Order", assocMissing},
		{"Nowhere.Thing", "Shop.Order", assocMissing},
		{"Shop.Order_Customer", "Shop.Region", assocNotAnEnd},
		{"Shop.Order_Customer", "Shop.Imported", assocUnknown},
	} {
		if _, res := resolveAssociationFrom(ctx, tc.assoc, tc.from); res != tc.want {
			t.Errorf("%s from %s = %v, want %v", tc.assoc, tc.from, res, tc.want)
		}
	}
}

// Only a DATABASE source names its entity in the statement. Anything else
// carries it elsewhere, so the constraint is left unchecked rather than checked
// against the wrong entity.
func TestValidateXPathMembers_OnlyChecksADatabaseSource(t *testing.T) {
	ctx := memberFixture(t)
	w := &ast.WidgetV3{
		Type: "datagrid", Name: "g",
		Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{
				Type: "association", Reference: "Shop.Order", Where: "[Nonsense = 'x']",
			},
		},
	}
	if errs := validateXPathMembers(ctx, pageWith(w)); len(errs) != 0 {
		t.Errorf("checked a non-database source: %v", errs)
	}
}

// A constraint mxcli cannot parse is skipped, not reported. The XPath parse runs
// with ANTLR's error listeners removed and can hand back a tree that quietly
// omits part of its input, so reporting from a partial tree means an error about
// text nobody wrote.
func TestValidateXPathMembers_SkipsAnUnparseableConstraint(t *testing.T) {
	ctx := memberFixture(t)
	for _, where := range []string{"[", "[[[", "[ = = ]"} {
		if errs := validateXPathMembers(ctx, pageWith(
			dbGrid("Shop.Order", where))); len(errs) != 0 {
			t.Errorf("reported from a constraint it could not read (%q): %v", where, errs)
		}
	}
}

// ---------------------------------------------------------------------------
// mendixlabs/mxcli#1271 — members the SAME script adds to a stored entity
// ---------------------------------------------------------------------------

// The microflow retrieve learned to see the members a script adds; the page
// data source did not. `alter entity Shop.Order add attribute IsPaid …` and a
// grid `database from Shop.Order where [IsPaid = true]` in one script was
// "names \"IsPaid\", which is neither an attribute nor an association of it",
// and exec refused a script mxbuild builds clean. Same for an attribute added by
// restating the entity, and for an association the script creates.
func TestValidateXPathMembers_SeesMembersTheScriptAdds(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, src string }{
		{"alter entity … add attribute", `alter entity Shop.Order add attribute IsPaid: Boolean default false;
create page Shop.P (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database from Shop.Order where [IsPaid = true]) {
    column (Attribute: OrderNo, Caption: 'No')
  }
};`},
		{"create or modify restating the entity", `create or modify persistent entity Shop.Customer (Name: String(200), Vip: Boolean default false);
create page Shop.P (Title: 'Customers', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database from Shop.Customer where [Vip = true]) {
    column (Attribute: Name, Caption: 'Name')
  }
};`},
		{"an association the script creates", `create association Shop.Order_Home from Shop.Order to Shop.Region type Reference;
create page Shop.P (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database from Shop.Order where [Shop.Order_Home/Shop.Region/RegionName = 'x']) {
    column (Attribute: OrderNo, Caption: 'No')
  }
};`},
	} {
		if errs := validateXPathMembers(ctx, parseMDL(t, tc.src)); len(errs) != 0 {
			t.Errorf("%s: a member the script adds was reported missing: %v", tc.name, errs)
		}
	}
}

// CONTROL: seeing the script is not a blanket pass. A restated entity's members
// are the ones the statement lists, so a name it does not list is still missing;
// and altering one entity vouches for nothing on another.
func TestValidateXPathMembers_ScriptMembersDoNotHideATypo(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{"restated entity, unlisted member", `create or modify persistent entity Shop.Customer (Name: String(200), Vip: Boolean default false);
create page Shop.P (Title: 'Customers', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database from Shop.Customer where [Vipp = true]) {
    column (Attribute: Name, Caption: 'Name')
  }
};`, "Vipp"},
		{"another entity altered", `alter entity Shop.Order add attribute IsPaid: Boolean default false;
create page Shop.P (Title: 'Customers', Layout: Atlas_Core.Atlas_Default) {
  datagrid dg (DataSource: database from Shop.Customer where [IsPaid = true]) {
    column (Attribute: Name, Caption: 'Name')
  }
};`, "IsPaid"},
	} {
		errs := validateXPathMembers(ctx, parseMDL(t, tc.src))
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), tc.want) {
			t.Errorf("%s: want one error naming %q, got %v", tc.name, tc.want, errs)
		}
	}
}
