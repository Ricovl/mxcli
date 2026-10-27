// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A constraint is evaluated on its entity, so it cannot start with that entity
// (or any entity) as a path step: `[Shop.Order/Status = 'Open']` on Shop.Order.
// The XPath member walk read the step as a cast and passed it; mxbuild 11.15.0
// reports CE0161 for it in a microflow retrieve and in a list view's database
// source, as it does in a user task's targeting (ako/mxcli#1711).

func TestLeadingEntityStep_ReportedOnARetrieve(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateRetrieveMembers(ctx, []retrieveConstraintRef{{
		entity: "Shop.Order", stored: "[Shop.Order/Status = 'Open']",
	}}, newScriptContext())
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	for _, w := range []string{"Shop.Order", "CE0161", "[Status = …]"} {
		if !strings.Contains(errs[0], w) {
			t.Errorf("error should mention %q: %s", w, errs[0])
		}
	}
}

func TestLeadingEntityStep_ReportedOnAWidgetSource(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateXPathMembers(ctx, pageWith(dbGrid("Shop.Order", "[Shop.Order/Status = 'Open']")))
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, "CE0161") || strings.Contains(msg, "CE1613") {
		t.Errorf("mxbuild reports a leading entity step as CE0161, not as a missing member: %s", msg)
	}
}

// CONTROL: an entity step AFTER an association is the specialization step of a
// path, and builds.
func TestLeadingEntityStep_AnEntityAfterAnAssociationIsFine(t *testing.T) {
	ctx := memberFixture(t)
	const where = "[Shop.Order_Customer/Shop.Customer/Name = 'x']"
	if errs := validateRetrieveMembers(ctx, []retrieveConstraintRef{{entity: "Shop.Order", stored: where}},
		newScriptContext()); len(errs) != 0 {
		t.Errorf("retrieve: %v", errs)
	}
	if errs := validateXPathMembers(ctx, pageWith(dbGrid("Shop.Order", where))); len(errs) != 0 {
		t.Errorf("widget source: %v", errs)
	}
}

// CONTROL: the plain walk, which the access-rule check uses, still passes a
// leading entity step. mx check did not reject one in an access rule on the
// same app, so that caller is left as it was.
func TestLeadingEntityStep_PlainWalkStaysSilent(t *testing.T) {
	ctx := memberFixture(t)
	m, resolve := scriptXPathResolver(ctx, newScriptContext())
	if bad := unresolvableXPathStepsWith(ctx, m, "[Shop.Order/Status = 'Open']", "Shop.Order", resolve); len(bad) != 0 {
		t.Errorf("the plain walk now reports %v", bad)
	}
}

// Any entity as the first step is CE0161 — measured on 11.15.0 for a
// specialization of the retrieved entity and for an unrelated one too — but
// dropping the step only helps when it names the constrained entity or a
// generalization of it: the members are then the entity's own.
func TestLeadingEntityStep_TheFixDependsOnWhichEntity(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct {
		name, where string
		dropFix     bool
	}{
		{"the constrained entity", "[Shop.Order/Status = 'Open']", true},
		{"a generalization", "[Shop.Base/Code = 'x']", true},
		{"another entity", "[Shop.Customer/Name = 'x']", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateRetrieveMembers(ctx, []retrieveConstraintRef{{entity: "Shop.Order", stored: tc.where}}, newScriptContext())
			if len(errs) != 1 {
				t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
			}
			if got := strings.Contains(errs[0], "drop the leading"); got != tc.dropFix {
				t.Errorf("offers dropping the step = %v, want %v: %s", got, tc.dropFix, errs[0])
			}
			if !tc.dropFix && !strings.Contains(errs[0], "association") {
				t.Errorf("for another entity the fix is to reach it through an association: %s", errs[0])
			}
		})
	}
}
