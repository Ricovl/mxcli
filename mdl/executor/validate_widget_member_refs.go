// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
)

// A member position inside a widget that reference checking walked past: an
// XPATH CONSTRAINT naming a member the constrained entity does not have
// (mendixlabs/mxcli#1049). It passed `check --references`, passed `exec`, and
// failed the build. The check resolved the entity in `database from
// Bench.Order` and never looked inside the `where […]`:
//
//	where [Bench.Order_Status = 'Open']
//	mx check -> [CE1613] "The selected association 'Bench.Order_Status'
//	no longer exists."
//
// This file also held MDL-WIDGET24, which refused a `$variable/` template
// parameter (#1046) because the writer bound it as an attribute named
// `$Customer/Name`. The writer now stores that spelling as the Mendix expression
// it is, as Studio Pro does (isVariableRootedTemplatePath, #1406), so the rule
// had nothing left to prevent and refused describe output of Studio Pro pages.

// ---------------------------------------------------------------------------
// XPath constraints: members of the entity being constrained (needs -p)
// ---------------------------------------------------------------------------

// validateXPathMembers resolves every step of a widget's XPath constraint
// against the entity the constraint filters.
//
// Only a DATABASE source is checked, because only there is the entity named in
// the statement. An association or microflow source carries its entity
// elsewhere, and an entity this cannot establish leaves the constraint
// unchecked rather than wrongly checked — the same rule as the member resolver
// in validate_member_refs.go, and for the same reason.
//
// The members are resolved against the project AND the script, with the
// resolver the microflow retrieve uses. Project-only, a member the script adds
// to a stored entity — `alter entity … add attribute`, a restating `create or
// modify`, a new association — was "neither an attribute nor an association",
// and exec refused a script that builds clean (mendixlabs/mxcli#1271).
func validateXPathMembers(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	m, resolve := scriptXPathResolver(ctx, sc)
	var errs []error
	for _, stmt := range prog.Statements {
		forEachWidget(stmt, func(w *ast.WidgetV3, where string) {
			ds := w.GetDataSource()
			if ds == nil || ds.Where == "" || !strings.EqualFold(ds.Type, "database") {
				return
			}
			entityQN := ds.Reference
			if entityQN == "" || !strings.Contains(entityQN, ".") {
				return
			}
			// Checked as it is stored: a qualified attribute of the entity is
			// the attribute, not a step to resolve (ako/mxcli#874).
			stored := storedXPathConstraint(ds.Where, entityQN)
			for _, bad := range unresolvableXPathStepsAndLeadingEntity(ctx, m, stored, entityQN, resolve) {
				if bad.kind == "entity" {
					errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf("%s: %s",
						where, leadingEntityStepMessage(ctx, stored, bad.name, entityQN))))
					continue
				}
				errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf(
					"%s: the constraint on %s names %q, which is neither an attribute nor an "+
						"association of it — mxbuild reports this as CE1613 \"The selected %s "+
						"'%s' no longer exists\"",
					where, entityQN, bad.name, bad.kind, bad.name)))
			}
		})
	}
	return errs
}

// badStep is one step of a constraint that resolved to nothing.
type badStep struct {
	name string
	kind string // "association" or "attribute" — which mxbuild will call it; "entity" for a leading entity step
}

// leadingEntityStepMessage is the report for a constraint that starts with an
// entity as a path step (unresolvableXPathStepsAndLeadingEntity). Any entity
// there is CE0161 — measured on 11.15.0 for the constrained entity, a
// specialization of it and an unrelated one — but dropping the step is the fix
// only when it names the constrained entity or a generalization, whose members
// the constraint already sees; another entity is reached through an association.
func leadingEntityStepMessage(ctx *ExecContext, constraint, entityStep, evaluatedOn string) string {
	fix := "name a member of " + evaluatedOn + ", or reach " + entityStep + " through an association (`[Module.Assoc/" + entityStep + "/…]`)"
	chain, _ := generalizationChain(ctx, evaluatedOn)
	for _, e := range chain {
		if e != entityStep {
			continue
		}
		fix = "name the member directly"
		if m := regexp.MustCompile(regexp.QuoteMeta(entityStep) + `/([A-Za-z_][A-Za-z0-9_.]*)`).FindStringSubmatch(constraint); m != nil {
			fix = fmt.Sprintf("drop the leading `%s/`: `[%s = …]`", entityStep, m[1])
		}
		break
	}
	return fmt.Sprintf("the constraint starts with the entity %s as a path step, but a constraint is "+
		"evaluated on %s and cannot start with an entity — %s. mxbuild rejects a leading entity step (CE0161 \"Error(s) in XPath constraint\")",
		entityStep, evaluatedOn, fix)
}

// unresolvableXPathStepsWith walks the constraint's predicate groups and returns
// the steps that name nothing on the entity. resolveBare is the bare-member
// lookup — every caller passes scriptXPathResolver's, which also knows the
// entities the script declares; nil is resolveMemberOnEntity.
//
// The parse is deliberately lenient (ANTLR with the error listeners removed), so
// a group it cannot read is SKIPPED rather than reported: a tree that quietly
// omits part of its input would otherwise produce an error about text nobody
// wrote. That is the same trap xpathrefs documents for the rename path, where
// the consequence is a corrupted constraint; here it is only a false error, but
// a false error still blocks a script that builds.
func unresolvableXPathStepsWith(ctx *ExecContext, m xpathrefs.Model, constraint, entityQN string,
	resolveBare func(entityQN, member string) memberResolution) []badStep {
	return walkXPathSteps(ctx, m, constraint, entityQN, resolveBare, false)
}

// unresolvableXPathStepsAndLeadingEntity is unresolvableXPathStepsWith that also
// reports a path whose FIRST step is an entity, as kind "entity".
//
// A constraint is evaluated on its entity, so `[Shop.Order/Status = 'Open']` on
// Shop.Order is not a cast; the walk read it as one. Measured on mxbuild 11.15.0,
// it is CE0161 "Error(s) in XPath constraint" in a microflow retrieve, a list
// view's database source and a user task's targeting (ako/mxcli#1711); after an
// association (`[Shop.Order_Customer/Shop.Customer/Name = …]`) the same step is
// the path's specialization step and builds. mx check did not reject one in an
// access rule on that app, so the access-rule check keeps the plain walk.
func unresolvableXPathStepsAndLeadingEntity(ctx *ExecContext, m xpathrefs.Model, constraint, entityQN string,
	resolveBare func(entityQN, member string) memberResolution) []badStep {
	return walkXPathSteps(ctx, m, constraint, entityQN, resolveBare, true)
}

func walkXPathSteps(ctx *ExecContext, m xpathrefs.Model, constraint, entityQN string,
	resolveBare func(entityQN, member string) memberResolution, leadingEntity bool) []badStep {
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		groups = []string{constraint}
	}
	var out []badStep
	for _, g := range groups {
		expr, ok := visitor.ParseXPathConstraint(g)
		if !ok || expr == nil {
			continue
		}
		v := &xpathMemberVisitor{ctx: ctx, model: m, resolveBare: resolveBare, leadingEntity: leadingEntity}
		v.walk(expr, entityQN)
		out = append(out, v.bad...)
	}
	return out
}

// xpathMemberVisitor walks a parsed constraint carrying the entity each step is
// evaluated against, the same traversal xpathrefs' walker performs for renames.
type xpathMemberVisitor struct {
	ctx           *ExecContext
	model         xpathrefs.Model
	resolveBare   func(entityQN, member string) memberResolution // nil: resolveMemberOnEntity
	leadingEntity bool                                           // report a path that starts with an entity step
	bad           []badStep
	seen          map[string]bool
}

func (v *xpathMemberVisitor) add(s badStep) {
	if v.seen == nil {
		v.seen = map[string]bool{}
	}
	if v.seen[s.name] {
		return
	}
	v.seen[s.name] = true
	v.bad = append(v.bad, s)
}

func (v *xpathMemberVisitor) walk(expr ast.Expression, cur string) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.XPathPathExpr:
		v.walkPath(e.Steps, cur)
	case *ast.BinaryExpr:
		v.walk(e.Left, cur)
		v.walk(e.Right, cur)
	case *ast.UnaryExpr:
		v.walk(e.Operand, cur)
	case *ast.ParenExpr:
		v.walk(e.Inner, cur)
	case *ast.FunctionCallExpr:
		for _, a := range e.Arguments {
			v.walk(a, cur)
		}
	case *ast.IfThenElseExpr:
		v.walk(e.Condition, cur)
		v.walk(e.ThenExpr, cur)
		v.walk(e.ElseExpr, cur)
	case *ast.SourceExpr:
		v.walk(e.Expression, cur)
	case *ast.IdentifierExpr:
		v.noteBare(e.Name, cur)
	case *ast.QualifiedNameExpr:
		// A qualified name standing alone in a predicate is the shape #1049
		// reported: `[Bench.Order_Status = 'Open']`, where the name looks like
		// an association and is not one.
		v.noteQualified(e.QualifiedName.String(), cur)
	}
}

// noteBare checks a bare step, which Mendix evaluates as an attribute of cur.
func (v *xpathMemberVisitor) noteBare(name, cur string) {
	if cur == "" || name == "" {
		return
	}
	// The members every entity has in XPath without declaring them. An entity
	// stores createdDate as a flag, not an attribute, so the attribute lookup
	// would call `[createdDate > …]` — which builds clean, measured on 11.13.0 —
	// missing.
	if xpathImplicitMembers[name] {
		return
	}
	resolve := v.resolveBare
	if resolve == nil {
		resolve = func(e, m string) memberResolution { return resolveMemberOnEntity(v.ctx, e, m) }
	}
	if resolve(cur, name) == memberMissing {
		v.add(badStep{name: name, kind: "attribute"})
	}
}

// noteQualified checks a `Module.Name` step, which is an association hop or an
// entity cast. Anything else names nothing.
func (v *xpathMemberVisitor) noteQualified(qn, cur string) {
	if qn == "" {
		return
	}
	if _, ok := v.model.AssociationTarget(qn, cur); ok {
		return
	}
	if v.model.IsEntity(qn) {
		return
	}
	// System.owner / System.changedBy are associations every entity is
	// addressed through in XPath; whether the entity STORES the member is
	// validateRetrieveConstraints' question, with its own message and fix.
	if _, ok := xpathSystemAssociations[qn]; ok {
		return
	}
	// Only report when the entity it would have hung off is itself KNOWN.
	//
	// Without this the check fires on a constraint whose base entity the project
	// does not have — a page written against a module the script creates, or run
	// against the wrong app — and reports a perfectly good association as
	// missing. Caught by a control: `[Bench.Order_Customer/…]` on a project with
	// no Bench module was reported, and the association was real.
	//
	// This is the same three-valued discipline the bare-member path gets from
	// resolveMemberOnEntity: could-not-establish is silence, not a finding.
	if cur == "" || !v.model.IsEntity(cur) {
		return
	}
	// And the same discipline on the OTHER axis, which this guard does not cover:
	// the base entity being known says nothing about whether the ASSOCIATION was
	// ruled out. AssociationTarget above answers yes-or-not-established, so a hop
	// mxcli merely failed to resolve would be reported as a defect. Only a lookup
	// that actually ruled the name out becomes a finding.
	switch _, res := resolveAssociationFrom(v.ctx, qn, cur); res {
	case assocMissing, assocNotAnEnd:
		v.add(badStep{name: qn, kind: "association"})
	}
}

func (v *xpathMemberVisitor) walkPath(steps []ast.XPathStep, cur string) {
	for i, st := range steps {
		next := ""
		switch e := st.Expr.(type) {
		case *ast.IdentifierExpr:
			if i == len(steps)-1 {
				v.noteBare(e.Name, cur)
			}
		case *ast.QualifiedNameExpr:
			qn := e.QualifiedName.String()
			if t, ok := v.model.AssociationTarget(qn, cur); ok {
				next = t
			} else if t, ok := xpathSystemAssociations[qn]; ok {
				next = t
			} else if v.model.IsEntity(qn) {
				if i == 0 && v.leadingEntity {
					v.add(badStep{name: qn, kind: "entity"})
				}
				next = qn
			} else {
				v.noteQualified(qn, cur)
			}
		}
		for _, pred := range st.Predicates {
			v.walk(pred, next)
		}
		cur = next
	}
}

// xpathImplicitMembers are the bare names XPath resolves on every entity
// without the entity declaring an attribute of that name.
//
// owner and changedBy are NOT among them: they are ASSOCIATIONS to System.User,
// addressed as System.owner / System.changedBy. Measured on mxbuild 11.14.0, on
// a System.FileDocument specialization (which stores both):
//
//	[System.owner = '[%CurrentUser%]']   clean
//	[owner = '[%CurrentUser%]']          CE0161
//	[System.Owner = '[%CurrentUser%]']   CE1613
//
// Listing the bare names here accepted the CE0161 spelling, and the qualified
// one — the only one that builds — was reported as naming nothing.
var xpathImplicitMembers = map[string]bool{
	"id": true, "createdDate": true, "changedDate": true,
}

// xpathSystemAssociations are the system members XPath addresses as an
// association, mapped to the entity the hop lands on.
var xpathSystemAssociations = map[string]string{
	"System.owner":     "System.User",
	"System.changedBy": "System.User",
}

// execXPathModel answers xpathrefs.Model from the connected project.
type execXPathModel struct{ ctx *ExecContext }

func (m *execXPathModel) IsEntity(qn string) bool {
	b, ok := m.ctx.Backend.(entityLookupBackend)
	if !ok {
		return false
	}
	_, found := findEntityByQN(b, qn)
	return found
}

func (m *execXPathModel) AssociationTarget(qn, from string) (string, bool) {
	return associationTargetFrom(m.ctx, qn, from)
}

// ---------------------------------------------------------------------------
// Shared widget walk
// ---------------------------------------------------------------------------

// forEachWidget visits every widget a statement carries, with a label naming
// where it is. Every widget-bearing field has to be walked: a widget the walk
// misses is a widget nothing checks, and the escape is silent both ways.
func forEachWidget(stmt ast.Statement, fn func(w *ast.WidgetV3, where string)) {
	var doc string
	var roots []*ast.WidgetV3
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		doc = "page " + s.Name.String()
		roots = allPageWidgets(s)
	case *ast.CreateSnippetStmtV3:
		doc = "snippet " + s.Name.String()
		roots = append(roots, s.Widgets...)
	case *ast.CreateLayoutStmt:
		doc = "layout " + s.Name.String()
		roots = append(roots, s.AllWidgets()...)
	case *ast.AlterPageStmt:
		doc = strings.ToLower(s.ContainerType) + " " + s.PageName.String()
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				roots = append(roots, o.Widgets...)
			case *ast.ReplaceWidgetOp:
				roots = append(roots, o.NewWidgets...)
			}
		}
	default:
		return
	}
	var walk func(w *ast.WidgetV3)
	walk = func(w *ast.WidgetV3) {
		if w == nil {
			return
		}
		fn(w, fmt.Sprintf("%s: %s %q", doc, strings.ToLower(w.Type), w.Name))
		for _, c := range w.Children {
			walk(c)
		}
	}
	for _, w := range roots {
		walk(w)
	}
}
