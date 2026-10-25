// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1650 (upstream mendixlabs/mxcli#1641): STRING_AGG is an OQL
// aggregate, and the GROUP BY rules did not know it — every view using it was
// MDL036 "not aggregated", refused by check, exec and exec --no-check alike.
package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// stringAggView is the issue's own view, over
//
//	MyFirstModule.Sale (Name: String(100), Amount: Decimal)
//	MyFirstModule.Booking (Code: String(20))
//	MyFirstModule.Sale_Booking from Sale to Booking
func stringAggView(column string) string {
	return `select b.Code as Code, ` + column + ` as Names
  from MyFirstModule.Sale as s
  join s/MyFirstModule.Sale_Booking/MyFirstModule.Booking as b
  group by b.Code`
}

// The grouping rules count every OQL aggregate as one. Each "fires: false"
// case below builds at 0 errors on mxbuild (see the version notes on
// oqlAggregateFunctions); each "fires: true" one is the rule still doing its
// job next to a STRING_AGG.
func TestGroupByRules_StringAggIsAnAggregate(t *testing.T) {
	for _, rule := range []string{"MDL035", "MDL036"} {
		runOQLRuleCases(t, rule, []oqlRuleCase{
			{"the issue's view", stringAggView(`STRING_AGG(s.Name, '; ')`), false},
			{"lower case", stringAggView(`string_agg(s.Name, ', ')`), false},
			{"a space before the parenthesis", stringAggView(`STRING_AGG (s.Name, ', ')`), false},
			{"a CASE as the argument", stringAggView(`STRING_AGG(CASE WHEN s.Amount > 0 THEN s.Name ELSE '-' END, '; ')`), false},
			{"from-first clause order",
				`from MyFirstModule.Sale as s join s/MyFirstModule.Sale_Booking/MyFirstModule.Booking as b group by b.Code select b.Code as Code, string_agg(s.Name, ';') as Names`, false},
		})
	}
	// Controls: a STRING_AGG elsewhere in the select list does not excuse a
	// column that is not aggregated.
	runOQLRuleCases(t, "MDL036", []oqlRuleCase{
		{"an ungrouped expression beside a STRING_AGG",
			`select b.Code as Code, length(s.Name) as L, string_agg(s.Name, ';') as Names
  from MyFirstModule.Sale as s join s/MyFirstModule.Sale_Booking/MyFirstModule.Booking as b group by b.Code`, true},
	})
	runOQLRuleCases(t, "MDL035", []oqlRuleCase{
		{"an ungrouped column beside a STRING_AGG",
			`select b.Code as Code, s.Name as Nm, string_agg(s.Name, ';') as Names
  from MyFirstModule.Sale as s join s/MyFirstModule.Sale_Booking/MyFirstModule.Booking as b group by b.Code`, true},
	})
}

// A STRING_AGG over a grouped column is the same CE0174 as count(r.Season)
// by r.Season (MDL034).
func TestMDL034_StringAggOverAGroupedColumn(t *testing.T) {
	runOQLRuleCases(t, "MDL034", []oqlRuleCase{
		{"string_agg(b.Code) by b.Code", stringAggView(`string_agg(b.Code, ';')`), true},
		{"string_agg(s.Name) by b.Code (control)", stringAggView(`string_agg(s.Name, ';')`), false},
	})
}

// The separator is a string literal, and Mendix requires one there: it is not
// the untyped aggregate ARGUMENT MDL037 is about.
func TestMDL037_StringAggSeparatorIsNotALiteralArgument(t *testing.T) {
	if got := selectRuleViolations(stringAggView(`string_agg(s.Name, '; ')`), "MDL037"); len(got) != 0 {
		t.Errorf("the separator literal drew MDL037: %v", got)
	}
}

func stringAggFixture(t *testing.T, version ...[3]int) (*ExecContext, *[]*domainmodel.Entity) {
	t.Helper()
	mod := mkModule("MyFirstModule")
	sale := mkEntity(mod.ID, "Sale")
	sale.Attributes = []*domainmodel.Attribute{
		{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 100}},
		{Name: "Amount", Type: &domainmodel.DecimalAttributeType{}},
	}
	booking := mkEntity(mod.ID, "Booking")
	booking.Attributes = []*domainmodel.Attribute{
		{Name: "Code", Type: &domainmodel.StringAttributeType{Length: 20}},
	}
	memo := mkEntity(mod.ID, "Memo")
	memo.Attributes = []*domainmodel.Attribute{
		{Name: "Body", Type: &domainmodel.StringAttributeType{}},
		{Name: "Brief", Type: &domainmodel.StringAttributeType{Length: 500}},
	}
	dm := mkDomainModel(mod.ID, sale, booking, memo)
	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			return nil
		},
	}
	if len(version) == 1 {
		v := version[0]
		mb.ProjectVersionFunc = func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: v[0], MinorVersion: v[1], PatchVersion: v[2],
				ProductVersion: fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])}
		}
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, &created
}

// The issue's step2.mdl reached the handler under `exec --no-check` and was
// refused there with "invalid OQL in view entity" — the handler runs
// ValidateOQLSyntax itself, so the pre-flight was not the only barrier.
func TestExecCreateViewEntity_WritesAStringAggView(t *testing.T) {
	ctx, created := stringAggFixture(t)
	s := parseViewStmt(t, `create view entity MyFirstModule.BookingNames (Code: String(20), Names: String(200)) as (
  SELECT b.Code AS Code, STRING_AGG(s.Name, '; ') AS Names
  FROM MyFirstModule.Sale AS s
  JOIN s/MyFirstModule.Sale_Booking/MyFirstModule.Booking AS b
  GROUP BY b.Code
);`)
	if err := execCreateViewEntity(ctx, s); err != nil {
		t.Fatalf("exec refused the view: %v", err)
	}
	if len(*created) != 1 || (*created)[0].Name != "BookingNames" {
		t.Fatalf("want BookingNames written, got %d entities", len(*created))
	}
	if err := strings.TrimSpace((*created)[0].OqlQuery); err == "" {
		t.Error("the view was written without its query")
	}
}

// The type of a STRING_AGG column is version-dependent (stringAggType), and
// every row below is an mx check result over the attribute shown:
//
//	from 11.7.0 and on 10.24.22+   String(200), whatever it aggregates
//	11.2.0 – 11.6.12               the argument's own type, exactly
//
// So the issue's `Names: String` (unlimited) over Sale.Name, a String(100), is
// CE6770 "View Entity is out of sync with the OQL Query" on every version that
// has STRING_AGG, and check says so before the write — with the fix for that
// project's version.
func TestViewTypes_StringAggColumnTypeFollowsTheVersion(t *testing.T) {
	cases := []struct {
		v        [3]int
		column   string // over Sale.Name: String(100), Memo.Body: String(unlimited), Memo.Brief: String(500)
		declared string
		fix      string // "" = builds
	}{
		// 11.7.0+: a derived String(200) (measured 11.7.0, 11.10.0, 11.12.2, 11.13.0, 11.15.0).
		{[3]int{11, 13, 0}, "STRING_AGG(s.Name, '; ')", "String", "Names: String(200)"},
		{[3]int{11, 13, 0}, "STRING_AGG(s.Name, '; ')", "String(100)", "Names: String(200)"},
		{[3]int{11, 13, 0}, "STRING_AGG(s.Name, '; ')", "String(200)", ""},
		{[3]int{11, 7, 0}, "string_agg(m.Body, ',')", "String", "Names: String(200)"},
		{[3]int{11, 7, 0}, "string_agg(m.Brief, ',')", "String(500)", "Names: String(200)"},
		{[3]int{11, 7, 0}, "string_agg(m.Brief, ',')", "String(200)", ""},
		{[3]int{11, 13, 0}, "STRING_AGG(CASE WHEN s.Amount > 0 THEN s.Name ELSE '-' END, '; ')", "String", "Names: String(200)"},
		{[3]int{11, 13, 0}, "string_agg(s.Name, ',')", "Integer", "Names: String(200)"},
		// The 10.24 line got the 11.7 behaviour with the function (measured 10.24.22, 10.24.24).
		{[3]int{10, 24, 22}, "STRING_AGG(s.Name, '; ')", "String(100)", "Names: String(200)"},
		{[3]int{10, 24, 24}, "STRING_AGG(s.Name, '; ')", "String(200)", ""},
		// 11.2–11.6: the argument's type (measured 11.2.0, 11.6.6, 11.6.8, 11.6.12).
		{[3]int{11, 6, 6}, "STRING_AGG(s.Name, '; ')", "String(200)", "Names: String(100)"},
		{[3]int{11, 6, 6}, "STRING_AGG(s.Name, '; ')", "String", "Names: String(100)"},
		{[3]int{11, 6, 6}, "STRING_AGG(s.Name, '; ')", "String(100)", ""},
		{[3]int{11, 2, 0}, "string_agg(m.Brief, ',')", "String(200)", "Names: String(500)"},
		{[3]int{11, 2, 0}, "string_agg(m.Brief, ',')", "String(500)", ""},
		{[3]int{11, 6, 12}, "STRING_AGG(CASE WHEN s.Amount > 0 THEN s.Name ELSE '-' END, '; ')", "String(100)", ""},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d.%d.%d %s as %s", c.v[0], c.v[1], c.v[2], c.column, c.declared), func(t *testing.T) {
			ctx, _ := stringAggFixture(t, c.v)
			from := "MyFirstModule.Sale as s"
			if strings.Contains(c.column, "m.") {
				from = "MyFirstModule.Memo as m"
			}
			s := parseViewStmt(t, `create view entity MyFirstModule.V (Names: `+c.declared+`) as (
  select `+c.column+` as Names from `+from+`
);`)
			errs, _ := validateViewEntityTypes(ctx, s)
			if c.fix == "" {
				if len(errs) != 0 {
					t.Fatalf("refused a declaration that builds: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], c.fix) {
				t.Fatalf("want one error with the fix %q, got %v", c.fix, errs)
			}
		})
	}

	// Without a project version neither answer is safe, so the column is not
	// judged — `check` without -p cannot tell the eras apart.
	if vs := ValidateOQLTypes(`select string_agg(s.Name, ',') as Names from MyFirstModule.Sale as s`,
		[]ast.ViewAttribute{{Name: "Names", Type: ast.DataType{Kind: ast.TypeString, Length: 200}}}); len(vs) != 0 {
		t.Errorf("judged a STRING_AGG column with no version: %v", vs)
	}
}

// Mendix added STRING_AGG to OQL twice: in 11.2.0, and in 10.24.22 on the 10.24
// line. Each floor was found in mxbuild's own OQL lexer (the 'STRING_AGG'
// token) and confirmed with mx check — below it every STRING_AGG view is
// CE0174 "Unknown function 'string_agg'". The refusal names the floor that
// project's upgrade reaches first.
func TestCheckVersionGates_StringAggIsOnTwoReleaseLines(t *testing.T) {
	view := &ast.CreateViewEntityStmt{Query: ast.OQLQuery{
		RawQuery: "select b.Code as Code, STRING_AGG(s.Name, '; ') as Names\nfrom M.Sale as s\njoin s/M.Sale_Booking/M.Booking as b\ngroup by b.Code"}}
	prog := &ast.Program{Statements: []ast.Statement{view}}
	cases := []struct {
		v     [3]int
		floor string // "" = available
	}{
		{[3]int{10, 24, 21}, "10.24.22"},
		{[3]int{10, 24, 22}, ""},
		{[3]int{10, 24, 28}, ""},
		{[3]int{11, 0, 0}, "11.2.0"},
		{[3]int{11, 1, 0}, "11.2.0"},
		{[3]int{11, 2, 0}, ""},
		{[3]int{11, 12, 5}, ""},
	}
	for _, c := range cases {
		vs := versionGateViolations(versionCtx(t, c.v[0], c.v[1], c.v[2]), prog)
		switch {
		case c.floor == "" && len(vs) != 0:
			t.Errorf("%v: STRING_AGG refused where Mendix has it: %+v", c.v, vs)
		case c.floor != "" && (len(vs) != 1 || !strings.Contains(vs[0].Message, "STRING_AGG") ||
			!strings.Contains(vs[0].Message, "Mendix "+c.floor+"+")):
			t.Errorf("%v: want one refusal naming STRING_AGG and Mendix %s+, got %+v", c.v, c.floor, vs)
		}
	}
}

// The gate fires on a call, not on the letters.
func TestOQLUsesStringAgg(t *testing.T) {
	cases := []struct {
		oql  string
		want bool
	}{
		{`select string_agg(s.Name, ',') as N from M.Sale as s`, true},
		{`select STRING_AGG (s.Name, ',') as N from M.Sale as s`, true},
		{`from M.Sale as s select String_Agg(s.Name, ',') as N`, true},
		{`select 'string_agg(' as L from M.Sale as s`, false},
		{"select s.Name as N -- string_agg(s.Name, ',')\nfrom M.Sale as s", false},
		{`select s.my_string_agg as N from M.Sale as s`, false},
		{`select s."string_agg" as N from M.Sale as s`, false},
		{`select count(s.Name) as N from M.Sale as s`, false},
	}
	for _, c := range cases {
		if got := oqlUsesStringAgg(c.oql); got != c.want {
			t.Errorf("oqlUsesStringAgg(%q) = %v, want %v", c.oql, got, c.want)
		}
	}
}
