// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#981 — view-entity OQL that check passed and mxbuild or the
// runtime refused. Every failing query below is one measured on mxbuild
// 11.13.0 (and, for MDL036–038, `run --local` on HSQLDB and PostgreSQL); every
// control is a form measured to build and run. The failing forms come from
// the JTSBootLogboek project (H16, H18) over
//
//	MyFirstModule.Race (Name: String(100), Season: Integer,
//	                    CurrentSeason: Integer, RaceDate: DateTime,
//	                    Points: Decimal, Nr: AutoNumber)
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

func selectRuleViolations(oql string, rule string) []linter.Violation {
	var out []linter.Violation
	all := append(ValidateOQLSyntax(oql), ValidateOQLPortability(oql)...)
	for _, v := range all {
		if v.RuleID == rule {
			out = append(out, v)
		}
	}
	return out
}

type oqlRuleCase struct {
	name  string
	oql   string
	fires bool
}

func runOQLRuleCases(t *testing.T, rule string, cases []oqlRuleCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selectRuleViolations(c.oql, rule)
			if (len(got) > 0) != c.fires {
				t.Fatalf("%s fired=%v, want %v\n  oql: %s\n  got: %v", rule, len(got) > 0, c.fires, c.oql, got)
			}
		})
	}
}

// MDL033 — a comparison as a select column is CE0174 ("The '=' part is
// incomplete or incorrect. You could use here: FROM."); `!=` likewise.
func TestMDL033_ComparisonAsSelectColumn(t *testing.T) {
	runOQLRuleCases(t, "MDL033", []oqlRuleCase{
		{"= as a column (CE0174)",
			`select r.Season as Season, r.Season = r.CurrentSeason as IsCurrent from MyFirstModule.Race as r`, true},
		{"!= as a column (CE0174)",
			`select r.Season != r.CurrentSeason as IsCurrent from MyFirstModule.Race as r`, true},
		// Controls, both build at 0 errors.
		{"the same comparison inside CASE",
			`select r.Season as Season, case when r.Season = r.CurrentSeason then true else false end as IsCurrent from MyFirstModule.Race as r`, false},
		{"a comparison in WHERE",
			`select r.Name as Nm from MyFirstModule.Race as r where r.Season > 1 and r.Name != 'x'`, false},
		{"a comparison inside a function argument or subquery",
			`select (select count(x.Name) from MyFirstModule.Race as x where x.Season = r.Season) as N from MyFirstModule.Race as r`, false},
		{"an operator inside a string literal",
			`select 'a=b' as L from MyFirstModule.Race as r`, false},
	})
	v := selectRuleViolations(`select r.Season = r.CurrentSeason as IsCurrent from MyFirstModule.Race as r`, "MDL033")[0]
	if !strings.Contains(v.Suggestion, "case when r.Season = r.CurrentSeason then true else false end as IsCurrent") {
		t.Errorf("the suggestion should spell out the CASE form: %s", v.Suggestion)
	}
}

// MDL034 — an aggregate over a column the GROUP BY also uses is CE0174.
func TestMDL034_AggregateOverAGroupedColumn(t *testing.T) {
	runOQLRuleCases(t, "MDL034", []oqlRuleCase{
		{"count(r.Season) by r.Season (CE0174)",
			`select r.Season as Season, count(r.Season) as N from MyFirstModule.Race as r group by r.Season`, true},
		{"max(r.RaceDate) by datepart(YEAR, r.RaceDate) (CE0174)",
			`select datepart(YEAR, r.RaceDate) as Yr, max(r.RaceDate) as LastDate from MyFirstModule.Race as r group by datepart(YEAR, r.RaceDate)`, true},
		{"sum(r.Season) by r.Season + 1 (CE0174)",
			`select r.Season + 1 as S, sum(r.Season) as Total from MyFirstModule.Race as r group by r.Season + 1`, true},
		{"from-first clause order",
			`from MyFirstModule.Race as r group by r.Season select r.Season as Season, count(r.Season) as N`, true},
		// Controls: build at 0 errors.
		{"count(r.Name) by r.Season",
			`select r.Season as Season, count(r.Name) as N from MyFirstModule.Race as r group by r.Season`, false},
		{"count(r.Points) by r.Season, r.Name",
			`select r.Season as Season, r.Name as Nm, count(r.Points) as N from MyFirstModule.Race as r group by r.Season, r.Name`, false},
		{"no GROUP BY at all",
			`select count(r.Season) as N from MyFirstModule.Race as r`, false},
	})
}

// MDL035 — a plain column that is neither aggregated nor grouped is CE0174
// ("Every expression in the SELECT clause must be included in the GROUP BY
// clause"), whatever the GROUP BY holds — an expression, another column, or
// the ID.
func TestMDL035_PlainColumnNotGrouped(t *testing.T) {
	runOQLRuleCases(t, "MDL035", []oqlRuleCase{
		{"r.Name next to group by datepart(...) (CE0174)",
			`select datepart(YEAR, r.RaceDate) as Yr, r.Name as Nm from MyFirstModule.Race as r group by datepart(YEAR, r.RaceDate)`, true},
		{"r.Name next to group by r.Season (CE0174)",
			`select r.Season as Season, r.Name as Nm from MyFirstModule.Race as r group by r.Season`, true},
		{"r.Name next to group by r.ID (CE0174, no functional dependency)",
			`select r.Name as Nm, count(r.Season) as N from MyFirstModule.Race as r group by r.ID`, true},
		{"from-first clause order (CE0174)",
			`from MyFirstModule.Race as r group by r.Season select r.Season as Season, r.Name as Nm`, true},
		// Controls: build at 0 errors.
		{"every plain column grouped",
			`select r.Season as Season, r.Name as Nm, count(r.Points) as N from MyFirstModule.Race as r group by r.Season, r.Name`, false},
		{"group by the select alias",
			`select datepart(YEAR, r.RaceDate) as Yr, count(r.Name) as N from MyFirstModule.Race as r group by Yr`, false},
		{"a constant next to the group",
			`select 'x' as L, r.Season as Season from MyFirstModule.Race as r group by r.Season`, false},
		{"quoting and case differ from the GROUP BY",
			`select R."Season" as Season, count(r.Name) as N from MyFirstModule.Race as r GROUP  BY r.Season`, false},
		{"a UNION is not judged",
			`select r.Name as Nm from MyFirstModule.Race as r group by r.Season union all select r.Name as Nm from MyFirstModule.Race as r`, false},
	})
}

// MDL036 — a non-aggregated EXPRESSION that is not a GROUP BY expression
// passes mxbuild and fails when the view is read: PostgreSQL 42803, HSQLDB
// 42574.
func TestMDL036_ExpressionNotGrouped(t *testing.T) {
	runOQLRuleCases(t, "MDL036", []oqlRuleCase{
		{"datepart(MONTH) next to group by datepart(YEAR) (42803 / 42574)",
			`select datepart(YEAR, r.RaceDate) as Yr, datepart(MONTH, r.RaceDate) as Mo from MyFirstModule.Race as r group by datepart(YEAR, r.RaceDate)`, true},
		// Controls.
		{"the select expression equals the GROUP BY expression",
			`select datepart(YEAR, r.RaceDate) as Yr, count(r.Name) as N from MyFirstModule.Race as r group by datepart(YEAR, r.RaceDate)`, false},
		{"built from the GROUP BY expression (builds)",
			`select datepart(YEAR, r.RaceDate) + 1 as Y1, count(r.Name) as N from MyFirstModule.Race as r group by datepart(YEAR, r.RaceDate)`, false},
		{"a cast of the grouped ID (keywords stay words)",
			`from M.Reading as r join r/M.Reading_Meter/M.Meter as m group by m.ID select cast(m.ID as string) as MeterId, sum(r.Kwh) as TotalKwh`, false},
		{"a function of a grouped column",
			`select datepart(YEAR, r.RaceDate) as Yr, count(r.Name) as N from MyFirstModule.Race as r group by r.RaceDate`, false},
		{"a constant expression",
			`select 1 + 1 as Two, count(r.Name) as N from MyFirstModule.Race as r group by r.Season`, false},
	})
	// The plain-column case is MDL035, not both.
	if got := selectRuleViolations(`select r.Season as Season, r.Name as Nm from MyFirstModule.Race as r group by r.Season`, "MDL036"); len(got) != 0 {
		t.Errorf("a plain column is MDL035's; MDL036 fired too: %v", got)
	}
}

// MDL037 — a literal aggregate argument is sent untyped, and HSQLDB refuses
// it with 42567 "data type cast needed".
func TestMDL037_LiteralAggregateArgument(t *testing.T) {
	runOQLRuleCases(t, "MDL037", []oqlRuleCase{
		{"sum(1)", `select r.Season as Season, sum(1) as N from MyFirstModule.Race as r group by r.Season`, true},
		{"count(1)", `select count(1) as N from MyFirstModule.Race as r`, true},
		{"max(0)", `select max(0) as N from MyFirstModule.Race as r`, true},
		{"count('x')", `select count('x') as N from MyFirstModule.Race as r`, true},
		{"count(true)", `select count(true) as N from MyFirstModule.Race as r`, true},
		{"in a UNION branch", `select count(r.Name) as N from MyFirstModule.Race as r union all select sum(1) as N from MyFirstModule.Race as r`, true},
		// Controls: run on HSQLDB.
		{"count(a column)", `select count(r.Name) as N from MyFirstModule.Race as r`, false},
		{"sum(cast(1 as Integer))", `select sum(cast(1 as Integer)) as N from MyFirstModule.Race as r`, false},
		{"sum(case … then 1 else 0 end)", `select sum(case when r.Season > 0 then 1 else 0 end) as N from MyFirstModule.Race as r`, false},
		{"a decimal literal, which Mendix casts", `select sum(1.5) as N from MyFirstModule.Race as r`, false},
	})
	for _, v := range selectRuleViolations(`select count(1) as N from MyFirstModule.Race as r`, "MDL037") {
		if v.Severity != linter.SeverityWarning {
			t.Errorf("MDL037 severity = %v, want warning: PostgreSQL and mxbuild accept it", v.Severity)
		}
		if !strings.Contains(v.Suggestion, "cast(1 as Integer)") {
			t.Errorf("suggestion should give the cast form: %s", v.Suggestion)
		}
	}
	// It is not an exec refusal: exec turns every ValidateOQLSyntax finding
	// into one, so the HSQLDB rules must stay out of it.
	for _, v := range ValidateOQLSyntax(`select count(1) as N, 1 as One from MyFirstModule.Race as r`) {
		if v.RuleID == "MDL037" || v.RuleID == "MDL038" {
			t.Errorf("%s is reported by ValidateOQLSyntax, which exec refuses on", v.RuleID)
		}
	}
}

// MDL038 — a bare integer or string literal column is sent untyped. The view
// reads, but `v.One + 1` is 11 on HSQLDB (2 on PostgreSQL), and a consumer that
// aggregates the column fails.
func TestMDL038_BareLiteralColumnIsANoteOnly(t *testing.T) {
	runOQLRuleCases(t, "MDL038", []oqlRuleCase{
		{"1 as One", `select r.Name as Nm, 1 as One from MyFirstModule.Race as r`, true},
		{"'Schipper' as Rol", `select r.Name as Nm, 'Schipper' as Rol from MyFirstModule.Race as r`, true},
		// Controls.
		{"cast('Schipper' as String)", `select r.Name as Nm, cast('Schipper' as String) as Rol from MyFirstModule.Race as r`, false},
		{"true as B (typed)", `select r.Name as Nm, true as B from MyFirstModule.Race as r`, false},
		{"0.0 as D (cast by Mendix)", `select r.Name as Nm, 0.0 as D from MyFirstModule.Race as r`, false},
		{"case … then 1 else 0 end", `select r.Name as Nm, case when r.Season > 0 then 1 else 0 end as X from MyFirstModule.Race as r`, false},
	})
	// The 'TOTAL' label the docs use must never be more than a note: it is
	// how a summary row is written, and it runs.
	for _, v := range append(ValidateOQLSyntax(`select 'TOTAL' as Label, sum(r.Points) as Amt from MyFirstModule.Race as r`),
		ValidateOQLPortability(`select 'TOTAL' as Label, sum(r.Points) as Amt from MyFirstModule.Race as r`)...) {
		if v.Severity > linter.SeverityInfo {
			t.Errorf("'TOTAL' as Label drew %s %s: %s", v.Severity, v.RuleID, v.Message)
		}
	}
	v := selectRuleViolations(`select 1 as One from MyFirstModule.Race as r`, "MDL038")[0]
	if !strings.Contains(v.Message, "11") || !strings.Contains(v.Suggestion, "cast(1 as Integer) as One") {
		t.Errorf("the note should name the HSQLDB result and the cast: %s / %s", v.Message, v.Suggestion)
	}
}
