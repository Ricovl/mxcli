// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A widget data source constraint comparing an association with a ONE-hop
// comparand off a variable — `[Mod.District_Region = $currentObject/Mod.X]` —
// that mxbuild rejects although the hop count is fine (mendixlabs/mxcli#1466).
// MDL055 already refuses two hops; one hop is valid, so the comparand's TYPE is
// what is left to get wrong. Measured on fresh 11.13.0 and 11.14.0 projects
// (identical), one page each, in a combo box and a list view:
//
//	[Mod.District_Region = $currentObject/Mod.AreaPicker_Region]   ok      Region = Region
//	… = $currentObject/Mod.AreaPicker_SubRegion                    ok      a specialization
//	[Mod.District_SubRegion = $currentObject/Mod.AreaPicker_Region] ok     a generalization
//	[Mod.District_Region = $currentObject/Mod.AreaPicker_City]     CE0161  Region vs City
//	[Mod.District_Region = $Picker/Mod.AreaPicker_City]            CE0161  page parameter root
//	[Mod.District_Region = $currentObject/Label]                   CE0161  object vs attribute
//	[Mod.District_Region = $currentObject/AreaPicker_Region]       CE0161  unqualified association
//
// mxbuild's message is the same for all of them: `"Error(s) in XPath
// constraint." at Combo box 'cb'`. The two rules here say which mistake it is:
//
//   - MDL-XPATH02: the two sides are objects of unrelated entities, or one side
//     is an object and the other an attribute value;
//   - MDL-XPATH01: the comparand names an association unqualified — the
//     page-datasource arm of the retrieve rule of that name.
//
// It runs without a project, because the issue's shape is ONE script declaring
// the entities, the associations and the page; with -p the project answers for
// what the script does not declare. Anything this cannot establish — the
// enclosing entity, a hop, a generalization chain through an entity neither
// knows — is silence, not a finding. Not gated on the language version: each
// refused form already fails the build, so no script that builds changes
// meaning (the same reasoning as MDL055's page arm).

const xpathComparandTypeRule = "MDL-XPATH02"

// ValidateDatasourceXPathComparands judges the one-hop comparands of every
// database data source constraint the script's pages and snippets carry.
func ValidateDatasourceXPathComparands(prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	if prog == nil || !programHasVariableComparand(prog) {
		return nil
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var ctx *ExecContext
	if project != nil {
		if b := project(); b != nil {
			ctx = &ExecContext{Backend: b}
		}
	}
	sigs := map[string]*flowSignature{}
	if ctx != nil {
		sigs = buildFlowSignatures(ctx)
	}
	for name, sig := range sc.flowParams {
		sigs[name] = sig
	}
	v := &comparandValidator{types: checkMemberTypeIndex(ctx, sc), sigs: sigs}

	for _, stmt := range prog.Statements {
		var doc string
		var params []ast.PageParameter
		var roots []*ast.WidgetV3
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			doc, params, roots = "page "+s.Name.String(), s.Parameters, allPageWidgets(s)
		case *ast.CreateSnippetStmtV3:
			doc, params, roots = "snippet "+s.Name.String(), s.Parameters, s.Widgets
		default:
			continue
		}
		v.doc = doc
		v.pageParams = map[string]string{}
		for _, p := range params {
			if qn := p.EntityType.String(); qn != "" && qn != "." {
				v.pageParams[strings.ToLower(p.Name)] = qn
			}
		}
		for _, w := range roots {
			v.walk(w, dataContext{})
		}
	}
	return v.out
}

// programHasVariableComparand is the cheap pre-test: a database data source
// whose constraint names a variable at all. Without one nothing is indexed and
// the project is never opened.
func programHasVariableComparand(prog *ast.Program) bool {
	found := false
	for _, stmt := range prog.Statements {
		forEachWidget(stmt, func(w *ast.WidgetV3, _ string) {
			if ds := w.GetDataSource(); ds != nil && strings.EqualFold(ds.Type, "database") && strings.Contains(ds.Where, "$") {
				found = true
			}
		})
		if found {
			return true
		}
	}
	return false
}

type comparandValidator struct {
	types      memberTypeIndex
	sigs       map[string]*flowSignature
	pageParams map[string]string
	doc        string
	out        []linter.Violation
}

func (v *comparandValidator) walk(w *ast.WidgetV3, enclosing dataContext) {
	if w == nil {
		return
	}
	v.checkWidget(w, enclosing)
	inner := childContext(enclosing, w.GetDataSource(), v.sigs, v.pageParams)
	// A pluggable widget's slots each get the context their own link says,
	// which this walk does not model: nothing inside one is judged.
	if w.TypeIsGeneric || w.Type == "pluggablewidget" || w.Type == "customwidget" {
		inner = enclosing.withUnresolved()
	}
	for _, c := range w.Children {
		if isControlBar(c) {
			v.walk(c, enclosing)
			continue
		}
		v.walk(c, inner)
	}
}

func (v *comparandValidator) checkWidget(w *ast.WidgetV3, enclosing dataContext) {
	ds := w.GetDataSource()
	if ds == nil || ds.Where == "" || !strings.EqualFold(ds.Type, "database") || !strings.Contains(ds.Reference, ".") {
		return
	}
	groups := visitor.SplitXPathPredicateGroups(ds.Where)
	if len(groups) == 0 {
		groups = []string{ds.Where}
	}
	for _, g := range groups {
		expr, ok := visitor.ParseXPathConstraint(g)
		if !ok || expr == nil {
			continue // lenient parse: a group it cannot read is skipped, never reported
		}
		v.walkExpr(w, ds.Reference, enclosing, expr)
	}
}

func (v *comparandValidator) walkExpr(w *ast.WidgetV3, entity string, enclosing dataContext, expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if e.Operator == "=" || e.Operator == "!=" {
			if !v.checkComparison(w, entity, enclosing, e.Left, e.Right) {
				v.checkComparison(w, entity, enclosing, e.Right, e.Left)
			}
			return
		}
		v.walkExpr(w, entity, enclosing, e.Left)
		v.walkExpr(w, entity, enclosing, e.Right)
	case *ast.ParenExpr:
		v.walkExpr(w, entity, enclosing, e.Inner)
	case *ast.UnaryExpr:
		v.walkExpr(w, entity, enclosing, e.Operand)
	}
}

// checkComparison judges `member = $var/step`. It reports whether it emitted.
func (v *comparandValidator) checkComparison(w *ast.WidgetV3, entity string, enclosing dataContext, member, comparand ast.Expression) bool {
	left, ok := v.objectPathTarget(member, entity)
	if !ok {
		return false
	}
	path, ok := comparand.(*ast.XPathPathExpr)
	if !ok || len(path.Steps) != 2 || len(path.Steps[0].Predicates) > 0 || len(path.Steps[1].Predicates) > 0 {
		return false // two or more hops are MDL055's
	}
	root, ok := path.Steps[0].Expr.(*ast.VariableExpr)
	if !ok {
		return false
	}
	varEntity := v.variableEntity(root.Name, enclosing)
	if varEntity == "" {
		return false
	}
	label := fmt.Sprintf("%s: %s", v.doc, widgetLabel(w.Name, w.Type))
	text := fmt.Sprintf("$%s/%s", root.Name, xpathStepText(path.Steps[1].Expr))

	switch st := path.Steps[1].Expr.(type) {
	case *ast.QualifiedNameExpr:
		qn := st.QualifiedName.String()
		_, right, ok := v.types.follow(qn, varEntity)
		if !ok || v.related(left, right) {
			return false
		}
		v.out = append(v.out, linter.Violation{
			RuleID:   xpathComparandTypeRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s datasource constraint compares %s (a %s) with `%s` (a %s) — objects of unrelated entities, which mxbuild rejects with CE0161 \"Error(s) in XPath constraint.\"",
				label, xpathStepText(member), left, text, right),
			Suggestion: fmt.Sprintf("Compare with an association of %s that leads to %s, or change the left-hand path so it ends at %s.",
				varEntity, left, right),
		})
		return true
	case *ast.IdentifierExpr:
		name := st.Name
		if xpathImplicitMembers[name] {
			return false
		}
		_, attr, found, known := v.types.lookup(varEntity, name)
		if !known {
			return false
		}
		if found {
			v.out = append(v.out, linter.Violation{
				RuleID:   xpathComparandTypeRule,
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s datasource constraint compares %s (a %s object) with `%s`, the attribute %s.%s — an association compares with an object, not an attribute value; mxbuild rejects this with CE0161 \"Error(s) in XPath constraint.\"",
					label, xpathStepText(member), left, text, varEntity, attr.name),
				Suggestion: fmt.Sprintf("Compare an attribute with the attribute (`[<Mod.Assoc>/<Mod.Entity>/<Attr> = %s]`) or the association with an object (`[%s = $%s/<Mod.Assoc>]`).",
					text, xpathStepText(member), root.Name),
			})
			return true
		}
		assocQN, _, isAssoc := v.types.follow(name, varEntity)
		if !isAssoc {
			return false
		}
		v.out = append(v.out, linter.Violation{
			RuleID:   xpathAssociationRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s datasource constraint names the association %s unqualified in `%s` — Mendix requires the qualified form and the build fails with CE0161 \"Error(s) in XPath constraint.\"",
				label, name, text),
			Suggestion: fmt.Sprintf("Write `$%s/%s` instead of `%s`.", root.Name, assocQN, text),
		})
		return true
	}
	return false
}

// objectPathTarget follows a member path from entity and returns the entity
// it ends at, when every step is an association or an entity cast the index
// knows. A path ending in an attribute, or with a predicate, is not judged.
func (v *comparandValidator) objectPathTarget(expr ast.Expression, entity string) (string, bool) {
	var steps []ast.Expression
	switch e := expr.(type) {
	case *ast.QualifiedNameExpr:
		steps = []ast.Expression{e}
	case *ast.XPathPathExpr:
		for _, st := range e.Steps {
			if len(st.Predicates) > 0 {
				return "", false
			}
			steps = append(steps, st.Expr)
		}
	default:
		return "", false
	}
	cur := entity
	for _, st := range steps {
		q, ok := st.(*ast.QualifiedNameExpr)
		if !ok {
			return "", false
		}
		qn := q.QualifiedName.String()
		if _, next, ok := v.types.follow(qn, cur); ok {
			cur = next
			continue
		}
		if _, isEntity := v.types.attrs[qn]; isEntity {
			cur = qn // an entity cast
			continue
		}
		return "", false
	}
	return cur, len(steps) > 0
}

// variableEntity is the entity `$name` holds in a widget's constraint:
// $currentObject is the enclosing data container's object, anything else a
// document parameter. "" when it cannot be established.
func (v *comparandValidator) variableEntity(name string, enclosing dataContext) string {
	if strings.EqualFold(name, "currentObject") {
		if enclosing.unresolved || len(enclosing.entities) == 0 {
			return ""
		}
		return enclosing.entities[len(enclosing.entities)-1]
	}
	return v.pageParams[strings.ToLower(name)]
}

// related reports whether a and b are the same entity or one generalizes the
// other — measured both ways round as building clean. It answers true (stay
// silent) whenever either chain runs through an entity the index does not hold.
func (v *comparandValidator) related(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	ca, cb := v.types.chain(a), v.types.chain(b)
	for _, chain := range [][]string{ca, cb} {
		for _, e := range chain {
			if _, known := v.types.attrs[e]; !known {
				return true
			}
		}
	}
	for _, e := range ca {
		if strings.EqualFold(e, b) {
			return true
		}
	}
	for _, e := range cb {
		if strings.EqualFold(e, a) {
			return true
		}
	}
	return false
}

// xpathStepText renders a parsed member path back as written.
func xpathStepText(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.QualifiedNameExpr:
		return e.QualifiedName.String()
	case *ast.IdentifierExpr:
		return e.Name
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.XPathPathExpr:
		parts := make([]string, 0, len(e.Steps))
		for _, st := range e.Steps {
			parts = append(parts, xpathStepText(st.Expr))
		}
		return strings.Join(parts, "/")
	}
	return "?"
}
