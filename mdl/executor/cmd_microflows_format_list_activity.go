// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// describeLanguage is the MDL language version describe writes: the newest
// frozen version (the one whose header describe emits, langver.HeaderLine),
// or the version of the script the describe runs in when that is newer.
//
// While mdl 1 is a preview, a plain `describe` therefore keeps writing the
// mdl 0 forms, because its output carries no header that would make the newer
// forms mean what they say (ADR-0011). Inside an `mdl 1;` script it writes the
// mdl 1 forms, which the same script can execute back.
func describeLanguage(ctx *ExecContext) langver.Version {
	v := langver.Frozen
	if ctx != nil && ctx.LanguageVersion > v {
		v = ctx.LanguageVersion
	}
	return v
}

// formatListActivity renders a List operation activity as the statement that
// mirrors it (PROPOSAL_mdl_beta_syntax_freeze.md §4, #733). It returns false
// for an activity the statement form cannot express, which then falls back to
// the call form.
func formatListActivity(op microflows.ListOperation, outputVar string) (string, bool) {
	out := "$" + outputVar
	switch o := op.(type) {
	case *microflows.HeadOperation:
		return fmt.Sprintf("%s = head $%s;", out, o.ListVariable), true
	case *microflows.TailOperation:
		return fmt.Sprintf("%s = tail $%s;", out, o.ListVariable), true
	case *microflows.FindOperation:
		return fmt.Sprintf("%s = find $%s where %s;", out, o.ListVariable, describeExpr(o.Expression)), true
	case *microflows.FilterOperation:
		return fmt.Sprintf("%s = filter $%s where %s;", out, o.ListVariable, describeExpr(o.Expression)), true
	case *microflows.FindByAttributeOperation:
		return formatMemberListActivity("find", out, o.ListVariable, o.Attribute, o.Association, o.Expression)
	case *microflows.FilterByAttributeOperation:
		return formatMemberListActivity("filter", out, o.ListVariable, o.Attribute, o.Association, o.Expression)
	case *microflows.SortOperation:
		if len(o.Sorting) == 0 {
			return "", false
		}
		items := make([]string, 0, len(o.Sorting))
		for _, s := range o.Sorting {
			dir := "asc"
			if s.Direction == microflows.SortDirectionDescending {
				dir = "desc"
			}
			name := s.AttributeQualifiedName
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			if name == "" {
				return "", false
			}
			items = append(items, mdlIdent(name)+" "+dir)
		}
		return fmt.Sprintf("%s = sort $%s by %s;", out, o.ListVariable, strings.Join(items, ", ")), true
	case *microflows.UnionOperation:
		return fmt.Sprintf("%s = union $%s with $%s;", out, o.ListVariable1, o.ListVariable2), true
	case *microflows.IntersectOperation:
		return fmt.Sprintf("%s = intersect $%s with $%s;", out, o.ListVariable1, o.ListVariable2), true
	case *microflows.SubtractOperation:
		// Studio Pro's Subtract is the first list minus the second.
		return fmt.Sprintf("%s = subtract $%s from $%s;", out, o.ListVariable2, o.ListVariable1), true
	case *microflows.ContainsOperation:
		return fmt.Sprintf("%s = contains $%s in $%s;", out, o.ObjectVariable, o.ListVariable), true
	case *microflows.EqualsOperation:
		return fmt.Sprintf("%s = equals $%s and $%s;", out, o.ListVariable1, o.ListVariable2), true
	case *microflows.ListRangeOperation:
		stmt := fmt.Sprintf("%s = range $%s", out, o.ListVariable)
		if o.OffsetExpression != "" {
			stmt += " offset " + describeExpr(o.OffsetExpression)
		}
		if o.LimitExpression != "" {
			stmt += " limit " + describeExpr(o.LimitExpression)
		}
		return stmt + ";", true
	}
	return "", false
}

// formatMemberListActivity renders Find / Filter by member: `by Member = value`.
func formatMemberListActivity(verb, out, list, attribute, association, value string) (string, bool) {
	field := extractFieldName(attribute, association)
	if value == "" {
		return "", false
	}
	if field == "" {
		return fmt.Sprintf("%s = %s $%s where %s;", out, verb, list, describeExpr(value)), true
	}
	return fmt.Sprintf("%s = %s $%s by %s = %s;", out, verb, list, field, describeExpr(value)), true
}

// formatAggregateActivity renders an Aggregate list activity as the statement
// that mirrors it. fn is the function's MDL keyword and attrName the short
// name of the aggregated attribute, both already resolved by the caller.
func formatAggregateActivity(ctx *ExecContext, a *microflows.AggregateListAction, fn, attrName, outputVar string, entityNames map[model.ID]string) string {
	out, list := "$"+outputVar, "$"+a.InputVariable
	switch a.Function {
	case microflows.AggregateFunctionCount:
		return fmt.Sprintf("%s = count %s;", out, list)
	case microflows.AggregateFunctionReduce:
		initial := describeExpr(a.ReduceInitialValue)
		if initial == "" {
			initial = "empty"
		}
		return fmt.Sprintf("%s = reduce %s from %s as %s using %s;", out, list, initial,
			formatMicroflowDataType(ctx, a.ReduceReturnType, entityNames), describeExpr(a.Expression))
	case microflows.AggregateFunctionAll, microflows.AggregateFunctionAny:
		return fmt.Sprintf("%s = %s %s where %s;", out, fn, list, describeExpr(a.Expression))
	}
	if a.UseExpression && a.Expression != "" {
		return fmt.Sprintf("%s = %s %s of %s;", out, fn, list, describeExpr(a.Expression))
	}
	return fmt.Sprintf("%s = %s %s by %s;", out, fn, list, mdlIdent(attrName))
}

// templateQuote writes the text of a log, show message or validation feedback
// template as the literal that reads back as that text (#746). Under mdl 1 a
// backslash is an ordinary character and a doubled apostrophe the only escape, so a line break
// is written into the literal as it is; the mdl 0 form writes it `\n`, which
// under mdl 1 would be a backslash and an n.
func templateQuote(ctx *ExecContext, text string) string {
	if describeLanguage(ctx) >= langver.V1 {
		return mdlQuoted(text)
	}
	return mdlQuote(text)
}
