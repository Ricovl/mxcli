// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1406: a text-template parameter Studio Pro stores as an
// Expression (`$currentObject/CollectionIdentifierLabel`, `$ChatContext/Topic`)
// was described as written and refused by check (MDL-WIDGET24), and executing
// it wrote the expression into the attribute name. The round trip: the stored
// expression is described, the description passes check, and executing it
// writes nothing.
func TestTemplateVariablePathRoundTrips_1406(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.addToFixture(`create module RT1406;
create persistent entity RT1406.EntA (Name: String(50));
create page RT1406.Pg_A (Title: 'A', Layout: Atlas_Core.Atlas_Default, Params: ( $EntA: RT1406.EntA )) {
  dataview dv1 (DataSource: $EntA) {
    dynamictext t1 (Content: '{1}', ContentParams: ({1} = $currentObject/Name))
    dynamictext t2 (Content: '{1}', ContentParams: ({1} = $EntA/Name))
  }
};`)

	// What was stored is Studio Pro's shape: the expression, no attribute.
	snap := h.snapshot()
	var got []string
	for id, label := range snap.label {
		if label == "Forms$Page Pg_A" {
			got = templateParamExpressions(t, snap.units[id])
		}
	}
	if strings.Join(got, ",") != "$currentObject/Name,$EntA/Name" {
		t.Fatalf("stored template parameter expressions = %q, want the two expressions as written", got)
	}

	first, err := h.describe("page RT1406.Pg_A")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, want := range []string{"({1} = $currentObject/Name)", "({1} = $EntA/Name)"} {
		if !strings.Contains(first, want) {
			t.Fatalf("describe does not print %s:\n%s", want, first)
		}
	}
	prog, errs := visitor.Build(first)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs[0], first)
	}
	for _, v := range executor.ValidateProgram(prog, h.mpr) {
		if v.Severity == linter.SeverityError {
			t.Errorf("check refuses the describe output: [%s] %s", v.RuleID, v.Message)
		}
	}
	if errs := h.exe.ValidateProgram(prog); len(errs) > 0 {
		t.Errorf("check --references refuses the describe output: %v", errs)
	}
	if err := h.exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("exec the describe output: %v", err)
	}
	if changed := snap.diff(h.snapshot()); len(changed) > 0 {
		t.Errorf("executing the describe output wrote:\n  %s", strings.Join(changed, "\n  "))
	}
	if again, _ := h.describe("page RT1406.Pg_A"); again != first {
		t.Errorf("describe -> exec -> describe changed the page:\n%s", lineDiff(first, again))
	}
}

// templateParamExpressions lists the Expression of every
// Forms$ClientTemplateParameter in a unit, in document order.
func templateParamExpressions(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			isParam, expr := false, ""
			for _, e := range x {
				if e.Key == "$Type" && e.Value == "Forms$ClientTemplateParameter" {
					isParam = true
				}
				if e.Key == "Expression" {
					expr, _ = e.Value.(string)
				}
			}
			if isParam {
				out = append(out, expr)
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}
