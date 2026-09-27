// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// List operations and aggregates are statements that mirror Studio Pro's
// activities (PROPOSAL_mdl_beta_syntax_freeze.md §4, ADR-0010 R2; #733).

const listActivityParams = "$Orders: List of M.Order, $A: List of M.Order, $B: List of M.Order, " +
	"$Order: M.Order, $Number: Integer, $S: String"

// listActivityMicroflow wraps body in a microflow whose parameters every case
// below can name.
func listActivityMicroflow(header, body string) string {
	return header + "create microflow M.F (" + listActivityParams + ")\nbegin\n  " + body + "\nend;"
}

// buildListActivity parses one statement inside a microflow and returns the
// program and the microflow's first body statement.
func buildListActivity(t *testing.T, header, body string) (*ast.Program, ast.MicroflowStatement, []error) {
	t.Helper()
	prog, errs := Build(listActivityMicroflow(header, body))
	if prog == nil || len(prog.Statements) == 0 {
		return prog, nil, errs
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok || len(mf.Body) == 0 {
		return prog, nil, errs
	}
	return prog, mf.Body[0], errs
}

func noteCodes(prog *ast.Program) []string {
	var out []string
	for _, n := range prog.LanguageNotes {
		out = append(out, n.Code)
	}
	return out
}

// Every row of the §4 table parses under both versions and builds the activity
// it names, with no deprecation and no language note.
func TestListActivityStatementsBuild(t *testing.T) {
	cases := []struct {
		src   string
		check func(t *testing.T, s ast.MicroflowStatement)
	}{
		{"$Open = filter $Orders by Status = M.Status.Open;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpFilter || lo.InputVariable != "Orders" || lo.ByExpression || !ast.IsMemberEquality(lo.Condition) {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Big = filter $Orders where $currentObject/Total > 1000;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpFilter || !lo.ByExpression || lo.Condition == nil {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Match = find $Orders by Number = $Number;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpFind || lo.ByExpression || lo.OutputVariable != "Match" {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Late = find $Orders where $currentObject/Number < 3;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpFind || !lo.ByExpression {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Sorted = sort $Orders by Date desc, Number asc, Count;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			want := []ast.SortSpec{{Attribute: "Date"}, {Attribute: "Number", Ascending: true}, {Attribute: "Count", Ascending: true}}
			if lo.Operation != ast.ListOpSort || !reflect.DeepEqual(lo.SortSpecs, want) {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$First = head $Orders;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpHead || lo.InputVariable != "Orders" {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Rest = tail $Orders;", func(t *testing.T, s ast.MicroflowStatement) {
			if lo := s.(*ast.ListOperationStmt); lo.Operation != ast.ListOpTail {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$Page = range $Orders offset 20 limit 10;", func(t *testing.T, s ast.MicroflowStatement) {
			lo := s.(*ast.ListOperationStmt)
			if lo.Operation != ast.ListOpRange || lo.OffsetExpr == nil || lo.LimitExpr == nil {
				t.Errorf("got %#v", lo)
			}
		}},
		{"$All = union $A with $B;", listPair(ast.ListOpUnion, "A", "B")},
		{"$Both = intersect $A with $B;", listPair(ast.ListOpIntersect, "A", "B")},
		// subtract $B from $A is A minus B: the first list is A.
		{"$Left = subtract $B from $A;", listPair(ast.ListOpSubtract, "A", "B")},
		// contains $Order in $Orders: the list is the first operand.
		{"$Has = contains $Order in $Orders;", listPair(ast.ListOpContains, "Orders", "Order")},
		{"$Same = equals $A and $B;", listPair(ast.ListOpEquals, "A", "B")},
		{"$N = count $Orders;", func(t *testing.T, s ast.MicroflowStatement) {
			ag := s.(*ast.AggregateListStmt)
			if ag.Operation != ast.AggregateCount || ag.InputVariable != "Orders" {
				t.Errorf("got %#v", ag)
			}
		}},
		{"$Total = sum $Orders by Amount;", func(t *testing.T, s ast.MicroflowStatement) {
			ag := s.(*ast.AggregateListStmt)
			if ag.Operation != ast.AggregateSum || ag.Attribute != "Amount" || ag.IsExpression || ag.InputVariable != "Orders" {
				t.Errorf("got %#v", ag)
			}
		}},
		{"$Total = sum $Orders of $currentObject/Price * $currentObject/Quantity;", func(t *testing.T, s ast.MicroflowStatement) {
			ag := s.(*ast.AggregateListStmt)
			if ag.Operation != ast.AggregateSum || !ag.IsExpression || ag.Expression == nil {
				t.Errorf("got %#v", ag)
			}
		}},
		{"$Avg = average $Orders by Amount;", aggregateOp(ast.AggregateAverage)},
		{"$Min = minimum $Orders by Amount;", aggregateOp(ast.AggregateMinimum)},
		{"$Max = maximum $Orders of $currentObject/Amount;", aggregateOp(ast.AggregateMaximum)},
		{"$AllPaid = all $Orders where $currentObject/Paid;", aggregateOp(ast.AggregateAll)},
		{"$AnyLate = any $Orders where $currentObject/Late;", aggregateOp(ast.AggregateAny)},
		{"$Csv = reduce $Orders from '' as String using $currentResult + $currentObject/Name;", func(t *testing.T, s ast.MicroflowStatement) {
			ag := s.(*ast.AggregateListStmt)
			if ag.Operation != ast.AggregateReduce || ag.InitialValue == nil || ag.ReturnType == nil ||
				ag.ReturnType.Kind != ast.TypeString || ag.Expression == nil {
				t.Errorf("got %#v", ag)
			}
		}},
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		for _, tc := range cases {
			t.Run(strings.TrimSpace(header+" "+tc.src), func(t *testing.T) {
				prog, stmt, errs := buildListActivity(t, header, tc.src)
				if len(errs) > 0 {
					t.Fatalf("parse: %v", errs)
				}
				if stmt == nil {
					t.Fatal("no statement built")
				}
				tc.check(t, stmt)
				if d := deprecationCodes(prog); len(d) != 0 {
					t.Errorf("canonical form recorded deprecations %v", d)
				}
				if n := noteCodes(prog); len(n) != 0 {
					t.Errorf("canonical form recorded language notes %v", n)
				}
			})
		}
	}
}

func listPair(op ast.ListOperationType, first, second string) func(*testing.T, ast.MicroflowStatement) {
	return func(t *testing.T, s ast.MicroflowStatement) {
		lo := s.(*ast.ListOperationStmt)
		if lo.Operation != op || lo.InputVariable != first || lo.SecondVariable != second {
			t.Errorf("got %#v, want %v(%s, %s)", lo, op, first, second)
		}
	}
}

func aggregateOp(op ast.AggregateListOperationType) func(*testing.T, ast.MicroflowStatement) {
	return func(t *testing.T, s ast.MicroflowStatement) {
		ag := s.(*ast.AggregateListStmt)
		if ag.Operation != op || ag.InputVariable != "Orders" {
			t.Errorf("got %#v", ag)
		}
	}
}

// The function form is a deprecated respelling of the statement: both build the
// same AST, and only the function form records the registry code. find and
// contains are not here — they clash with the string functions (see
// TestFindContainsFunctionFormIsVersionGated).
func TestListFunctionFormIsAnAlias(t *testing.T) {
	cases := []struct{ old, canon, code string }{
		{"$H = head($Orders);", "$H = head $Orders;", deprecation.ListOperationFunctionForm},
		{"$T = tail($Orders);", "$T = tail $Orders;", deprecation.ListOperationFunctionForm},
		{"$F = filter($Orders, Status = M.Status.Open);", "$F = filter $Orders by Status = M.Status.Open;", deprecation.ListOperationFunctionForm},
		{"$F = filter($Orders, $currentObject/Total > 10);", "$F = filter $Orders where $currentObject/Total > 10;", deprecation.ListOperationFunctionForm},
		{"$F = sort($Orders, OrderDate desc, Number);", "$F = sort $Orders by OrderDate desc, Number;", deprecation.ListOperationFunctionForm},
		{"$U = union($A, $B);", "$U = union $A with $B;", deprecation.ListOperationFunctionForm},
		{"$U = intersect($A, $B);", "$U = intersect $A with $B;", deprecation.ListOperationFunctionForm},
		{"$U = subtract($A, $B);", "$U = subtract $B from $A;", deprecation.ListOperationFunctionForm},
		{"$E = equals($A, $B);", "$E = equals $A and $B;", deprecation.ListOperationFunctionForm},
		{"$R = range($Orders, 20, 10);", "$R = range $Orders offset 20 limit 10;", deprecation.ListOperationFunctionForm},
		{"$N = count($Orders);", "$N = count $Orders;", deprecation.AggregateFunctionForm},
		{"$N = sum($Orders.Amount);", "$N = sum $Orders by Amount;", deprecation.AggregateFunctionForm},
		{"$N = average($Orders, $currentObject/Amount * 2);", "$N = average $Orders of $currentObject/Amount * 2;", deprecation.AggregateFunctionForm},
		{"$N = all($Orders, $currentObject/Paid);", "$N = all $Orders where $currentObject/Paid;", deprecation.AggregateFunctionForm},
		{"$N = reduce($Orders, $currentResult + 1, initial: 0, returns: Integer);", "$N = reduce $Orders from 0 as Integer using $currentResult + 1;", deprecation.AggregateFunctionForm},
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		for _, tc := range cases {
			t.Run(strings.TrimSpace(header+" "+tc.old), func(t *testing.T) {
				oldProg, oldStmt, errs := buildListActivity(t, header, tc.old)
				if len(errs) > 0 {
					t.Fatalf("old form: %v", errs)
				}
				canonProg, canonStmt, errs := buildListActivity(t, header, tc.canon)
				if len(errs) > 0 {
					t.Fatalf("canonical form: %v", errs)
				}
				if !reflect.DeepEqual(oldStmt, canonStmt) {
					t.Errorf("forms build different statements:\n old:   %#v\n canon: %#v", oldStmt, canonStmt)
				}
				if got := deprecationCodes(oldProg); !reflect.DeepEqual(got, []string{tc.code}) {
					t.Errorf("old form recorded %v, want [%s]", got, tc.code)
				}
				if got := deprecationCodes(canonProg); len(got) != 0 {
					t.Errorf("canonical form recorded %v", got)
				}
				if n := noteCodes(oldProg); len(n) != 0 {
					t.Errorf("an alias is not a change of meaning, but recorded notes %v", n)
				}
			})
		}
	}
}

// `$x = find(a, b)` and `$x = contains(a, b)` are either the list operation or
// the string function, decided today by what the arguments look like. Under
// mdl 0 that meaning is kept and warned; under mdl 1 the form is refused.
func TestFindContainsFunctionFormIsVersionGated(t *testing.T) {
	for _, src := range []string{
		"$M = find($Orders, Number = $Number);",
		"$M = contains($Orders, $Order);",
	} {
		t.Run("mdl 0 "+src, func(t *testing.T) {
			prog, stmt, errs := buildListActivity(t, "", src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			if _, ok := stmt.(*ast.ListOperationStmt); !ok {
				t.Errorf("mdl 0 must keep the list operation, got %T", stmt)
			}
			if got := noteCodes(prog); !reflect.DeepEqual(got, []string{listCallForm.Code}) {
				t.Errorf("notes = %v, want [%s]", got, listCallForm.Code)
			}
			if d := deprecationCodes(prog); len(d) != 0 {
				t.Errorf("find/contains are not aliases, but recorded %v", d)
			}
		})
		t.Run("mdl 1 "+src, func(t *testing.T) {
			_, _, errs := buildListActivity(t, "mdl 1;\n", src)
			if len(errs) == 0 {
				t.Fatal("accepted under mdl 1")
			}
			if msg := errs[0].Error(); !strings.Contains(msg, "set $M =") || !strings.Contains(msg, "List operation") {
				t.Errorf("the error should name both readings: %v", msg)
			}
		})
	}
}

// Under mdl 1 `set` always assigns an expression, so `set $x = find(…)` is the
// string function whatever its arguments look like.
func TestSetFindIsTheStringFunctionUnderMdl1(t *testing.T) {
	body := "declare $I Integer = 0;\n  set $I = find($S, $S);"
	prog, errs := Build(listActivityMicroflow("mdl 1;\n", body))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[1]
	if _, ok := stmt.(*ast.MfSetStmt); !ok {
		t.Errorf("mdl 1: set $I = find($S, $S) built %T, want *ast.MfSetStmt", stmt)
	}

	// Control: under mdl 0 the same text keeps today's list-operation reading
	// and warns that its meaning differs under mdl 1.
	prog, errs = Build(listActivityMicroflow("", body))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[1].(*ast.ListOperationStmt); !ok {
		t.Errorf("mdl 0 must keep the list operation reading")
	}
	if got := noteCodes(prog); !reflect.DeepEqual(got, []string{listCallForm.Code}) {
		t.Errorf("notes = %v, want [%s]", got, listCallForm.Code)
	}
}

// Nesting one activity inside another cannot be written under mdl 1; under
// mdl 0 it keeps parsing (MDL-LISTOP02 refuses it at check time) and warns.
func TestNestedListOperationRefusedUnderMdl1(t *testing.T) {
	for _, src := range []string{
		"$N = count(filter($Orders, Status = M.Status.Open));",
		"set $N = count(filter($Orders, Status = M.Status.Open));",
		"set $H = head($Orders);",
	} {
		t.Run("mdl 1 "+src, func(t *testing.T) {
			_, _, errs := buildListActivity(t, "mdl 1;\n", src)
			if len(errs) == 0 {
				t.Fatal("accepted under mdl 1")
			}
			if msg := errs[0].Error(); !strings.Contains(msg, "one statement per activity") {
				t.Errorf("error should explain the statement form: %v", msg)
			}
		})
		t.Run("mdl 0 "+src, func(t *testing.T) {
			prog, _, errs := buildListActivity(t, "", src)
			if len(errs) > 0 {
				t.Fatalf("mdl 0 must keep parsing: %v", errs)
			}
			if got := noteCodes(prog); !reflect.DeepEqual(got, []string{listCallForm.Code}) {
				t.Errorf("notes = %v, want [%s]", got, listCallForm.Code)
			}
		})
	}
}

// `set` is mandatory for reassignment under mdl 1.
func TestSetIsMandatoryUnderMdl1(t *testing.T) {
	body := "declare $I Integer = 0;\n  $I = 5;"
	_, errs := Build(listActivityMicroflow("mdl 1;\n", body))
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "set $I = 5") {
		t.Fatalf("mdl 1 must refuse a reassignment without set, got %v", errs)
	}

	prog, errs := Build(listActivityMicroflow("", body))
	if len(errs) > 0 {
		t.Fatalf("mdl 0 must keep parsing: %v", errs)
	}
	if got := noteCodes(prog); !reflect.DeepEqual(got, []string{setIsMandatory.Code}) {
		t.Errorf("notes = %v, want [%s]", got, setIsMandatory.Code)
	}

	// Control: with `set`, neither version says anything.
	for _, header := range []string{"", "mdl 1;\n"} {
		prog, errs := Build(listActivityMicroflow(header, "declare $I Integer = 0;\n  set $I = 5;"))
		if len(errs) > 0 || len(prog.LanguageNotes) > 0 {
			t.Errorf("%q: set $I = 5 gave errors %v, notes %v", header, errs, prog.LanguageNotes)
		}
	}
	// An attribute target is `change`'s territory (plan item 3.x), not this rule.
	prog, errs = Build(listActivityMicroflow("mdl 1;\n", "$Order/Number = 5;"))
	if len(errs) > 0 || len(prog.LanguageNotes) > 0 {
		t.Errorf("$Order/Number = 5 gave errors %v, notes %v", errs, prog.LanguageNotes)
	}
}

// `by` picks a member: anything but `Member = value` belongs after `where`.
func TestFilterByNeedsMemberEquality(t *testing.T) {
	_, _, errs := buildListActivity(t, "", "$F = filter $Orders by $currentObject/Total > 3;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "where") {
		t.Fatalf("want an error pointing at `where`, got %v", errs)
	}
}
