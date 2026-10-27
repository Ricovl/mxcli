// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1685, the read side. A stored constraint holding `@Module.Constant`
// was re-parsed with `@` as an attribute name: describe's enum enrichment then
// printed `Total > @`, and the long-constraint layout wrote `[Total > @]` back.
// mxbuild rejects such a constraint (CE0161, see validateXPathConstants), but a
// stored one is still described as stored, not as something shorter.

func TestXPathConstant_EnrichmentKeepsTheConstant(t *testing.T) {
	got := enrichXPathGroups("[Status = 'Open' and Total > @Shop.C_Limit]", map[string]string{"Status": "Shop.S"})
	if want := "[Status = Shop.S.Open and Total > @Shop.C_Limit]"; got != want {
		t.Errorf("enrichXPathGroups = %q, want %q", got, want)
	}
}

func TestXPathConstant_StoredLayoutKeepsTheConstant(t *testing.T) {
	where := "Total > @Shop.C_Limit and Name != empty and Description != empty and Category = 'Hardware'"
	got := retrieveXPathConstraint(&ast.SourceExpr{Source: where}, "Shop.Item")
	if !strings.Contains(got, "Total > @Shop.C_Limit") {
		t.Errorf("stored constraint lost the constant:\n%s", got)
	}
}

// End to end on a real project: the constraint is written as given, described
// back as written (with enum enrichment in play, which is where `@` went
// missing), re-executing the description writes nothing, and RENAME CONSTANT
// rewrites it — the path ako/mxcli#1671 could only unit-test, because MDL could
// not write a constant into a constraint. This runs the executor directly, past
// the MDL-XPATH03 refusal a script would meet first.
func TestXPathConstant_DescribeRoundTripAndRename(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	setup := `create module Shop1685;
create constant Shop1685.C_Limit (Type: Integer, DefaultValue: '10');
create enumeration Shop1685.Status (Open 'Open', Closed 'Closed');
create persistent entity Shop1685.Item (Total: Integer, Name: String(100), Status: Shop1685.Status);
create module role Shop1685.User;
create microflow Shop1685.Fetch ()
begin
  retrieve $Items from Shop1685.Item where [Status = Shop1685.Status.Open and Total > @Shop1685.C_Limit];
end;
create page Shop1685.Items (title: 'Items', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database from Shop1685.Item where [Total > @Shop1685.C_Limit]) {
    dynamictext t (Content: 'x')
  }
};
grant read * on entity Shop1685.Item to Shop1685.User where [Total > @Shop1685.C_Limit];`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	describe := func(stmt string) string {
		t.Helper()
		out.Reset()
		if err := afRun(t, exec, stmt); err != nil {
			t.Fatalf("%s: %v\n%s", stmt, err, out.String())
		}
		return out.String()
	}
	describeAll := func() map[string]string {
		return map[string]string{
			"microflow": describe("describe microflow Shop1685.Fetch;"),
			"page":      describe("describe page Shop1685.Items;"),
			"entity":    describe("describe entity Shop1685.Item;"),
		}
	}

	descs := describeAll()
	for doc, want := range map[string]string{
		"microflow": "Total > @Shop1685.C_Limit",
		"page":      "[Total > @Shop1685.C_Limit]",
		"entity":    "where [Total > @Shop1685.C_Limit]",
	} {
		if !strings.Contains(descs[doc], want) {
			t.Errorf("describe %s lacks %q:\n%s", doc, want, descs[doc])
		}
	}

	// Describe → exec writes nothing.
	before := projectFiles(t, dir)
	for doc, d := range descs {
		if err := afRun(t, exec, d); err != nil {
			t.Fatalf("re-executing the %s description: %v\n%s", doc, err, d)
		}
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("re-executing the descriptions rewrote %v", changed)
	}

	out.Reset()
	if err := afRun(t, exec, "rename constant Shop1685.C_Limit to C_Max;"); err != nil {
		t.Fatalf("rename: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Updated 3 reference(s)") {
		t.Errorf("rename should report the three constraints it rewrote:\n%s", out.String())
	}
	for doc, d := range describeAll() {
		if strings.Contains(d, "C_Limit") || !strings.Contains(d, "@Shop1685.C_Max") {
			t.Errorf("after the rename, describe %s should name @Shop1685.C_Max only:\n%s", doc, d)
		}
	}
}
