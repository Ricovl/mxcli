// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// alter workflow on the generic ALTER (ADR-0012 decision 2, ako/mxcli#712):
// every old action spelling builds exactly the statement its canonical
// spelling builds, records its own MDL-DEPR14x code, and the canonical
// spelling records nothing.
var alterWorkflowPairs = []struct {
	name, old, canonical, code string
}{
	{"set display", `alter workflow M.W set display 'Order';`,
		`alter workflow M.W { set (Display: 'Order'); };`, deprecation.AlterWorkflowSet},
	{"set description", `alter workflow M.W set description 'd';`,
		`alter workflow M.W { set (Description: 'd'); };`, deprecation.AlterWorkflowSet},
	{"set export level", `alter workflow M.W set export level API;`,
		`alter workflow M.W { set (ExportLevel: API); };`, deprecation.AlterWorkflowSet},
	{"set export level hidden", `alter workflow M.W set export level Hidden;`,
		`alter workflow M.W { set (ExportLevel: Hidden); };`, deprecation.AlterWorkflowSet},
	{"set due date", `alter workflow M.W set due date addDays([%CurrentDateTime%], 3);`,
		`alter workflow M.W { set (DueDate: addDays([%CurrentDateTime%], 3)); };`, deprecation.AlterWorkflowSet},
	{"set overview page", `alter workflow M.W set overview page M.Admin;`,
		`alter workflow M.W { set (OverviewPage: M.Admin); };`, deprecation.AlterWorkflowSet},
	{"set parameter", `alter workflow M.W set parameter $WorkflowContext: M.Ctx;`,
		`alter workflow M.W { set (Parameter: $WorkflowContext: M.Ctx); };`, deprecation.AlterWorkflowSet},
	{"set activity page", `alter workflow M.W set activity Review page M.TaskPage;`,
		`alter workflow M.W { set (Page: M.TaskPage) on Review; };`, deprecation.AlterWorkflowSetActivity},
	{"set activity page by caption @2", `alter workflow M.W set activity 'Review order'@2 page M.TaskPage;`,
		`alter workflow M.W { set (Page: M.TaskPage) on 'Review order'@2; };`, deprecation.AlterWorkflowSetActivity},
	{"set activity description", `alter workflow M.W set activity Review description 'x';`,
		`alter workflow M.W { set (Description: 'x') on Review; };`, deprecation.AlterWorkflowSetActivity},
	{"set activity targeting microflow", `alter workflow M.W set activity Review targeting microflow M.Target;`,
		`alter workflow M.W { set (Targeting: microflow M.Target) on Review; };`, deprecation.AlterWorkflowSetActivity},
	{"set activity targeting xpath", `alter workflow M.W set activity Review targeting xpath [Active = true()];`,
		`alter workflow M.W { set (Targeting: xpath [Active = true()]) on Review; };`, deprecation.AlterWorkflowSetActivity},
	{"set activity due date", `alter workflow M.W set activity Review due date addDays([%CurrentDateTime%], 1);`,
		`alter workflow M.W { set (DueDate: addDays([%CurrentDateTime%], 1)) on Review; };`, deprecation.AlterWorkflowSetActivity},
	{"insert after", `alter workflow M.W insert after Review call microflow M.Notify;`,
		`alter workflow M.W { insert after Review { call microflow M.Notify; } };`, deprecation.AlterWorkflowInsertAfter},
	{"drop activity", `alter workflow M.W drop activity Review;`,
		`alter workflow M.W { drop Review; };`, deprecation.AlterWorkflowDropActivity},
	{"replace activity", `alter workflow M.W replace activity Review with call microflow M.Notify;`,
		`alter workflow M.W { replace Review with { call microflow M.Notify; } };`, deprecation.AlterWorkflowReplaceActivity},
	{"insert outcome", `alter workflow M.W insert outcome 'Escalate' on Review { call microflow M.Notify; };`,
		`alter workflow M.W { insert into Review { outcomes 'Escalate' { call microflow M.Notify; } } };`, deprecation.AlterWorkflowInsertOutcome},
	{"insert path", `alter workflow M.W insert path on split1 { call microflow M.Notify; };`,
		`alter workflow M.W { insert into split1 { path { call microflow M.Notify; } } };`, deprecation.AlterWorkflowInsertPath},
	{"insert condition", `alter workflow M.W insert condition 'true' on decision1 { };`,
		`alter workflow M.W { insert into decision1 { outcomes 'true' -> { } } };`, deprecation.AlterWorkflowInsertCondition},
	{"insert boundary event", `alter workflow M.W insert boundary event on Review interrupting timer addHours([%CurrentDateTime%], 1) { };`,
		`alter workflow M.W { insert into Review { boundary event interrupting timer addHours([%CurrentDateTime%], 1) { } } };`, deprecation.AlterWorkflowInsertBoundaryEvent},
	{"drop outcome", `alter workflow M.W drop outcome 'Reject' on Review;`,
		`alter workflow M.W { drop Review outcome 'Reject'; };`, deprecation.AlterWorkflowDropMember},
	{"drop condition true", `alter workflow M.W drop condition 'true' on decision1;`,
		`alter workflow M.W { drop decision1 outcome true; };`, deprecation.AlterWorkflowDropMember},
	{"drop condition default", `alter workflow M.W drop condition 'default' on decision1;`,
		`alter workflow M.W { drop decision1 outcome default; };`, deprecation.AlterWorkflowDropMember},
	{"drop path", `alter workflow M.W drop path 'Path 2' on split1;`,
		`alter workflow M.W { drop split1 path 2; };`, deprecation.AlterWorkflowDropMember},
	{"drop boundary event", `alter workflow M.W drop boundary event on Review;`,
		`alter workflow M.W { drop Review boundary event; };`, deprecation.AlterWorkflowDropMember},
}

func TestAlterWorkflowGeneric_OldAndCanonicalBuildTheSameStatement(t *testing.T) {
	for _, tc := range alterWorkflowPairs {
		t.Run(tc.name, func(t *testing.T) {
			old := mustBuild(t, tc.old)
			canon := mustBuild(t, tc.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical %q recorded %v, want none", tc.canonical, got)
			}
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{tc.code}) {
				t.Errorf("old %q recorded %v, want [%s]", tc.old, got, tc.code)
			}
			if len(canon.Statements) != 1 {
				t.Fatalf("canonical built %d statements", len(canon.Statements))
			}
			if _, ok := canon.Statements[0].(*ast.AlterWorkflowStmt); !ok {
				t.Fatalf("canonical built %T", canon.Statements[0])
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements[0], canon.Statements[0])
			}
		})
	}
}

// Several operations in one block, one property list setting several keys,
// and the forms the old grammar had no spelling for.
func TestAlterWorkflowGeneric_BlockForms(t *testing.T) {
	prog := mustBuild(t, `alter workflow M.W {
  set (Display: 'Order', Description: 'Handles orders');
  set (Page: M.TaskPage, DueDate: addDays([%CurrentDateTime%], 2)) on Review;
  insert before Review { call microflow M.Prepare; call microflow M.Log; }
  insert after 'Review order'@2 { wait for notification waitHere; }
  insert into split1 { path 3 { call microflow M.Notify; } }
  insert into decision1 { outcomes false -> { } default -> { } }
  drop Review outcome 'Reject', split1 path 1, callMf boundary event;
};`)
	if got := deprecationCodes(prog); len(got) != 0 {
		t.Fatalf("recorded %v, want none", got)
	}
	s := prog.Statements[0].(*ast.AlterWorkflowStmt)
	var kinds []string
	for _, op := range s.Operations {
		kinds = append(kinds, strings.TrimPrefix(reflect.TypeOf(op).String(), "*ast."))
	}
	want := []string{
		"SetWorkflowPropertyOp", "SetWorkflowPropertyOp",
		"SetActivityPropertyOp", "SetActivityPropertyOp",
		"InsertBeforeOp", "InsertAfterOp", "InsertPathOp",
		"InsertBranchOp", "InsertBranchOp",
		"DropOutcomeOp", "DropPathOp", "DropBoundaryEventOp",
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("ops = %v\nwant %v", kinds, want)
	}
	before := s.Operations[4].(*ast.InsertBeforeOp)
	if before.ActivityRef != "Review" || len(before.NewActivities) != 2 {
		t.Errorf("insert before = %+v", before)
	}
	after := s.Operations[5].(*ast.InsertAfterOp)
	if after.ActivityRef != "Review order" || after.AtPosition != 2 {
		t.Errorf("insert after target = %q@%d", after.ActivityRef, after.AtPosition)
	}
	if p := s.Operations[6].(*ast.InsertPathOp); p.PathNumber != 3 {
		t.Errorf("path number = %d, want 3", p.PathNumber)
	}
	if d := s.Operations[8].(*ast.InsertBranchOp); d.Condition != "default" {
		t.Errorf("branch = %q, want default", d.Condition)
	}
	if d := s.Operations[10].(*ast.DropPathOp); d.PathCaption != "Path 1" {
		t.Errorf("drop path = %q", d.PathCaption)
	}
}

// A workflow activity has a name or a caption, never a member path or a grid
// column address; an unknown property key names the ones that exist.
func TestAlterWorkflowGeneric_RefusesAddressesAndKeysItCannotMean(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`alter workflow M.W { drop Review.Caption; };`, "addressed by its name"},
		{`alter workflow M.W { drop dg column(Name); };`, "addressed by its name"},
		{`alter workflow M.W { set (Colour: 'red'); };`, "Display"},
		{`alter workflow M.W { set (Page: M.P); };`, "on <activity>"},
		{`alter workflow M.W { set (ExportLevel: API) on Review; };`, "Page"},
	} {
		_, errs := Build(tc.src)
		if len(errs) == 0 {
			t.Errorf("%s: accepted", tc.src)
			continue
		}
		if !strings.Contains(errs[0].Error(), tc.want) {
			t.Errorf("%s: error %q does not mention %q", tc.src, errs[0], tc.want)
		}
	}
}

// Several old actions in one statement become one block; the braces belong to
// the first and last action's rewrite, so each record still has one code.
func TestAlterWorkflowGeneric_MultiActionOldFormBuildsTheBlock(t *testing.T) {
	old := mustBuild(t, `alter workflow M.W set display 'A' drop activity X insert after Y call microflow M.F;`)
	canon := mustBuild(t, `alter workflow M.W { set (Display: 'A'); drop X; insert after Y { call microflow M.F; } };`)
	if !reflect.DeepEqual(old.Statements, canon.Statements) {
		t.Fatalf("different statements:\n old:   %#v\n canon: %#v", old.Statements[0], canon.Statements[0])
	}
	want := []string{deprecation.AlterWorkflowSet, deprecation.AlterWorkflowDropActivity, deprecation.AlterWorkflowInsertAfter}
	if got := deprecationCodes(old); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
}
