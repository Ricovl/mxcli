// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/mendixlabs/mxcli/mdl/ast"

// flowObjectTypes is what the text of a flow says each variable holds: the
// entity of every variable known to hold one object, and of every variable
// known to hold a list. It needs no project, so it sees only the producers
// written in the flow — an entity parameter, a create object, a database
// retrieve, a create list, a loop over a typed list, a head, filter, sort or
// tail. An association retrieve is an object or a list by the association's
// type and direction, which needs the project, so it is left unknown rather
// than guessed.
//
// Feed it every statement in order (forEachMicroflowStatement) through
// observe, AFTER judging the statement: a name a statement produces is
// whatever that statement makes it, so the statement itself still sees the
// binding before it.
type flowObjectTypes struct {
	objects map[string]string // variable -> entity, for a single object
	lists   map[string]string // variable -> entity, for a list
}

func newFlowObjectTypes(params []ast.MicroflowParam) *flowObjectTypes {
	t := &flowObjectTypes{objects: map[string]string{}, lists: map[string]string{}}
	for _, p := range params {
		if p.Type.EntityRef == nil {
			continue
		}
		if p.Type.Kind == ast.TypeListOf {
			t.lists[p.Name] = p.Type.EntityRef.String()
		} else {
			t.objects[p.Name] = p.Type.EntityRef.String()
		}
	}
	return t
}

// observe records the variables statement s binds.
func (t *flowObjectTypes) observe(s ast.MicroflowStatement) {
	for _, p := range statementProducedVars(s) {
		delete(t.objects, p.name)
		delete(t.lists, p.name)
	}
	switch st := s.(type) {
	case *ast.CreateObjectStmt:
		if st.Variable != "" && st.EntityType.Module != "" {
			t.objects[st.Variable] = st.EntityType.String()
		}
	case *ast.RetrieveStmt:
		if st.Variable != "" && st.StartVariable == "" && st.Source.Module != "" {
			if st.First {
				t.objects[st.Variable] = st.Source.String()
			} else {
				t.lists[st.Variable] = st.Source.String()
			}
		}
	case *ast.CreateListStmt:
		if st.Variable != "" && st.EntityType.Module != "" {
			t.lists[st.Variable] = st.EntityType.String()
		}
	case *ast.LoopStmt:
		if entity, ok := t.lists[st.ListVariable]; ok && st.LoopVariable != "" {
			t.objects[st.LoopVariable] = entity
		}
	case *ast.ListOperationStmt:
		// head is list-only; find is also the String function, so it is left out.
		if entity, ok := t.lists[st.InputVariable]; ok && st.OutputVariable != "" {
			switch st.Operation {
			case ast.ListOpHead:
				t.objects[st.OutputVariable] = entity
			case ast.ListOpFilter, ast.ListOpSort, ast.ListOpTail:
				t.lists[st.OutputVariable] = entity
			}
		}
	}
}
