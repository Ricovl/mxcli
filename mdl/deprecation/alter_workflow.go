// SPDX-License-Identifier: Apache-2.0

package deprecation

// Codes 140-149 are `alter workflow`'s old action forms, ported onto the
// generic ALTER (ADR-0012 decision 2, ako/mxcli#712):
//
//	alter workflow M.W {
//	  set ( Key: value, … ) [on <activity>];
//	  insert before|after <activity> { <activities> }
//	  insert into <activity> { outcomes … | path { … } | boundary event … }
//	  replace <activity> with { <activities> }
//	  drop <activity> [outcome 'X' | outcome true | path n | boundary event];
//	}
//
// Each old action is recorded under its own code. The old statement had no
// braces; the rewrite of a statement's first action opens the block and the
// rewrite of its last closes it, so every record still carries one code.
const (
	// AlterWorkflowSet is `set display 'x'`, `set description …`, `set export
	// level …`, `set due date …`, `set overview page …`, `set parameter …`.
	AlterWorkflowSet = "MDL-DEPR140"
	// AlterWorkflowSetActivity is `set activity X page M.P` and the other
	// activity properties.
	AlterWorkflowSetActivity = "MDL-DEPR141"
	// AlterWorkflowInsertAfter is `insert after X <activity>;`, the activity
	// not in braces.
	AlterWorkflowInsertAfter = "MDL-DEPR142"
	// AlterWorkflowDropActivity is `drop activity X`.
	AlterWorkflowDropActivity = "MDL-DEPR143"
	// AlterWorkflowReplaceActivity is `replace activity X with <activity>;`.
	AlterWorkflowReplaceActivity = "MDL-DEPR144"
	// AlterWorkflowInsertOutcome is `insert outcome 'N' on X { … }`.
	AlterWorkflowInsertOutcome = "MDL-DEPR145"
	// AlterWorkflowInsertPath is `insert path on X { … }`.
	AlterWorkflowInsertPath = "MDL-DEPR146"
	// AlterWorkflowInsertCondition is `insert condition 'V' on X { … }`.
	AlterWorkflowInsertCondition = "MDL-DEPR147"
	// AlterWorkflowInsertBoundaryEvent is `insert boundary event on X <event>`.
	AlterWorkflowInsertBoundaryEvent = "MDL-DEPR148"
	// AlterWorkflowDropMember is `drop outcome 'N' on X`, `drop condition 'V'
	// on X`, `drop path 'Path n' on X` and `drop boundary event on X`.
	AlterWorkflowDropMember = "MDL-DEPR149"
)

const alterWorkflowBlockNote = "`alter workflow` is the generic alter (ADR-0012): its operations go in " +
	"{ }, a target is an activity's name or 'caption' with an optional @n, and a fragment is " +
	"written exactly as in `create workflow`."

var alterWorkflowEntries = []Entry{
	{
		Code:      AlterWorkflowSet,
		Old:       "alter workflow M.W set display 'x' / set description … / set export level … / set due date … / set overview page … / set parameter $P: M.E",
		Canonical: "alter workflow M.W { set ( Display: 'x', Description: …, ExportLevel: …, DueDate: …, OverviewPage: …, Parameter: $P: M.E ); }",
		Rewrite:   Rewrite{Structural: "property as `set ( Key: value )`, inside the statement's { }"},
		RemovedIn: 2,
		Note:      "R3: `:` sets a model property, in create's property list. " + alterWorkflowBlockNote,
		Example:   "alter workflow M.W set display 'Order';",
		// The rewrite keeps the old statement's own terminator after the block.
		CanonicalExample: "alter workflow M.W { set (Display: 'Order'); };",
	},
	{
		Code:             AlterWorkflowSetActivity,
		Old:              "alter workflow M.W set activity X page M.P / description … / targeting … / due date …",
		Canonical:        "alter workflow M.W { set ( Page: M.P, Description: …, Targeting: …, DueDate: … ) on X; }",
		Rewrite:          Rewrite{Structural: "`set activity X <prop> v` as `set ( Key: v ) on X`, inside the statement's { }"},
		RemovedIn:        2,
		Note:             "The target follows `on`, as in every generic alter. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W set activity Review page M.TaskPage;",
		CanonicalExample: "alter workflow M.W { set (Page: M.TaskPage) on Review; };",
	},
	{
		Code:             AlterWorkflowInsertAfter,
		Old:              "alter workflow M.W insert after X <activity>;",
		Canonical:        "alter workflow M.W { insert after X { <activity>; } }",
		Rewrite:          Rewrite{Structural: "inserted activity in { }, inside the statement's { }"},
		RemovedIn:        2,
		Note:             "A fragment is a brace body, so it can hold several activities. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W insert after Review call microflow M.Notify;",
		CanonicalExample: "alter workflow M.W { insert after Review { call microflow M.Notify; } };",
	},
	{
		Code:             AlterWorkflowDropActivity,
		Old:              "alter workflow M.W drop activity X",
		Canonical:        "alter workflow M.W { drop X; }",
		Rewrite:          Rewrite{Structural: "`drop activity X` as `drop X`, inside the statement's { }"},
		RemovedIn:        2,
		Note:             "The target names the element; the kind is its own. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W drop activity Review;",
		CanonicalExample: "alter workflow M.W { drop Review; };",
	},
	{
		Code:             AlterWorkflowReplaceActivity,
		Old:              "alter workflow M.W replace activity X with <activity>;",
		Canonical:        "alter workflow M.W { replace X with { <activity>; } }",
		Rewrite:          Rewrite{Structural: "`replace activity X with a;` as `replace X with { a; }`, inside the statement's { }"},
		RemovedIn:        2,
		Note:             alterWorkflowBlockNote,
		Example:          "alter workflow M.W replace activity Review with call microflow M.Notify;",
		CanonicalExample: "alter workflow M.W { replace Review with { call microflow M.Notify; } };",
	},
	{
		Code:             AlterWorkflowInsertOutcome,
		Old:              "alter workflow M.W insert outcome 'N' on X { … }",
		Canonical:        "alter workflow M.W { insert into X { outcomes 'N' { … } } }",
		Rewrite:          Rewrite{Structural: "`insert outcome 'N' on X { … }` as `insert into X { outcomes 'N' { … } }`"},
		RemovedIn:        2,
		Note:             "The fragment is the user task's own `outcomes` clause, as `create workflow` writes it. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W insert outcome 'Escalate' on Review { call microflow M.Notify; };",
		CanonicalExample: "alter workflow M.W { insert into Review { outcomes 'Escalate' { call microflow M.Notify; } } };",
	},
	{
		Code:             AlterWorkflowInsertPath,
		Old:              "alter workflow M.W insert path on X { … }",
		Canonical:        "alter workflow M.W { insert into X { path { … } } }",
		Rewrite:          Rewrite{Structural: "`insert path on X { … }` as `insert into X { path { … } }`"},
		RemovedIn:        2,
		Note:             "The fragment is the split's own `path n { … }`; the number may be left out, and when written it must be the next one. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W insert path on split1 { call microflow M.Notify; };",
		CanonicalExample: "alter workflow M.W { insert into split1 { path { call microflow M.Notify; } } };",
	},
	{
		Code:             AlterWorkflowInsertCondition,
		Old:              "alter workflow M.W insert condition 'V' on X { … }",
		Canonical:        "alter workflow M.W { insert into X { outcomes 'V' -> { … } } }",
		Rewrite:          Rewrite{Structural: "`insert condition 'V' on X { … }` as `insert into X { outcomes 'V' -> { … } }`"},
		RemovedIn:        2,
		Note:             "The fragment is the decision's own `outcomes` clause, as `create workflow` writes it (`true -> { … }`, `default -> { … }`). " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W insert condition 'true' on decision1 { };",
		CanonicalExample: "alter workflow M.W { insert into decision1 { outcomes 'true' -> { } } };",
	},
	{
		Code:             AlterWorkflowInsertBoundaryEvent,
		Old:              "alter workflow M.W insert boundary event on X <event>",
		Canonical:        "alter workflow M.W { insert into X { boundary event <event> } }",
		Rewrite:          Rewrite{Structural: "`insert boundary event on X e` as `insert into X { boundary event e }`"},
		RemovedIn:        2,
		Note:             "The fragment is the activity's own `boundary event` clause. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W insert boundary event on Review interrupting timer addHours([%CurrentDateTime%], 1) { };",
		CanonicalExample: "alter workflow M.W { insert into Review { boundary event interrupting timer addHours([%CurrentDateTime%], 1) { } } };",
	},
	{
		Code:      AlterWorkflowDropMember,
		Old:       "alter workflow M.W drop outcome 'N' on X / drop condition 'V' on X / drop path 'Path n' on X / drop boundary event on X",
		Canonical: "alter workflow M.W { drop X outcome 'N'; drop X outcome true; drop X path n; drop X boundary event; }",
		Rewrite:   Rewrite{Structural: "member after the activity it belongs to: `drop X outcome 'N'`, `drop X path n`, `drop X boundary event`"},
		RemovedIn: 2,
		Note: "A decision's `true`, `false` and `default` outcomes are written as those words; any other " +
			"outcome by its value. `drop path 'x'` whose caption is not `Path n` has no rewrite. " + alterWorkflowBlockNote,
		Example:          "alter workflow M.W drop outcome 'Reject' on Review;",
		CanonicalExample: "alter workflow M.W { drop Review outcome 'Reject'; };",
	},
}

func init() {
	entries = append(entries, alterWorkflowEntries...)
}
