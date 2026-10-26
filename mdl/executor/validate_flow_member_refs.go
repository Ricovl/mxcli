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
	if s, ok := st.(*ast.RetrieveStmt); ok {
		report(r.bareRetrieveAssociation(s, vars))
	}
	return out
}
