// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// flowMemberRefs is what check --references knows while it walks a script's
// flows in order, for the member questions member_verdict.go answers: the
// project, and what the script declares.
//
// Entities are taken from the whole script — a flow's parameters name them, so
// exec cannot reach the flow before they exist. Associations only from the
// statements BEFORE the flow, because that is all exec's builder sees when it
// resolves a bare association name: one declared further down resolves at
// check time and is refused at exec time otherwise.
type flowMemberRefs struct {
	ctx      *ExecContext
	sc       *scriptContext
	declared map[string]assocShape // associations of the statements so far
	project  *associationCatalog   // read on first use
	merged   *associationCatalog   // project + declared, rebuilt when declared grows
}

func newFlowMemberRefs(ctx *ExecContext, sc *scriptContext) *flowMemberRefs {
	return &flowMemberRefs{ctx: ctx, sc: sc, declared: map[string]assocShape{}}
}

// declare records the associations stmt creates, for the flows after it.
func (r *flowMemberRefs) declare(stmt ast.Statement) {
	switch stmt.(type) {
	case *ast.CreateAssociationStmt, *ast.CreateViewEntityStmt:
	default:
		return
	}
	one := newScriptContext()
	one.collectSingle(stmt)
	for k, v := range one.associationShapes {
		r.declared[k] = v
		r.merged = nil
	}
}

// catalog is the project's associations overlaid with the declared ones.
func (r *flowMemberRefs) catalog() associationCatalog {
	if r.merged != nil {
		return *r.merged
	}
	if r.project == nil {
		cat := checkAssociationCatalog(r.ctx, nil)
		r.project = &cat
	}
	merged := associationCatalog{shapes: make(map[string]assocShape, len(r.project.shapes)+len(r.declared)),
		unnamed: r.project.unnamed, complete: r.project.complete}
	for k, v := range r.project.shapes {
		merged.shapes[k] = v
	}
	for k, v := range r.declared {
		merged.shapes[k] = v
	}
	r.merged = &merged
	return merged
}

// chain is entityQN's generalization chain through the script's declarations
// and then the project.
func (r *flowMemberRefs) chain(entityQN string) ([]string, bool) {
	var chain []string
	seen := map[string]bool{}
	for qn := entityQN; qn != ""; {
		if seen[qn] {
			return chain, false
		}
		seen[qn] = true
		if r.sc != nil && r.sc.entities[qn] {
			chain = append(chain, qn)
			qn = r.sc.entityGeneralizations[strings.ToLower(qn)]
			continue
		}
		rest, complete := generalizationChain(r.ctx, qn)
		return append(chain, rest...), complete
	}
	return chain, true
}

// bareRetrieveAssociation reports `retrieve $X from $V/Name` when Name, a bare
// association name, does not resolve to exactly one association from the
// entity $V holds (ako/mxcli#1690). It is silent where the entity is not known
// or the model could not be read: exec decides those, and refuses them too,
// since a bare name cannot be written.
func (r *flowMemberRefs) bareRetrieveAssociation(s *ast.RetrieveStmt, vars map[string]string) string {
	if s.StartVariable == "" || s.Source.Module != "" || s.Source.Name == "" {
		return ""
	}
	entity := vars[strings.TrimPrefix(s.StartVariable, "$")]
	if entity == "" {
		return ""
	}
	chain, complete := r.chain(entity)
	_, matches, verdict := resolveBareAssociation(s.Source.Name, chain, complete, r.catalog())
	if verdict != bareAssocMissing && verdict != bareAssocAmbiguous {
		return ""
	}
	return bareRetrieveAssociationRefusal(s.Variable, s.StartVariable, s.Source.Name, entity, matches, verdict)
}

// statementMembers is what one flow statement names that its entity does not
// have, each report prefixed with the flow's name.
func (r *flowMemberRefs) statementMembers(flowQN string, st ast.MicroflowStatement, vars map[string]string) []string {
	var out []string
	report := func(msg string) {
		if msg != "" {
			out = append(out, flowQN+": "+msg)
		}
	}
	switch s := st.(type) {
	case *ast.RetrieveStmt:
		report(r.bareRetrieveAssociation(s, vars))
	case *ast.ListOperationStmt:
		for _, m := range listOperationMembers(s) {
			report(r.listMember(m.verb, s.InputVariable, vars[s.InputVariable], m.name))
		}
	case *ast.AggregateListStmt:
		if !s.IsExpression && s.Attribute != "" {
			report(r.listMember(strings.ToLower(s.Operation.String()), s.InputVariable, vars[s.InputVariable], s.Attribute))
		}
	}
	return out
}

type listOpMember struct{ verb, name string }

// listOperationMembers are the members a find, filter or sort names: the left
// of a by-member find / filter (the by-expression form is an expression), and
// each sort key.
func listOperationMembers(s *ast.ListOperationStmt) []listOpMember {
	switch s.Operation {
	case ast.ListOpFind, ast.ListOpFilter:
		if s.ByExpression || !ast.IsMemberEquality(s.Condition) {
			return nil
		}
		name, ok := listOperationFieldName(s.Condition.(*ast.BinaryExpr).Left)
		if !ok {
			return nil
		}
		return []listOpMember{{strings.ToLower(s.Operation.String()), name}}
	case ast.ListOpSort:
		var out []listOpMember
		for _, spec := range s.SortSpecs {
			out = append(out, listOpMember{"sort", spec.Attribute})
		}
		return out
	}
	return nil
}

// listMember reports a list operation's or aggregate's bare member that the
// list's entity does not have (ako/mxcli#1646), in the words exec's builder
// refuses it with (refuseMissingListMember). A qualified member is a different
// question — validateListOperationSystemMembers and the builder's association
// lookup answer it — and an untyped list is not judged.
func (r *flowMemberRefs) listMember(verb, list, entityQN, name string) string {
	if entityQN == "" || name == "" || strings.Contains(name, ".") {
		return ""
	}
	if res, _ := r.member(entityQN, name, !isAggregateVerb(verb)); res != memberMissing {
		return ""
	}
	return listMemberRefusal(listMemberWhere(verb, list, name), entityQN, name, r.nearMembers(entityQN))
}

// isAggregateVerb reports an aggregate list function: one that cannot read a
// system date (listMemberRefusal).
func isAggregateVerb(verb string) bool {
	switch verb {
	case "sum", "average", "minimum", "maximum":
		return true
	}
	return false
}

// member is flowMemberVerdict through the script: the same walk
// (memberVerdictOver), reading each link from the script where the script
// declares it — exec's builder will find exactly that in the project — and
// from the project otherwise. A link the script alters is not judged: the
// alter may add the member. An association the statements before the flow
// create counts as a member of its module, as a stored one does.
func (r *flowMemberRefs) member(entityQN, name string, systemDates bool) (memberResolution, memberKind) {
	return memberVerdictOver(scriptMemberModel{r}, entityQN, name, systemDates)
}

type scriptMemberModel struct{ r *flowMemberRefs }

func (m scriptMemberModel) memberLink(entityQN, name string) (memberLinkFacts, bool) {
	r := m.r
	if r.sc != nil && r.sc.alteredEntities[entityQN] {
		return memberLinkFacts{}, false
	}
	var decl *ast.CreateEntityStmt
	if r.sc != nil {
		decl = r.sc.entityDecls[entityQN]
	}
	if decl == nil {
		facts, ok := backendMemberModel{r.backend()}.memberLink(entityQN, name)
		if ok && !facts.association {
			facts.association = r.declaredAssociationInModule(entityQN, name)
		}
		return facts, ok
	}
	var facts memberLinkFacts
	if decl.Generalization != nil {
		facts.generalization = decl.Generalization.String()
	}
	for _, a := range decl.Attributes {
		switch {
		case isAutoSystemMemberType(a.Type.Kind):
			if systemMemberOfPseudoType(a.Type.Kind) == name {
				facts.storesDate = true
			}
		case a.Name == name:
			facts.attribute = true
		}
	}
	facts.association = r.declaredAssociationInModule(entityQN, name) || r.projectAssociationInModule(entityQN, name)
	return facts, true
}

func (r *flowMemberRefs) backend() entityLookupBackend {
	if r.ctx == nil {
		return nil
	}
	b, _ := r.ctx.Backend.(entityLookupBackend)
	return b
}

// declaredAssociationInModule reports an association of this name that a
// statement before the flow creates in entityQN's module.
func (r *flowMemberRefs) declaredAssociationInModule(entityQN, name string) bool {
	module, _, _ := strings.Cut(entityQN, ".")
	_, ok := r.declared[strings.ToLower(module+"."+name)]
	return ok
}

// projectAssociationInModule reports a stored association of this name in
// entityQN's module, for an entity the script (re)declares.
func (r *flowMemberRefs) projectAssociationInModule(entityQN, name string) bool {
	module, _, _ := strings.Cut(entityQN, ".")
	_, ok := r.catalog().shapes[strings.ToLower(module+"."+name)]
	return ok
}

// nearMembers is nearMembersFor through the script's declarations.
func (r *flowMemberRefs) nearMembers(entityQN string) string {
	if r.sc != nil {
		if decl := r.sc.entityDecls[entityQN]; decl != nil {
			var names []string
			for _, a := range decl.Attributes {
				if !isAutoSystemMemberType(a.Type.Kind) {
					names = append(names, a.Name)
				}
			}
			return nearMemberList(names)
		}
	}
	return nearMembersFor(r.backend(), entityQN)
}
