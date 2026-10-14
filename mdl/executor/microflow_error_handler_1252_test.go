// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1252: an Aggregate list activity with a custom error handler,
// authored in Studio Pro 11.12.3, came back from DESCRIBE as a bare
// `$count = count($TEYAs);`, and `on error` could not be written on it — nor on
// list operations, create list, add/remove, cast, rollback or show home page.
//
// Every one of those is a Microflows$MicroflowAction, the type that declares
// errorHandlingType in the Mendix Model SDK (mendixmodelsdk 4.116.0,
// src/gen/microflows.js, unbounded StructureVersionInfo). MDL077 refused the
// clause on list operations and aggregates on the premise that the property was
// absent from the metamodel; it is not.

// The statements #1252 opens up, one row per grammar rule (and per alternative,
// where a rule has a deprecated call form that is still accepted headerless).
func onErrorStatements1252() []struct{ name, body string } {
	return []struct{ name, body string }{
		{"count", "$n = count $list on error { rollback $x; };"},
		{"sum", "$s = sum $list by Price on error { rollback $x; };"},
		{"head", "$h = head $list on error { rollback $x; };"},
		{"filter", "$f = filter $list where $currentObject/Paid on error { rollback $x; };"},
		{"count call form", "$n = count($list) on error { rollback $x; };"},
		{"head call form", "$h = head($list) on error { rollback $x; };"},
		{"create list", "$l = create list of M.Car on error { rollback $x; };"},
		{"add", "add $Car to $list on error { rollback $x; };"},
		{"remove", "remove $Car from $list on error { rollback $x; };"},
		{"cast", "$c = cast $Car on error { rollback $x; };"},
		{"rollback", "rollback $Car on error { rollback $x; };"},
		{"show home page", "show home page on error { rollback $x; };"},
	}
}

func TestAuthorOnError_1252_StatementsThatCouldNotCarryTheClause(t *testing.T) {
	for _, tc := range onErrorStatements1252() {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, tc.body)

			// One activity carries Custom: the one the clause was written on. The
			// handler's own `rollback $x` keeps the flow default.
			if n := countErrorHandling(fb, microflows.ErrorHandlingTypeCustom); n != 1 {
				t.Errorf("%d activities carry ErrorHandlingType %q, want 1 — got %v",
					n, microflows.ErrorHandlingTypeCustom, actionErrorHandlingTypes(fb))
			}
			var errFlows int
			for _, f := range fb.flows {
				if f.IsErrorHandler {
					errFlows++
				}
			}
			if errFlows != 1 {
				t.Errorf("got %d error-handler flows, want 1 — the handler body was not wired", errFlows)
			}
		})
	}
}

// The bodyless forms reach the action too, and `without rollback` stays distinct.
func TestAuthorOnError_1252_BodylessAndWithoutRollback(t *testing.T) {
	for _, tc := range []struct {
		body string
		want microflows.ErrorHandlingType
	}{
		{"$n = count $list on error continue;", microflows.ErrorHandlingTypeContinue},
		{"$n = count $list on error without rollback { rollback $x; };", microflows.ErrorHandlingTypeCustomWithoutRollback},
		{"rollback $Car on error continue;", microflows.ErrorHandlingTypeContinue},
		{"show home page on error continue;", microflows.ErrorHandlingTypeContinue},
	} {
		fb := buildFlowFromMDL(t, tc.body)
		if got := actionErrorHandlingTypes(fb); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s\n  got %v, want first activity %q", tc.body, got, tc.want)
		}
	}
}

// Control: with no clause, rollback and show home page keep writing NO
// ErrorHandlingType, which is what both writers have always done. Supplying the
// flow default here would change every existing rollback activity's BSON.
func TestAuthorOnError_1252_NoClauseLeavesRollbackAndHomePageUnset(t *testing.T) {
	for _, body := range []string{"rollback $Car;", "show home page;"} {
		fb := buildFlowFromMDL(t, body)
		if got := actionErrorHandlingTypes(fb); len(got) != 1 || got[0] != "" {
			t.Errorf("%s: got %v, want one activity with no ErrorHandlingType", body, got)
		}
	}
}

// MDL077 is gone with its premise: the clause is no longer refused on a list
// operation or an aggregate.
func TestMDL077_NoLongerRefusesListOperationsAndAggregates(t *testing.T) {
	for _, body := range []string{
		"$h = head $list on error { rollback $x; };",
		"$n = count $list on error { rollback $x; };",
	} {
		if out := checkMicroflowBodyForTest(t, body); strings.Contains(out, "MDL077") {
			t.Errorf("MDL077 fired on %q:\n%s", body, out)
		}
	}
}

// The read side. Rollback and show home page carried no ErrorHandlingType in
// sdk/microflows, so DESCRIBE saw "" and dropped the whole handler branch — the
// reported symptom. The list activities already carried the field, so for them
// the describer emitted a live clause the grammar could not read back.
func TestDescribe_1252_ErrorHandlerSurvives(t *testing.T) {
	for _, tc := range []errorHandlerCase{
		{"aggregate", &microflows.AggregateListAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			Function:          microflows.AggregateFunctionCount,
			InputVariable:     "TEYAs", OutputVariable: "count",
		}, "$count = count $TEYAs"},
		{"rollback", &microflows.RollbackObjectAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			RollbackVariable:  "Car",
		}, "rollback $Car"},
		{"show home page", &microflows.ShowHomePageAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustomWithoutRollback,
		}, "show home page on error without rollback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := describeWithErrorHandler(t, tc.action)
			if !strings.Contains(out, "rollback $Handled;") || !strings.Contains(out, "on error") {
				t.Errorf("the error branch was dropped:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("statement %q missing from output:\n%s", tc.want, out)
			}
		})
	}
}

// MEASURED on mxbuild 11.12.3 (the reporter's version), one document per cell:
// in a MICROFLOW every statement #1252 opened up accepts both custom handlers
// and `on error rollback`, and every one but `rollback` rejects
// `on error continue` with CE6035. MDL076 must say so at check time.
func TestMDL076_1252_ContinueRefusedWhereMxbuildRejectsIt(t *testing.T) {
	for _, body := range []string{
		"$n = count $list on error continue;",
		"$s = sum $list by Price on error continue;",
		"$h = head $list on error continue;",
		"$l = create list of M.Car on error continue;",
		"add $Car to $list on error continue;",
		"remove $Car from $list on error continue;",
		"$c = cast $Car on error continue;",
		"show home page on error continue;",
	} {
		if out := checkMicroflowBodyForTest(t, body); !strings.Contains(out, "MDL076") {
			t.Errorf("no MDL076 for %q, but mxbuild reports CE6035:\n%s", body, out)
		}
	}
	// Controls: rollback accepts continue, and the custom handler is accepted
	// on every row above.
	for _, body := range []string{
		"rollback $Car on error continue;",
		"$n = count $list on error begin return; end error;",
		"add $Car to $list on error without rollback begin return; end error;",
		"show home page on error rollback;",
	} {
		if out := checkMicroflowBodyForTest(t, body); strings.Contains(out, "MDL076") {
			t.Errorf("MDL076 fired on %q, which builds:\n%s", body, out)
		}
	}
}

// MEASURED on mxbuild 11.12.3: in a NANOFLOW the list activities and cast accept
// only a custom handler WITHOUT rollback; rollback accepts every form.
func TestNanoflow_1252_ListActivitiesTakeOnlyWithoutRollback(t *testing.T) {
	stmts := []string{
		"$n = count $list", "$h = head $list", "$l = create list of M.Car",
		"add $Car to $list", "remove $Car from $list", "$c = cast $Car",
	}
	for _, stmt := range stmts {
		for _, clause := range []string{
			" on error continue;", " on error rollback;", " on error begin return; end error;",
		} {
			if errs := nanoflowErrorsFor(t, stmt+clause); !containsSubstringAny(errs, "on error") {
				t.Errorf("%s%s accepted in a nanoflow, but mxbuild reports CE6035: %v", stmt, clause, errs)
			}
		}
		for _, ok := range []string{" on error without rollback begin return; end error;", ";"} {
			if errs := nanoflowErrorsFor(t, stmt+ok); containsSubstringAny(errs, "on error") {
				t.Errorf("%s%s refused, but it builds: %v", stmt, ok, errs)
			}
		}
	}
	// Control: rollback builds with every clause in a nanoflow.
	for _, clause := range []string{" on error continue;", " on error rollback;", " on error begin return; end error;"} {
		if errs := nanoflowErrorsFor(t, "rollback $Car"+clause); containsSubstringAny(errs, "on error") {
			t.Errorf("rollback $Car%s refused, but it builds: %v", clause, errs)
		}
	}
}
