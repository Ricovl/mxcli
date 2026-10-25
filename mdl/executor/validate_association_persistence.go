// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Mendix lets a persistable and a non-persistable entity be associated in one
// shape only. mxbuild's own words, CE0001:
//
//	An association between a persistable entity and a non-persistable entity
//	must start in the non-persistable entity and have owner 'Default'.
//
// So two things are refused, and they are one rule: a FROM end that is the
// persistable one (a database row cannot point at an object that has no row),
// and `owner both` (which would make the persistable end own a reference too).
// Measured on mxbuild 11.13.0, a blank app with security on, one association
// each: non-persistable FROM / persistable TO is 0 errors under owner Default
// and CE0001 under owner Both (Reference and ReferenceSet, same-module and
// cross-module alike); persistable FROM / non-persistable TO is CE0001 under
// both owners.
//
// mxcli checked neither, so `create association … owner both` between a
// non-persistent and a persistent entity — the 1-1 shape people reach for, seen
// on PedApp's APD_Account — passed check and exec and failed the build
// (ako/mxcli#819). Refused in `check` and in `exec`, through one function each
// for the rule (associationPersistenceViolation) and for finding the ends of an
// association that is already stored (storedAssociationEnds), so the two cannot
// disagree.
const associationPersistenceRule = "MDL-ASSOC01"

// associationPersistenceViolation applies the CE0001 rule to an association
// whose two ends are known. persistable answers for one end, and says when it
// cannot tell — an entity neither the script nor the project resolves, a view or
// an external entity — and then nothing is reported: the rule is judged only on
// what is known.
func associationPersistenceViolation(assoc, from, to ast.QualifiedName, ownerBoth bool,
	persistable func(ast.QualifiedName) (bool, bool)) *linter.Violation {
	fromP, okFrom := persistable(from)
	toP, okTo := persistable(to)
	if !okFrom || !okTo || fromP == toP {
		return nil
	}
	var why, fix string
	switch {
	case fromP && ownerBoth:
		why = fmt.Sprintf("it starts in the persistable entity %s and has owner Both", from)
		fix = fmt.Sprintf("Reverse it with owner Default: `from %s to %s`.", to, from)
	case fromP:
		why = fmt.Sprintf("it starts in the persistable entity %s", from)
		fix = fmt.Sprintf("Reverse it: `from %s to %s`.", to, from)
	case ownerBoth:
		why = fmt.Sprintf("it has owner Both (%s is persistable, %s is not)", to, from)
		fix = "Drop `owner both` — owner Default is the only owner Mendix allows here."
	default:
		return nil // FROM the non-persistable end, owner Default: the allowed shape
	}
	return &linter.Violation{
		RuleID:   associationPersistenceRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("association %s links a persistable and a non-persistable entity, and %s — "+
			"Mendix requires such an association to start in the non-persistable entity and have "+
			"owner Default (CE0001 at build time)", assoc, why),
		Location:   linter.Location{DocumentType: "association", DocumentName: assoc.String()},
		Suggestion: fix,
	}
}

// projectEntityPersistence answers associationPersistenceViolation's question
// from the project: the backend resolves a specialization's persistability from
// its root generalization, System entities included.
func projectEntityPersistence(ctx *ExecContext) func(ast.QualifiedName) (bool, bool) {
	return func(qn ast.QualifiedName) (bool, bool) {
		if ctx == nil || ctx.Backend == nil || qn.Module == "" {
			return false, false
		}
		ent, err := findEntity(ctx, qn.Module, qn.Name)
		if err != nil || ent == nil || !judgedForPersistence(ent) {
			return false, false
		}
		return ent.Persistable, true
	}
}

// judgedForPersistence reports whether an entity's Persistable flag decides the
// CE0001 rule. A view entity cannot be associated at all (CE6771, refused on its
// own), and an external entity's storage is the remote service's — neither has
// been measured against this rule, so neither is judged by it.
func judgedForPersistence(ent *domainmodel.Entity) bool {
	return !isViewEntity(ent) && !strings.HasPrefix(ent.Source, "Rest$OData")
}

// storedAssociationEnds returns the FROM and TO entities of an association that
// is in the project, qualified, as `alter association … set owner both` needs
// them to judge the rule.
func storedAssociationEnds(ctx *ExecContext, name ast.QualifiedName) (from, to ast.QualifiedName, ok bool) {
	if ctx == nil || ctx.Backend == nil || name.Module == "" {
		return from, to, false
	}
	mod, err := findModule(ctx, name.Module)
	if err != nil || mod == nil {
		return from, to, false
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return from, to, false
	}
	local := func(id string) (ast.QualifiedName, bool) {
		for _, e := range dm.Entities {
			if string(e.ID) == id {
				return ast.QualifiedName{Module: mod.Name, Name: e.Name}, true
			}
		}
		return ast.QualifiedName{}, false
	}
	for _, a := range dm.Associations {
		if a.Name == name.Name {
			f, okF := local(string(a.ParentID))
			t, okT := local(string(a.ChildID))
			return f, t, okF && okT
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name.Name {
			f, okF := local(string(ca.ParentID))
			i := strings.LastIndex(ca.ChildRef, ".")
			if i <= 0 {
				return from, to, false
			}
			return f, ast.QualifiedName{Module: ca.ChildRef[:i], Name: ca.ChildRef[i+1:]}, okF
		}
	}
	return from, to, false
}

// ValidateAssociationPersistence is the `check` side: every CREATE [OR MODIFY]
// ASSOCIATION, and every ALTER ASSOCIATION … SET OWNER BOTH, against the CE0001
// rule. An entity the script declares answers from its declaration (an
// `extends` from its generalization, as Mendix inherits it); anything else from
// the project, when there is one.
func ValidateAssociationPersistence(prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	if prog == nil || !programAssociatesEntities(prog) {
		return nil
	}
	var ctx *ExecContext
	if project != nil {
		if b := project(); b != nil {
			ctx = &ExecContext{Backend: b}
		}
	}
	fromProject := projectEntityPersistence(ctx)
	decls := map[string]*ast.CreateEntityStmt{}
	created := map[string][2]ast.QualifiedName{}

	var persistable func(qn ast.QualifiedName, depth int) (bool, bool)
	persistable = func(qn ast.QualifiedName, depth int) (bool, bool) {
		d, ok := decls[strings.ToLower(qn.String())]
		if !ok {
			return fromProject(qn)
		}
		switch {
		case d.Kind == ast.EntityView || d.Kind == ast.EntityExternal:
			return false, false
		case d.Generalization != nil:
			if depth > 16 {
				return false, false // a cycle is not this rule's concern
			}
			return persistable(*d.Generalization, depth+1)
		}
		return d.Kind != ast.EntityNonPersistent, true
	}
	answer := func(qn ast.QualifiedName) (bool, bool) { return persistable(qn, 0) }

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			decls[strings.ToLower(s.Name.String())] = s
		case *ast.CreateAssociationStmt:
			from, to := s.Parent, s.Child
			if from.Module == "" {
				from.Module = s.Name.Module
			}
			if to.Module == "" {
				to.Module = s.Name.Module
			}
			created[strings.ToLower(s.Name.String())] = [2]ast.QualifiedName{from, to}
			if v := associationPersistenceViolation(s.Name, from, to, s.Owner == ast.OwnerBoth, answer); v != nil {
				out = append(out, *v)
			}
		case *ast.AlterAssociationStmt:
			if s.Operation != ast.AlterAssociationSetOwner || s.Owner != ast.OwnerBoth {
				continue
			}
			ends, ok := created[strings.ToLower(s.Name.String())]
			if !ok {
				f, t, found := storedAssociationEnds(ctx, s.Name)
				if !found {
					continue
				}
				ends = [2]ast.QualifiedName{f, t}
			}
			if v := associationPersistenceViolation(s.Name, ends[0], ends[1], true, answer); v != nil {
				out = append(out, *v)
			}
		}
	}
	return out
}

// programAssociatesEntities is the cheap pre-test: without a statement the rule
// applies to, the project is never opened.
func programAssociatesEntities(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateAssociationStmt:
			return true
		case *ast.AlterAssociationStmt:
			if s.Operation == ast.AlterAssociationSetOwner && s.Owner == ast.OwnerBoth {
				return true
			}
		}
	}
	return false
}
