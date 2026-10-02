// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// Widget properties that hold ONE Mendix expression, written as-is:
//
//	dynamicclasses: if $currentObject/Featured then 'is-featured' else ''
//	dynamicclasses: 'is-featured'          -- the string: the class is-featured
//
// A quoted value is a Mendix string, as for the OData client's credentials, not
// the expression's text — the old spelling, a quoted string holding the
// expression with its own quotes doubled, is
// refused at check time (MDL-WIDGET33). This is the named-property slice of
// PROPOSAL_first_class_expressions.md §6.2 slice 2. Pluggable properties whose
// schema kind is Expression take the same unquoted form through the generic
// keys (genericWidgetExpression, keepExpressionSource below); the executor,
// which knows the kind, reads the text and refuses it elsewhere (MDL-WIDGET39).
var widgetExpressionProps = map[string]bool{
	"dynamicclasses":   true, // any widget's Appearance.DynamicClasses
	"dynamiccellclass": true, // a datagrid column's columnClass
}

func isWidgetExpressionProp(name string) bool {
	return widgetExpressionProps[strings.ToLower(name)]
}

// widgetExpressionValue returns an expression property's value: the source text
// of whatever the grammar matched after the `:` or `=`, whitespace kept. Which
// alternative matched is not a signal — `$x/Cls + ' x'` may be claimed by the
// datasource or action alternatives, which also start with a variable — so the
// text is taken from the value node itself. A bracketed list is the one
// exception: it is returned as the list the other readers build, so MDL-WIDGET27
// (empty) and MDL-WIDGET32 (non-empty) still report it.
func widgetExpressionValue(valueNode antlr.ParserRuleContext) any {
	if pv, ok := valueNode.(*parser.PropertyValueV3Context); ok && (pv.LBRACKET() != nil || pv.ObjectEntryListV3() != nil) {
		return buildPropertyValueV3(pv)
	}
	return ruleSourceText(valueNode)
}

// lastRuleChild is the rule node after `name :` / `name =` — the value.
func lastRuleChild(ctx antlr.ParserRuleContext) antlr.ParserRuleContext {
	children := ctx.GetChildren()
	for i := len(children) - 1; i >= 0; i-- {
		if rc, ok := children[i].(antlr.ParserRuleContext); ok {
			return rc
		}
	}
	return nil
}

func widgetExpressionNotAllowed(name string, expr parser.IExpressionContext) error {
	return fmt.Errorf(
		"property %s takes a plain value, not an expression: %s — "+
			"only DynamicClasses and a column's DynamicCellClass take an expression",
		name, ruleSourceText(expr.(antlr.ParserRuleContext)))
}

// genericWidgetExpression is the value of a generic widget property written as
// an expression no other value form accepts. The visitor cannot tell an
// Expression-typed pluggable property (a chart series' dynamicBarColor) from a
// plain one, so it keeps the text and the schema-aware layers decide
// (PROPOSAL_first_class_expressions.md §6.2, the schema-driven slice).
func genericWidgetExpression(expr parser.IExpressionContext) *ast.WidgetExpressionV3 {
	return &ast.WidgetExpressionV3{Text: ruleSourceText(expr.(antlr.ParserRuleContext))}
}

// keepExpressionSource records the source text of a generic property whose
// value parsed as a VARIABLE-led data source — `$currentObject/ColorHex`,
// `$Param` — because that is also how a bare Mendix expression reads, and an
// Expression-typed pluggable property needs the text, not a data source
// (ast.WidgetV3.ValueSource). The keyword-led forms (database, microflow, …)
// are never expressions and are not recorded.
func keepExpressionSource(widget *ast.WidgetV3, key string, dsCtx parser.IDataSourceExprV3Context) {
	ds, ok := dsCtx.(*parser.DataSourceExprV3Context)
	if !ok || ds.VARIABLE() == nil || ds.DATABASE() != nil || ds.ASSOCIATION() != nil {
		return
	}
	if widget.ValueSource == nil {
		widget.ValueSource = map[string]string{}
	}
	widget.ValueSource[key] = ruleSourceText(ds)
}
