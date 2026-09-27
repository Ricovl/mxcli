// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// describe prints a List operation / Aggregate list activity as the statement
// that mirrors it when it describes under mdl 1, and keeps the call form while
// it describes under mdl 0 — which, while mdl 1 is a preview, is every describe
// outside an `mdl 1;` script (#733).
func TestDescribeListActivityUnderMdl1(t *testing.T) {
	cases := []struct {
		action   microflows.MicroflowAction
		mdl1     string
		mdl0     string
		wantKind string // the statement type the mdl 1 output parses back to
	}{
		{listOp(&microflows.HeadOperation{ListVariable: "Orders"}, "First"),
			"$First = head $Orders;", "$First = head($Orders);", "list"},
		{listOp(&microflows.TailOperation{ListVariable: "Orders"}, "Rest"),
			"$Rest = tail $Orders;", "$Rest = tail($Orders);", "list"},
		{listOp(&microflows.FilterOperation{ListVariable: "Orders", Expression: "$currentObject/Total > 1000"}, "Big"),
			"$Big = filter $Orders where $currentObject/Total > 1000;", "$Big = filter($Orders, $currentObject/Total > 1000);", "list"},
		{listOp(&microflows.FindOperation{ListVariable: "Orders", Expression: "$currentObject/Number < 3"}, "Late"),
			"$Late = find $Orders where $currentObject/Number < 3;", "$Late = find($Orders, $currentObject/Number < 3);", "list"},
		{listOp(&microflows.FilterByAttributeOperation{ListVariable: "Orders", Attribute: "M.Order.Status", Expression: "M.Status.Open"}, "Open"),
			"$Open = filter $Orders by Status = M.Status.Open;", "$Open = filter($Orders, Status = M.Status.Open);", "list"},
		{listOp(&microflows.FindByAttributeOperation{ListVariable: "Orders", Association: "M.Order_Customer", Expression: "$Customer"}, "Match"),
			"$Match = find $Orders by Order_Customer = $Customer;", "$Match = find($Orders, Order_Customer = $Customer);", "list"},
		{listOp(&microflows.SortOperation{ListVariable: "Orders", Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "M.Order.Date", Direction: microflows.SortDirectionDescending},
			{AttributeQualifiedName: "M.Order.Number", Direction: microflows.SortDirectionAscending},
		}}, "Sorted"),
			`$Sorted = sort $Orders by "Date" desc, Number asc;`, `$Sorted = sort($Orders, "Date" desc, Number asc);`, "list"},
		{listOp(&microflows.ListRangeOperation{ListVariable: "Orders", OffsetExpression: "20", LimitExpression: "10"}, "Page"),
			"$Page = range $Orders offset 20 limit 10;", "$Page = range($Orders, 20, 10);", "list"},
		{listOp(&microflows.ListRangeOperation{ListVariable: "Orders", LimitExpression: "10"}, "Page"),
			"$Page = range $Orders limit 10;", "$Page = range($Orders, 0, 10);", "list"},
		{listOp(&microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"}, "All"),
			"$All = union $A with $B;", "$All = union($A, $B);", "list"},
		{listOp(&microflows.IntersectOperation{ListVariable1: "A", ListVariable2: "B"}, "Both"),
			"$Both = intersect $A with $B;", "$Both = intersect($A, $B);", "list"},
		{listOp(&microflows.SubtractOperation{ListVariable1: "A", ListVariable2: "B"}, "Left"),
			"$Left = subtract $B from $A;", "$Left = subtract($A, $B);", "list"},
		{listOp(&microflows.ContainsOperation{ListVariable: "Orders", ObjectVariable: "Order"}, "Has"),
			"$Has = contains $Order in $Orders;", "$Has = contains($Orders, $Order);", "list"},
		{listOp(&microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"}, "Same"),
			"$Same = equals $A and $B;", "$Same = equals($A, $B);", "list"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "N", Function: microflows.AggregateFunctionCount},
			"$N = count $Orders;", "$N = count($Orders);", "aggregate"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "Total", Function: microflows.AggregateFunctionSum, AttributeQualifiedName: "M.Order.Amount"},
			"$Total = sum $Orders by Amount;", "$Total = sum($Orders.Amount);", "aggregate"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "Total", Function: microflows.AggregateFunctionAverage, UseExpression: true, Expression: "$currentObject/Price * 2"},
			"$Total = average $Orders of $currentObject/Price * 2;", "$Total = average($Orders, $currentObject/Price * 2);", "aggregate"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "Paid", Function: microflows.AggregateFunctionAll, UseExpression: true, Expression: "$currentObject/Paid"},
			"$Paid = all $Orders where $currentObject/Paid;", "$Paid = all($Orders, $currentObject/Paid);", "aggregate"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "Late", Function: microflows.AggregateFunctionAny, UseExpression: true, Expression: "$currentObject/Late"},
			"$Late = any $Orders where $currentObject/Late;", "$Late = any($Orders, $currentObject/Late);", "aggregate"},
		{&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "Csv", Function: microflows.AggregateFunctionReduce, UseExpression: true,
			Expression: "$currentResult + $currentObject/Name", ReduceInitialValue: "''", ReduceReturnType: &microflows.StringType{}},
			"$Csv = reduce $Orders from '' as String using $currentResult + $currentObject/Name;",
			"$Csv = reduce($Orders, $currentResult + $currentObject/Name, initial: '', returns: String);", "aggregate"},
	}
	for _, tc := range cases {
		t.Run(tc.mdl1, func(t *testing.T) {
			if got := formatAction(&ExecContext{LanguageVersion: langver.V1}, tc.action, nil, nil); got != tc.mdl1 {
				t.Errorf("mdl 1 describe:\n got  %s\n want %s", got, tc.mdl1)
			}
			// Control: outside an mdl 1 script describe keeps the call form.
			for _, ctx := range []*ExecContext{nil, {LanguageVersion: langver.V0}} {
				if got := formatAction(ctx, tc.action, nil, nil); got != tc.mdl0 {
					t.Errorf("mdl 0 describe:\n got  %s\n want %s", got, tc.mdl0)
				}
			}

			// The mdl 1 output parses back under the header, with nothing to warn about.
			src := "mdl 1;\ncreate microflow M.A ($Orders: List of M.Order, $A: List of M.Order, $B: List of M.Order, " +
				"$Order: M.Order, $Customer: M.Customer)\nbegin\n  " + tc.mdl1 + "\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("the mdl 1 describe output does not parse: %v", errs)
			}
			if len(prog.Deprecations) > 0 || len(prog.LanguageNotes) > 0 {
				t.Errorf("the mdl 1 describe output warns: %v %v", prog.Deprecations, prog.LanguageNotes)
			}
		})
	}
}

func listOp(op microflows.ListOperation, out string) *microflows.ListOperationAction {
	return &microflows.ListOperationAction{Operation: op, OutputVariable: out}
}

// The describe language is the newest frozen version unless the describe runs
// inside a script whose header names a newer one.
func TestDescribeLanguage(t *testing.T) {
	if got := describeLanguage(nil); got != langver.Frozen {
		t.Errorf("describeLanguage(nil) = %v, want the frozen %v", got, langver.Frozen)
	}
	if got := describeLanguage(&ExecContext{LanguageVersion: langver.V1}); got != langver.V1 {
		t.Errorf("under mdl 1 = %v", got)
	}
}
