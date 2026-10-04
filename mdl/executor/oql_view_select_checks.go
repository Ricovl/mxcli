// SPDX-License-Identifier: Apache-2.0

// Select-list checks for view-entity OQL that the type rules cannot see
// (ako/mxcli#981). Each one predicts a failure that check used to pass:
//
//	MDL033  a comparison as a select column          mxbuild CE0174
//	MDL034  an aggregate over a grouped column       mxbuild CE0174
//	MDL035  a plain column that is not grouped       mxbuild CE0174
//	MDL036  an expression that is not grouped        runtime (PostgreSQL 42803, HSQLDB 42574)
//	MDL037  a literal as an aggregate argument       runtime on HSQLDB (42567)
//	MDL038  a bare integer/string literal column     wrong data for consumers on HSQLDB
//
// MDL033–036 are errors and run inside ValidateOQLSyntax, so exec refuses them
// too. MDL037/038 are a warning and an info note about the database, not the
// model; they live in ValidateOQLPortability, which only check and the LSP
// call — exec turns every ValidateOQLSyntax finding into a refusal.
//
// Measured on mxbuild 11.13.0 and `run --local` on HSQLDB and PostgreSQL
// (ako/mxcli#981; the controls are in oql_view_select_checks_test.go).
package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// oqlTopLevelMask marks the bytes of expr that sit at parenthesis depth 0,
// outside quotes and outside every CASE … END. An operator found at such a
// byte belongs to the expression itself, not to a function argument, a
// subquery or a WHEN condition.
func oqlTopLevelMask(expr string) []bool {
	mask := make([]bool, len(expr))
	depth, caseDepth := 0, 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			for i++; i < len(expr) && expr[i] != c; i++ {
			}
			continue
		case c == '(':
			depth++
			continue
		case c == ')':
			depth--
			continue
		case isIdentChar(c) && (i == 0 || !isIdentChar(expr[i-1])):
			j := i
			for j < len(expr) && isIdentChar(expr[j]) {
				j++
			}
			if depth == 0 {
				switch strings.ToUpper(expr[i:j]) {
				case "CASE":
					caseDepth++
				case "END":
					if caseDepth > 0 {
						caseDepth--
					}
				}
			}
			i = j - 1
			continue
		}
		if depth == 0 && caseDepth == 0 {
			mask[i] = true
		}
	}
	return mask
}

// topLevelComparison returns the comparison operator an expression is built
// on, or "" when it has none at the top level.
func topLevelComparison(expr string) string {
	mask := oqlTopLevelMask(expr)
	for i := 0; i < len(expr); i++ {
		if !mask[i] {
			continue
		}
		switch expr[i] {
		case '=':
			return "="
		case '!', '<', '>':
			if i+1 < len(expr) && mask[i+1] && (expr[i+1] == '=' || (expr[i] == '<' && expr[i+1] == '>')) {
				return expr[i : i+2]
			}
			if expr[i] != '!' {
				return expr[i : i+1]
			}
		}
	}
	return ""
}

// splitTopLevelPlus splits expr at every top-level '+'. One part means there
// is no top-level '+'.
func splitTopLevelPlus(expr string) []string {
	mask := oqlTopLevelMask(expr)
	var parts []string
	last := 0
	for i := 0; i < len(expr); i++ {
		if mask[i] && expr[i] == '+' {
			parts = append(parts, strings.TrimSpace(expr[last:i]))
			last = i + 1
		}
	}
	return append(parts, strings.TrimSpace(expr[last:]))
}

// normalizeOQLExpr is the form two OQL expressions are compared in: case and
// whitespace folded and identifier quotes dropped outside string literals, so
// `datepart(YEAR, r."RaceDate")` and `DATEPART(year,r.RaceDate)` are equal.
func normalizeOQLExpr(expr string) string {
	var b strings.Builder
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case c == '\'':
			j := i + 1
			for j < len(expr) && expr[j] != '\'' {
				j++
			}
			if j < len(expr) {
				j++
			}
			b.WriteString(expr[i:j])
			i = j - 1
		case isOQLSpace(c):
			// Dropped, except between two words: `cast(m.ID as string)` must
			// not read as a column `m.idasstring`.
			j := i
			for j < len(expr) && isOQLSpace(expr[j]) {
				j++
			}
			out := b.String()
			if len(out) > 0 && j < len(expr) && isIdentChar(out[len(out)-1]) && isIdentChar(expr[j]) {
				b.WriteByte(' ')
			}
			i = j - 1
		case c == '"' || c == '`':
			// dropped
		default:
			b.WriteString(strings.ToLower(string(c)))
		}
	}
	return b.String()
}

var (
	oqlStringLiteralRe = regexp.MustCompile(`'[^']*'`)
	// oqlColumnRefRe finds `alias.attr` column references (and the qualified
	// segments of an association path) in a normalized expression.
	oqlColumnRefRe = regexp.MustCompile(`[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*`)
	// oqlPlainColumnRe is an expression that is nothing but one column.
	oqlPlainColumnRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*$`)
	oqlBareIdentRe   = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	// oqlAggregateCallRe matches an aggregate call in a normalized expression.
	oqlAggregateCallRe = regexp.MustCompile(`(^|[^a-z0-9_.])(count|sum|avg|min|max)\(`)
	// oqlRawAggregateCallRe is oqlAggregateCallRe over query text as written.
	oqlRawAggregateCallRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_.])(count|sum|avg|min|max)\s*\(`)
)

// oqlColumnRefs lists the column references in a normalized expression,
// ignoring anything inside a string literal.
func oqlColumnRefs(n string) []string {
	n = oqlStringLiteralRe.ReplaceAllString(n, "''")
	var out []string
	for _, loc := range oqlColumnRefRe.FindAllStringIndex(n, -1) {
		if loc[0] > 0 && (isIdentChar(n[loc[0]-1]) || n[loc[0]-1] == '.') {
			continue
		}
		out = append(out, n[loc[0]:loc[1]])
	}
	return out
}

// callArgument returns the text between the '(' at open and its matching ')'.
func callArgument(s string, open int) string {
	depth := 0
	for j := open; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : j]
			}
		case '\'':
			for j++; j < len(s) && s[j] != '\''; j++ {
			}
		}
	}
	return ""
}

// oqlAggregateArgs returns the argument of every aggregate call in a
// normalized expression.
func oqlAggregateArgs(n string) []string {
	var out []string
	for _, loc := range oqlAggregateCallRe.FindAllStringIndex(n, -1) {
		out = append(out, callArgument(n, loc[1]-1))
	}
	return out
}

// replaceOQLSubexpr replaces every occurrence of sub in n that stands on
// identifier boundaries, so `r.season` does not match inside `r.seasonend`.
func replaceOQLSubexpr(n, sub string) string {
	if sub == "" {
		return n
	}
	var b strings.Builder
	for i := 0; i < len(n); {
		if strings.HasPrefix(n[i:], sub) {
			before := i == 0 || !(isIdentChar(n[i-1]) || n[i-1] == '.')
			end := i + len(sub)
			after := end >= len(n) || !isIdentChar(n[end])
			if before && after {
				b.WriteString("#")
				i = end
				continue
			}
		}
		b.WriteByte(n[i])
		i++
	}
	return b.String()
}

// groupByExpressions returns the top-level GROUP BY list of a query, in either
// clause order. ok is false when there is no GROUP BY, or when the query is a
// UNION, whose branches each carry their own list and select clause.
func groupByExpressions(oql string) ([]string, bool) {
	upper := strings.ToUpper(oql)
	if topLevelKeywordIndex(oql, upper, 0, "UNION") >= 0 {
		return nil, false
	}
	at := topLevelKeywordIndex(oql, upper, 0, "GROUP BY")
	if at < 0 {
		return nil, false
	}
	start, _ := matchPhraseAt(oql, upper, at, "GROUP BY")
	end := topLevelKeywordIndex(oql, upper, start, "HAVING", "ORDER BY", "LIMIT", "OFFSET", "SELECT")
	if end < 0 {
		end = len(oql)
	}
	var out []string
	for _, e := range parseSelectColumns(oql[start:end]) {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out, len(out) > 0
}

// selectColumnParts splits a select column into its expression and its alias
// (empty when it has none), dropping a leading DISTINCT.
func selectColumnParts(col string) (expr, alias string) {
	col = strings.TrimSpace(col)
	if m := oqlAliasSuffixRe.FindStringSubmatch(col); m != nil {
		alias = unquoteOQLIdent(m[1])
		col = strings.TrimSuffix(col, m[0])
	}
	col = strings.TrimSpace(col)
	if len(col) > 9 && strings.EqualFold(col[:9], "distinct ") {
		col = strings.TrimSpace(col[9:])
	}
	return col, alias
}

func columnName(alias string, i int) string {
	if alias != "" {
		return alias
	}
	return fmt.Sprintf("column %d", i+1)
}

func viewOQLViolation(rule string, sev linter.Severity, msg, fix string) linter.Violation {
	return linter.Violation{
		RuleID:     rule,
		Severity:   sev,
		Message:    msg,
		Location:   linter.Location{DocumentType: "viewentity"},
		Suggestion: fix,
	}
}

// validateOQLSelectExpressions holds MDL033–MDL036: the select-list shapes
// mxbuild or the database refuses although every column is well typed.
func validateOQLSelectExpressions(oql string) []linter.Violation {
	selectClause := extractSelectClause(oql)
	if selectClause == "" {
		return nil
	}
	columns := parseSelectColumns(selectClause)
	out := comparisonColumns(columns)
	if groupBy, ok := groupByExpressions(oql); ok {
		out = append(out, groupByColumns(columns, groupBy)...)
	}
	return out
}

// comparisonColumns is MDL033: `r.Season = r.CurrentSeason as IsCurrent` is
// CE0174 ("The '=' part is incomplete or incorrect. You could use here:
// FROM."), and `!=` likewise; the same comparison inside a CASE builds
// (measured, 11.13.0).
func comparisonColumns(columns []string) []linter.Violation {
	var out []linter.Violation
	for i, col := range columns {
		expr, alias := selectColumnParts(col)
		if strings.HasPrefix(strings.ToLower(strings.TrimLeft(expr, "( ")), "select") {
			continue
		}
		op := topLevelComparison(expr)
		if op == "" {
			continue
		}
		name := columnName(alias, i)
		out = append(out, viewOQLViolation("MDL033", linter.SeverityError,
			fmt.Sprintf("select column %d (%s) is a comparison (%s) — a comparison is not a select "+
				"expression in OQL, and MxBuild rejects the view with CE0174 (\"The '%s' part is "+
				"incomplete or incorrect\")", i+1, name, op, op),
			fmt.Sprintf("Wrap it in a CASE: `case when %s then true else false end as %s`", expr, name)))
	}
	return out
}

// groupByColumns is MDL034–MDL036, the select columns of a grouped query.
func groupByColumns(columns, groupBy []string) []linter.Violation {
	var out []linter.Violation
	gbNorm := make([]string, 0, len(groupBy))
	gbRefs := map[string]bool{}
	gbAliases := map[string]bool{}
	for _, g := range groupBy {
		n := normalizeOQLExpr(g)
		gbNorm = append(gbNorm, n)
		for _, r := range oqlColumnRefs(n) {
			gbRefs[r] = true
		}
		if oqlBareIdentRe.MatchString(n) {
			gbAliases[n] = true
		}
	}
	// Longest first, so `datepart(year,r.racedate)` is replaced before a
	// shorter `r.racedate` could break it up.
	sort.Slice(gbNorm, func(i, j int) bool { return len(gbNorm[i]) > len(gbNorm[j]) })

	for i, col := range columns {
		expr, alias := selectColumnParts(col)
		n := normalizeOQLExpr(expr)
		if strings.Contains(n, "(select") || strings.HasPrefix(n, "select") {
			continue // a subquery is its own scope
		}
		name := columnName(alias, i)

		if args := oqlAggregateArgs(n); len(args) > 0 {
			// MDL034: an aggregate whose argument reads a column a GROUP BY
			// expression also reads — count(r.Season) by r.Season,
			// max(r.RaceDate) by datepart(YEAR, r.RaceDate), sum(r.Season) by
			// r.Season + 1 — is CE0174 on 11.13.0. count(r.Name) by r.Season
			// builds.
			if r := firstGroupedRef(args, gbRefs); r != "" {
				out = append(out, viewOQLViolation("MDL034", linter.SeverityError,
					fmt.Sprintf("select column %d (%s) aggregates %s, which the GROUP BY also uses — "+
						"MxBuild rejects the view with CE0174", i+1, name, r),
					"Aggregate a different column (count a non-null column that is not grouped), "+
						"or compute the value in a subquery"))
			}
			continue // aggregated: the grouping rules below do not apply
		}

		// MDL035/036: every other column has to be one of the GROUP BY
		// expressions, built from them, or a constant.
		if gbAliases[strings.ToLower(alias)] {
			continue // `group by Yr` groups the column aliased Yr (builds, 11.13.0)
		}
		rest := n
		for _, g := range gbNorm {
			rest = replaceOQLSubexpr(rest, g)
		}
		left := oqlColumnRefs(rest)
		if len(left) == 0 {
			continue
		}
		if oqlPlainColumnRe.MatchString(n) {
			out = append(out, viewOQLViolation("MDL035", linter.SeverityError,
				fmt.Sprintf("select column %d (%s) is %s, which is neither aggregated nor in the GROUP BY — "+
					"MxBuild rejects the view with CE0174 (\"Every expression in the SELECT clause must be "+
					"included in the GROUP BY clause\"); grouping by the ID does not exempt it", i+1, name, expr),
				fmt.Sprintf("Add %s to the GROUP BY, or aggregate it (e.g. max(%s))", expr, expr)))
			continue
		}
		out = append(out, viewOQLViolation("MDL036", linter.SeverityError,
			fmt.Sprintf("select column %d (%s) is not aggregated and is not a GROUP BY expression "+
				"(it reads %s outside one) — MxBuild accepts the view, but the database refuses the query "+
				"whenever the view is read (PostgreSQL 42803, HSQLDB 42574)", i+1, name, strings.Join(left, ", ")),
			fmt.Sprintf("Add `%s` to the GROUP BY, or aggregate it", expr)))
	}
	return out
}

// firstGroupedRef returns the first column an aggregate argument reads that a
// GROUP BY expression also reads, or "".
func firstGroupedRef(args []string, gbRefs map[string]bool) string {
	for _, a := range args {
		for _, r := range oqlColumnRefs(a) {
			if gbRefs[r] {
				return r
			}
		}
	}
	return ""
}

var (
	oqlIntLiteralRe  = regexp.MustCompile(`^-?\d+$`)
	oqlBoolLiteralRe = regexp.MustCompile(`^(?i:true|false)$`)
)

// oqlUntypedLiteral reports whether expr is a literal Mendix sends to the
// database as an untyped `?` parameter: an integer, string or boolean.
// Decimal literals are sent with a cast, so they are not among them.
func oqlUntypedLiteral(expr string) (kind string, ok bool) {
	expr = strings.TrimSpace(expr)
	for len(expr) >= 2 && expr[0] == '(' && expr[len(expr)-1] == ')' {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	switch {
	case oqlIntLiteralRe.MatchString(expr):
		return "Integer", true
	case oqlBoolLiteralRe.MatchString(expr):
		return "Boolean", true
	case len(expr) >= 2 && expr[0] == '\'' && expr[len(expr)-1] == '\'' &&
		!strings.Contains(strings.ReplaceAll(expr[1:len(expr)-1], "''", ""), "'"):
		return "String", true
	}
	return "", false
}

// ValidateOQLPortability reports the literals Mendix sends to the database as
// untyped parameters, which HSQLDB — Studio Pro's default database — cannot
// type (MDL037, MDL038). Neither concerns the model: mxbuild and PostgreSQL
// accept both, so they are a warning and a note, and exec does not run them.
func ValidateOQLPortability(oql string) []linter.Violation {
	var out []linter.Violation
	oql = stripOQLComments(oql)

	// MDL037: sum(1), count(1), max(0), count('x'), count(true) give HSQLDB
	// 42567 "data type cast needed" when the view is read. Anywhere in the
	// query, every branch.
	seen := map[string]bool{}
	for _, loc := range oqlRawAggregateCallRe.FindAllStringSubmatchIndex(oql, -1) {
		arg := strings.TrimSpace(callArgument(oql, loc[1]-1))
		kind, ok := oqlUntypedLiteral(arg)
		if !ok {
			continue
		}
		fn := strings.ToLower(oql[loc[4]:loc[5]])
		call := fn + "(" + arg + ")"
		if seen[call] {
			continue
		}
		seen[call] = true
		out = append(out, viewOQLViolation("MDL037", linter.SeverityWarning,
			fmt.Sprintf("%s aggregates a bare %s literal — Mendix sends it to the database as an untyped "+
				"parameter, and HSQLDB (Studio Pro's default database) refuses the query with 42567 "+
				"\"data type cast needed\" when the view is read; PostgreSQL accepts it", call, strings.ToLower(kind)),
			fmt.Sprintf("Aggregate a non-null column (`count(t.SomeColumn)`), or cast the literal: `%s(cast(%s as %s))`",
				fn, arg, kind)))
	}

	// MDL038: a bare integer or string literal as a view column reaches the
	// database untyped. The view reads fine; a consumer that aggregates it
	// fails as above, and on HSQLDB arithmetic on it concatenates instead
	// (`v.One + 1` is 11 there, 2 on PostgreSQL). Boolean columns are not
	// reported: nothing does arithmetic on them.
	selectClause := extractSelectClause(oql)
	if selectClause == "" {
		return out
	}
	for i, col := range parseSelectColumns(selectClause) {
		expr, alias := selectColumnParts(col)
		kind, ok := oqlUntypedLiteral(expr)
		if !ok || kind == "Boolean" {
			continue
		}
		name := columnName(alias, i)
		risk := "a consumer that aggregates it fails on HSQLDB, and arithmetic on it concatenates instead " +
			"(`v." + name + " + 1` is 11 for a column holding 1, where PostgreSQL gives 2)"
		if kind == "String" {
			risk = "a consumer that aggregates it fails on HSQLDB, which cannot type the parameter"
		}
		out = append(out, viewOQLViolation("MDL038", linter.SeverityInfo,
			fmt.Sprintf("select column %d (%s) is a bare %s literal, which Mendix sends to the database as an "+
				"untyped parameter: the view reads fine, but %s", i+1, name, strings.ToLower(kind), risk),
			fmt.Sprintf("Give it a type: `cast(%s as %s) as %s`", expr, kind, name)))
	}
	return out
}
