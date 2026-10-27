// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// associationRetrieveYieldsList reports whether `retrieve $X from $Start/Assoc`
// binds a LIST rather than one object. It is the one answer both the flow
// builder (which types the output variable from it) and check --references
// (MDL-RETRIEVE03 over an association) use, so a check can never describe a
// retrieve differently from how exec writes it.
//
//   - A ReferenceSet yields a list from either end.
//   - A Reference followed from its FROM entity (the foreign-key owner, the
//     "many" side) yields the one TO object.
//   - A Reference followed from its TO entity (the "one" side) yields the list
//     of FROM objects pointing at it — unless the owner is Both, which makes it
//     one-to-one, and then it is one object from that side too.
//
// The same shapes measured for list widgets (MDL-ASSOCDS01, 11.14.0) and for
// microflow retrieves (mendixlabs/mxcli#1516, 11.6.6 and 11.15.0).
func associationRetrieveYieldsList(refSet, ownerBoth, fromToSide bool) bool {
	if refSet {
		return true
	}
	return fromToSide && !ownerBoth
}

// validateAssociationRetrieveListUse is MDL-RETRIEVE03 for a retrieve over an
// association. The no-project rule judges only database retrieves, because
// whether `retrieve $L from $h/M.A` binds a list or an object depends on the
// association's type, owner and which end $h is — the project knows, the AST
// does not. Without it, using the list as an object passed check --references,
// exec wrote it, and the build failed (mendixlabs/mxcli#1516, measured on
// 11.6.6 and 11.15.0):
//
//	$L/Qty in any expression        CE0117 "Error(s) in expression."
//	change $L (…)                   CE7247 "Variable 'L' is of type List of …, but should be of type Object."
//	call M.Sub(obj = $L)            CE0117 (the parameter takes one object)
//
// The bare list retrieve and list uses of it (count, loop, a list parameter)
// build clean on both versions, so they are not reported.
//
// Anything not established is silence: an association neither the project nor
// the script defines, an owner mxcli does not write, a self-association, a start
// variable whose entity this walk did not track, or one that is no end of the
// association by its generalization chain as far as it could be read.
func validateAssociationRetrieveListUse(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	var shapes map[string]assocShape
	var sigs map[string]*flowSignature
	signature := func(name string) *flowSignature {
		key := strings.ToLower(name)
		if sig, ok := sc.flowParams[key]; ok {
			return sig
		}
		if sigs == nil {
			sigs = buildFlowSignatures(ctx)
		}
		return sigs[key]
	}

	var errs []error
	for i, stmt := range prog.Statements {
		var params []ast.MicroflowParam
		var body []ast.MicroflowStatement
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			params, body = s.Parameters, s.Body
		case *ast.CreateNanoflowStmt:
			params, body = s.Parameters, s.Body
		default:
			continue
		}
		if !hasAssociationRetrieve(body) {
			continue
		}
		if shapes == nil {
			shapes = checkAssociationShapes(ctx, sc)
		}
		msgs := associationListObjectUses(ctx, shapes, signature, params, body)
		if len(msgs) == 0 {
			continue
		}
		if kind, name, excluded := excludedFlow(stmt); excluded {
			sc.warnExcluded(kind, name, msgs)
			continue
		}
		errs = append(errs, mdlerrors.NewValidationf("statement %d: a retrieve over an association "+
			"that yields a list is used as one object:\n  - %s", i+1, strings.Join(msgs, "\n  - ")))
	}
	return errs
}

func hasAssociationRetrieve(body []ast.MicroflowStatement) bool {
	found := false
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if r, ok := s.(*ast.RetrieveStmt); ok && r.StartVariable != "" {
			found = true
		}
	})
	return found
}

// assocListBinding is what one association retrieve bound to a list.
type assocListBinding struct {
	assoc, start, startEntity, elem string
}

// associationListObjectUses walks one flow body in statement order, tracking the
// entity each object variable holds (so a retrieve's start end can be decided)
// and which variables an association retrieve bound to a list.
func associationListObjectUses(ctx *ExecContext, shapes map[string]assocShape,
	signature func(string) *flowSignature, params []ast.MicroflowParam, body []ast.MicroflowStatement) []string {

	objects := map[string]string{} // object variable -> entity QN
	lists := map[string]string{}   // list variable -> element entity QN
	for _, p := range params {
		if e := astDataTypeEntity(p.Type); e != "" {
			if p.Type.Kind == ast.TypeListOf {
				lists[p.Name] = e
			} else {
				objects[p.Name] = e
			}
		}
	}
	bound := map[string]assocListBinding{}

	var out []string
	walkListObjectUses(body, func(name string) bool { _, ok := bound[name]; return ok },
		func(s ast.MicroflowStatement, name string, use listObjectUse) {
			b := bound[name]
			what := fmt.Sprintf("$%s = retrieve over %s from $%s (a %s) is a list of %s", name, b.assoc, b.start, b.startEntity, b.elem)
			var how, rejection string
			switch use {
			case listUseChange:
				how, rejection = fmt.Sprintf("`change $%s` needs one object", name),
					fmt.Sprintf("CE7247 \"Variable '%s' is of type List of %s, but should be of type Object.\"", name, b.elem)
			case listUseMemberPath:
				how, rejection = fmt.Sprintf("a member path `$%s/…` has nothing to navigate", name),
					"CE0117 \"Error(s) in expression.\""
			}
			out = append(out, fmt.Sprintf("%s, so %s — mxbuild rejects it with %s [MDL-RETRIEVE03]. %s",
				what, how, rejection, assocListSuggestion(name, b)))
		},
		func(s ast.MicroflowStatement) {
			// Object arguments: a list passed to a parameter that takes one object.
			var callee string
			var args []ast.CallArgument
			switch c := s.(type) {
			case *ast.CallMicroflowStmt:
				callee, args = c.MicroflowName.String(), c.Arguments
			case *ast.CallNanoflowStmt:
				callee, args = c.NanoflowName.String(), c.Arguments
			}
			if callee == "" || !strings.Contains(callee, ".") {
				return
			}
			for _, a := range args {
				v, ok := a.Value.(*ast.VariableExpr)
				if !ok {
					continue
				}
				b, ok := bound[v.Name]
				if !ok {
					continue
				}
				sig := signature(callee)
				if sig == nil {
					return
				}
				for _, p := range sig.Params {
					if p.Name == a.Name && p.Object && !p.List {
						out = append(out, fmt.Sprintf("$%s = retrieve over %s from $%s (a %s) is a list of %s, "+
							"so it cannot be passed to parameter %s of %s, which takes one %s — mxbuild rejects it "+
							"with CE0117 \"Error(s) in expression.\" [MDL-RETRIEVE03]. %s",
							v.Name, b.assoc, b.start, b.startEntity, b.elem, p.Name, callee, p.Entity,
							assocListSuggestion(v.Name, b)))
					}
				}
			}
		},
		func(s ast.MicroflowStatement) {
			// Rebinding, after the statement's uses were judged.
			for _, p := range statementProducedVars(s) {
				delete(bound, p.name)
				delete(objects, p.name)
				delete(lists, p.name)
			}
			switch st := s.(type) {
			case *ast.CreateObjectStmt:
				if st.Variable != "" && st.EntityType.Module != "" {
					objects[st.Variable] = st.EntityType.String()
				}
			case *ast.DeclareStmt:
				if e := astDataTypeEntity(st.Type); e != "" {
					if st.Type.Kind == ast.TypeListOf {
						lists[st.Variable] = e
					} else {
						objects[st.Variable] = e
					}
				}
			case *ast.LoopStmt:
				if e, ok := lists[st.ListVariable]; ok && st.LoopVariable != "" {
					objects[st.LoopVariable] = e
				}
			case *ast.RetrieveStmt:
				if st.Variable == "" || st.Source.Name == "" {
					return
				}
				if st.StartVariable == "" {
					if st.First {
						objects[st.Variable] = st.Source.String()
					} else {
						lists[st.Variable] = st.Source.String()
					}
					return
				}
				start := strings.TrimPrefix(st.StartVariable, "$")
				startEntity, ok := objects[start]
				if !ok {
					return
				}
				other, isList, known := associationRetrieveShape(ctx, shapes, st.Source, startEntity)
				if !known {
					return
				}
				if isList {
					lists[st.Variable] = other
					bound[st.Variable] = assocListBinding{
						assoc: shapes[strings.ToLower(qualifyAssoc(ctx, shapes, st.Source, startEntity))].qn,
						start: start, startEntity: startEntity, elem: other,
					}
				} else {
					objects[st.Variable] = other
				}
			}
		})
	return out
}

// qualifyAssoc is the association a retrieve names. A bare name resolves the
// way the flow builder resolves it (resolveBareAssociation, ako/mxcli#1690),
// not by assuming the start entity's module; one that does not resolve is "",
// which no shape matches, so the rule stays silent on it.
func qualifyAssoc(ctx *ExecContext, shapes map[string]assocShape, assoc ast.QualifiedName, startEntity string) string {
	if assoc.Module != "" {
		return assoc.String()
	}
	chain, complete := generalizationChain(ctx, startEntity)
	if len(chain) == 0 {
		chain = []string{startEntity}
	}
	qn, _, verdict := resolveBareAssociation(assoc.Name, chain, complete, associationCatalog{shapes: shapes, complete: true})
	if verdict != bareAssocResolved {
		return ""
	}
	return qn
}

// associationRetrieveShape decides what a retrieve over assoc from an object of
// startEntity binds: the entity at the other end, and whether it is a list.
// known is false whenever that was not established.
func associationRetrieveShape(ctx *ExecContext, shapes map[string]assocShape,
	assoc ast.QualifiedName, startEntity string) (other string, isList, known bool) {

	shape, ok := shapes[strings.ToLower(qualifyAssoc(ctx, shapes, assoc, startEntity))]
	if !ok || shape.owner == "" || strings.EqualFold(shape.from, shape.to) {
		return "", false, false
	}
	// The start may be a specialization of an end; walk as far as the
	// generalization chain can be read, and stay silent if no link is an end.
	chain, _ := generalizationChain(ctx, startEntity)
	if len(chain) == 0 {
		chain = []string{startEntity}
	}
	for _, qn := range chain {
		switch {
		case strings.EqualFold(qn, shape.from):
			return shape.to, associationRetrieveYieldsList(shape.refSet, shape.owner == "Both", false), true
		case strings.EqualFold(qn, shape.to):
			return shape.from, associationRetrieveYieldsList(shape.refSet, shape.owner == "Both", true), true
		}
	}
	return "", false, false
}

func assocListSuggestion(name string, b assocListBinding) string {
	return fmt.Sprintf("Keep the list and work on its items (`loop $item in $%s`, or `$item = head($%s);`), "+
		"or retrieve one %s from the database: `retrieve $%s from %s where [%s = $%s] first;`.",
		name, name, b.elem, name, b.elem, b.assoc, b.start)
}
