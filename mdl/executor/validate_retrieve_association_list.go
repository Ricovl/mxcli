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

// assocRetrieveEnd is the end of an association a retrieve over it starts from.
type assocRetrieveEnd int

const (
	assocEndNone      assocRetrieveEnd = iota // the start is no end of it, as far as is known
	assocEndFrom                              // the FROM end: the association's parent, the foreign-key owner
	assocEndTo                                // the TO end: its child
	assocEndAmbiguous                         // a specialization of both ends of an association between two entities
)

// associationRetrieveEnd decides which end `retrieve $X from $Start/Assoc`
// starts from, given whether $Start holds (a specialization of) the
// association's FROM entity and its TO entity. It is the one answer the full
// build (addRetrieveAction), a spliced fragment (associationRetrieveType) and
// check --references (associationRetrieveShape) take, each asking "is it a" of
// its own view of the project.
//
// A self-reference has the start at both ends, and the stored
// AssociationRetrieveSource records no direction (only the start variable and
// the association): Mendix follows it from its FROM end, so a Reference gives
// the object it holds and a Reference set the list. Measured on 11.13.0
// (ako/mxcli#1777, #1793): over M.Order_Parent from M.Order to M.Order a
// `change` of the output builds, while `count` is CE0013 "Input variable 'P'
// must be of type 'List'.", a loop CE0100 "'P' is of type M.Order, but should
// be of type List.", from an M.Order and from a specialization of it alike.
// Owner Both on a self-reference is CE0002, so the one-to-one case does not
// arise. A start that is a specialization of both ends of an association
// between two different entities is ambiguous: which end Mendix takes there is
// not measured.
func associationRetrieveEnd(isFrom, isTo, selfReference bool) assocRetrieveEnd {
	switch {
	case isFrom && isTo:
		if selfReference {
			return assocEndFrom
		}
		return assocEndAmbiguous
	case isTo:
		return assocEndTo
	case isFrom:
		return assocEndFrom
	}
	return assocEndNone
}

// validateAssociationRetrieveListUse is MDL-RETRIEVE03 for a retrieve over an
// association, and MDL-RETRIEVE01 the other way round. The no-project rules
// judge only database retrieves, because whether `retrieve $L from $h/M.A`
// binds a list or an object depends on the association's type, owner and
// which end $h is — the project knows, the AST does not. Without it, using the
// list as an object passed check --references, exec wrote it, and the build
// failed (mendixlabs/mxcli#1516, measured on 11.6.6 and 11.15.0):
//
//	$L/Qty in any expression        CE0117 "Error(s) in expression."
//	change $L (…)                   CE7247 "Variable 'L' is of type List of …, but should be of type Object."
//	call M.Sub(obj = $L)            CE0117 (the parameter takes one object)
//
// The bare list retrieve and list uses of it (count, loop, a list parameter)
// build clean on both versions, so they are not reported.
//
// The reverse — the one object a Reference followed from its FROM end binds,
// used as a list — passed the same way (ako/mxcli#1793, measured on 11.13.0).
// A self-reference is always followed from its FROM end, and the full build
// used to type it as a list, so that was where it showed:
//
//	$n = count $P;  (any aggregate)        CE0013 "Input variable 'P' must be of type 'List'."
//	$h = head $P;   (any list operation)   CE0097 "The selected 'P' variable must be of type List."
//	loop $x in $P                          CE0100 "'P' is of type M.Order, but should be of type List."
//	call M.Sub(list = $P)                  CE0117 (the parameter takes a list)
//
// `add`, `remove`, `clear` and `set` over the object are Change list
// activities and a `range` is a Microflows$ListRange; mx check types none of
// their variables (each measured clean), so they are not reported here — `set`
// on an object is MDL-SET01's.
//
// Anything not established is silence: an association neither the project nor
// the script defines, an owner mxcli does not write, a start variable whose
// entity this walk did not track, one that is no end of the association by its
// generalization chain as far as it could be read, or a specialization of both
// ends of an association between two entities (associationRetrieveEnd). The
// object a one-to-one gives from its TO end is not judged as a list either: the
// full build rewrites a list use of it into a database retrieve.
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
		uses := associationListObjectUses(ctx, sc, shapes, signature, params, body)
		for _, group := range []struct {
			msgs []string
			what string
		}{
			{uses.listAsObject, "that yields a list is used as one object"},
			{uses.objectAsList, "that yields one object is used as a list"},
		} {
			if len(group.msgs) == 0 {
				continue
			}
			if kind, name, excluded := excludedFlow(stmt); excluded {
				sc.warnExcluded(kind, name, group.msgs)
				continue
			}
			errs = append(errs, mdlerrors.NewValidationf("statement %d: a retrieve over an association "+
				"%s:\n  - %s", i+1, group.what, strings.Join(group.msgs, "\n  - ")))
		}
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

// assocListBinding is what one association retrieve bound: a list of elem, or
// one elem.
type assocListBinding struct {
	assoc, start, startEntity, elem string
	selfReference                   bool
}

// assocRetrieveUses are the misuses of association retrieves in one flow body.
type assocRetrieveUses struct {
	listAsObject []string // MDL-RETRIEVE03: a list taken as one object
	objectAsList []string // MDL-RETRIEVE01: one object taken as a list
}

// associationListObjectUses walks one flow body in statement order, tracking the
// entity each object variable holds (so a retrieve's start end can be decided)
// and which variables an association retrieve bound to a list, or to the one
// object a Reference followed from its FROM end holds.
func associationListObjectUses(ctx *ExecContext, sc *scriptContext, shapes map[string]assocShape,
	signature func(string) *flowSignature, params []ast.MicroflowParam, body []ast.MicroflowStatement) assocRetrieveUses {

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
	bound := map[string]assocListBinding{}  // bound to a list
	single := map[string]assocListBinding{} // bound to the object at a Reference's TO end

	var out assocRetrieveUses
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
			out.listAsObject = append(out.listAsObject, fmt.Sprintf("%s, so %s — mxbuild rejects it with %s [MDL-RETRIEVE03]. %s",
				what, how, rejection, assocListSuggestion(name, b)))
		},
		func(s ast.MicroflowStatement) {
			// One object taken by a list activity.
			if name, op, rejection := listActivityInput(s); name != "" {
				if b, ok := single[name]; ok {
					out.objectAsList = append(out.objectAsList, assocObjectAsList(name, b, op, rejection))
				}
			}
			// Arguments: a list passed to a parameter that takes one object,
			// and one object passed to a parameter that takes a list.
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
				listB, isList := bound[v.Name]
				oneB, isOne := single[v.Name]
				if !isList && !isOne {
					continue
				}
				sig := signature(callee)
				if sig == nil {
					return
				}
				for _, p := range sig.Params {
					if p.Name != a.Name || !p.Object {
						continue
					}
					switch {
					case isList && !p.List:
						b := listB
						out.listAsObject = append(out.listAsObject, fmt.Sprintf("$%s = retrieve over %s from $%s (a %s) is a list of %s, "+
							"so it cannot be passed to parameter %s of %s, which takes one %s — mxbuild rejects it "+
							"with CE0117 \"Error(s) in expression.\" [MDL-RETRIEVE03]. %s",
							v.Name, b.assoc, b.start, b.startEntity, b.elem, p.Name, callee, p.Entity,
							assocListSuggestion(v.Name, b)))
					case isOne && p.List:
						out.objectAsList = append(out.objectAsList, assocObjectAsList(v.Name, oneB,
							fmt.Sprintf("parameter %s of %s, which takes a list of %s,", p.Name, callee, p.Entity),
							"CE0117 \"Error(s) in expression.\""))
					}
				}
			}
		},
		func(s ast.MicroflowStatement) {
			// Rebinding, after the statement's uses were judged.
			for _, p := range statementProducedVars(s) {
				delete(bound, p.name)
				delete(single, p.name)
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
				shape, known := associationRetrieveShape(ctx, sc, shapes, st.Source, startEntity)
				if !known {
					return
				}
				b := assocListBinding{assoc: shape.qn, start: start, startEntity: startEntity, elem: shape.other,
					selfReference: shape.selfReference}
				switch {
				case shape.isList:
					lists[st.Variable] = shape.other
					bound[st.Variable] = b
				case !shape.fromTo:
					objects[st.Variable] = shape.other
					single[st.Variable] = b
				default:
					// One object of a one-to-one from its TO end: the full build
					// turns a list use of it into a database retrieve.
					objects[st.Variable] = shape.other
				}
			}
		})
	return out
}

// listActivityInput is the variable a list activity takes as its list — an
// aggregate's, a list operation's first operand, a loop's — with what takes it
// and the CE code mxbuild reports when it holds one object. "" for any other
// statement. Measured on 11.13.0 over a self-reference's object
// (ako/mxcli#1793): every aggregate (count, sum, average, minimum, maximum,
// all, any, reduce) CE0013; every list operation but range (head, tail, find,
// filter, sort, union, intersect, subtract, contains, equals) CE0097; a loop
// CE0100. A range is Microflows$ListRange and mx check reports nothing on it,
// so it is not judged; nor is a list operation's second operand, which union,
// intersect, subtract and contains take as an object.
func listActivityInput(s ast.MicroflowStatement) (name, op, rejection string) {
	switch st := s.(type) {
	case *ast.AggregateListStmt:
		if st.InputVariable != "" {
			return st.InputVariable, "`" + strings.ToLower(st.Operation.String()) + "`",
				fmt.Sprintf("CE0013 \"Input variable '%s' must be of type 'List'.\"", st.InputVariable)
		}
	case *ast.ListOperationStmt:
		if st.InputVariable != "" && st.Operation != ast.ListOpRange {
			return st.InputVariable, "`" + strings.ToLower(st.Operation.String()) + "`",
				fmt.Sprintf("CE0097 \"The selected '%s' variable must be of type List.\"", st.InputVariable)
		}
	case *ast.LoopStmt:
		if st.ListVariable != "" {
			return st.ListVariable, "a loop", ""
		}
	}
	return "", "", ""
}

// assocObjectAsList is the MDL-RETRIEVE01 message for one object an association
// retrieve bound, taken by what as a list. An empty rejection is a loop's, which
// names the entity.
func assocObjectAsList(name string, b assocListBinding, what, rejection string) string {
	if rejection == "" {
		rejection = fmt.Sprintf("CE0100 \"'%s' is of type %s, but should be of type List.\"", name, b.elem)
	}
	why := "a Reference followed from its FROM end"
	if b.selfReference {
		why += ", which is how Mendix follows a self-reference"
	}
	return fmt.Sprintf("$%s = retrieve over %s from $%s (a %s) is one %s — %s — so %s cannot take it: "+
		"mxbuild rejects it with %s [MDL-RETRIEVE01]. Use $%s as the one object it is (`$%s/…`, `change $%s (…)`); "+
		"a list of %s needs a Reference set, or a retrieve from the database: `retrieve $%s from %s where […];`.",
		name, b.assoc, b.start, b.startEntity, b.elem, why, what, rejection, name, name, name, b.elem, name, b.elem)
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

// assocRetrieveShape is what a retrieve over an association binds.
type assocRetrieveShape struct {
	qn            string // the association, qualified as the project spells it
	other         string // the entity at the end the retrieve does not start from
	fromTo        bool   // it starts from the association's TO end
	isList        bool   // it binds a list, not one object
	selfReference bool
}

// associationRetrieveShape decides what a retrieve over assoc from an object of
// startEntity binds, by associationRetrieveEnd and associationRetrieveYieldsList
// — the rules the full build takes. known is false whenever that was not
// established.
func associationRetrieveShape(ctx *ExecContext, sc *scriptContext, shapes map[string]assocShape,
	assoc ast.QualifiedName, startEntity string) (shape assocRetrieveShape, known bool) {

	s, ok := shapes[strings.ToLower(qualifyAssoc(ctx, shapes, assoc, startEntity))]
	if !ok || s.owner == "" {
		return assocRetrieveShape{}, false
	}
	// The start may be a specialization of an end; read as much of the
	// generalization chain as can be read, through the script's entities first
	// (a specialization the same script creates is not in the project yet).
	chain, _ := scriptGeneralizationChain(ctx, sc, startEntity)
	if len(chain) == 0 {
		chain = []string{startEntity}
	}
	isA := func(qn string) bool {
		for _, link := range chain {
			if strings.EqualFold(link, qn) {
				return true
			}
		}
		return false
	}
	self := strings.EqualFold(s.from, s.to)
	ownerBoth := s.owner == "Both"
	switch associationRetrieveEnd(isA(s.from), isA(s.to), self) {
	case assocEndFrom:
		return assocRetrieveShape{qn: s.qn, other: s.to, selfReference: self,
			isList: associationRetrieveYieldsList(s.refSet, ownerBoth, false)}, true
	case assocEndTo:
		return assocRetrieveShape{qn: s.qn, other: s.from, fromTo: true, selfReference: self,
			isList: associationRetrieveYieldsList(s.refSet, ownerBoth, true)}, true
	}
	return assocRetrieveShape{}, false
}

func assocListSuggestion(name string, b assocListBinding) string {
	if b.selfReference {
		// Which way a database constraint over a self-reference runs is its own
		// question; keep to the list the retrieve already has.
		return fmt.Sprintf("Keep the list and work on its items (`loop $item in $%s`, or `$item = head($%s);`).", name, name)
	}
	return fmt.Sprintf("Keep the list and work on its items (`loop $item in $%s`, or `$item = head($%s);`), "+
		"or retrieve one %s from the database: `retrieve $%s from %s where [%s = $%s] first;`.",
		name, name, b.elem, name, b.elem, b.assoc, b.start)
}
