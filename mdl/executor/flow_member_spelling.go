// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// describedMemberSpellings returns a copy of a declared flow body with every
// entity member it names spelled the way describe prints the member it stores,
// wherever the builder provably stores the same member for both spellings.
//
// diff-then-patch compares the declared statements with the stored flow's
// description (declaredMatches), and describe prints a member short: a sort's
// `M.Dept.SortOrder` as `SortOrder`, a create's `M.Emp_Dept` as `Emp_Dept`, a
// find's `M.Emp_Dept = $d` as `Emp_Dept = $d`, and a change's association in
// full. A script naming the same member in the other spelling never matched
// its own stored activity, so every run replaced it (writing under mdl 0 and
// mdl 1 alike), and inside a loop body or beside an end event the phantom
// change was refused under mdl 1 (ako/mxcli#859, rehearsal S3/S6/S7).
//
// A spelling is only changed when resolving both through the builder's own
// rules gives the same stored member, so the copy states exactly what the
// original does: it can be built instead of it, and a real change of member —
// another attribute, or a same-named association of another module — still
// compares as a change.
func describedMemberSpellings(ctx *ExecContext, params []ast.MicroflowParam, body []ast.MicroflowStatement) []ast.MicroflowStatement {
	if !ctx.Connected() || len(body) == 0 {
		return body
	}
	body = cloneAST(reflect.ValueOf(body)).Interface().([]ast.MicroflowStatement)
	s := &memberSpeller{ctx: ctx, fb: &flowBuilder{backend: ctx.Backend}, entities: map[string]string{}}
	for _, p := range params {
		if p.Type.EntityRef != nil && (p.Type.Kind == ast.TypeEntity || p.Type.Kind == ast.TypeListOf) {
			s.bind(p.Name, p.Type.EntityRef.String())
		}
	}
	eachFlowStatement(body, s.collect)
	eachFlowStatement(body, s.respell)
	return body
}

// memberSpeller rewrites member names to describe's spelling. entities maps a
// variable to the entity it holds, or the entity of the list it holds.
type memberSpeller struct {
	ctx      *ExecContext
	fb       *flowBuilder
	entities map[string]string
}

func (s *memberSpeller) bind(v, entity string) {
	if _, ok := s.entities[v]; !ok && v != "" && entity != "" {
		s.entities[v] = entity
	}
}

// collect records the entity each declared variable holds, where the
// statement states it.
func (s *memberSpeller) collect(st ast.MicroflowStatement) {
	switch x := st.(type) {
	case *ast.RetrieveStmt:
		if x.StartVariable == "" {
			s.bind(x.Variable, x.Source.String())
		}
	case *ast.CreateObjectStmt:
		s.bind(x.Variable, x.EntityType.String())
	case *ast.CreateListStmt:
		s.bind(x.Variable, x.EntityType.String())
	case *ast.DeclareStmt:
		if x.Type.EntityRef != nil {
			s.bind(x.Variable, x.Type.EntityRef.String())
		}
	case *ast.LoopStmt:
		s.bind(x.LoopVariable, s.entities[x.ListVariable])
	case *ast.ListOperationStmt:
		s.bind(x.OutputVariable, s.entities[x.InputVariable])
	}
}

func (s *memberSpeller) respell(st ast.MicroflowStatement) {
	switch x := st.(type) {
	case *ast.RetrieveStmt:
		if x.StartVariable != "" {
			return
		}
		for i := range x.SortColumns {
			col := &x.SortColumns[i]
			if len(col.Associations) == 0 {
				col.Attribute = shortSortAttribute(s.ctx, x.Source.String(), col.Attribute)
			}
		}
	case *ast.CreateObjectStmt:
		entity := x.EntityType.String()
		for i := range x.Changes {
			x.Changes[i].Attribute = s.member(entity, x.Changes[i].Attribute, func(attr, assoc string) string {
				if assoc != "" {
					if mod, name, ok := strings.Cut(assoc, "."); ok && mod == x.EntityType.Module {
						return name
					}
					return assoc
				}
				return lastSegment(attr)
			})
		}
	case *ast.ChangeObjectStmt:
		entity := s.entities[x.Variable]
		for i := range x.Changes {
			x.Changes[i].Attribute = s.member(entity, x.Changes[i].Attribute, func(attr, assoc string) string {
				if assoc != "" {
					return assoc
				}
				return lastSegment(attr)
			})
		}
	case *ast.ListOperationStmt:
		entity := s.entities[x.InputVariable]
		if entity == "" {
			return
		}
		if (x.Operation == ast.ListOpFind || x.Operation == ast.ListOpFilter) && !x.ByExpression && ast.IsMemberEquality(x.Condition) {
			binary := x.Condition.(*ast.BinaryExpr)
			name, ok := listOperationFieldName(binary.Left)
			if !ok {
				return
			}
			short := s.member(entity, name, func(attr, assoc string) string {
				return lastSegment(attr + assoc)
			})
			if short != name {
				binary.Left = &ast.IdentifierExpr{Name: short}
			}
		}
		for i := range x.SortSpecs {
			// The builder qualifies a bare name with the list's entity.
			if a := x.SortSpecs[i].Attribute; a == entity+"."+lastSegment(a) {
				x.SortSpecs[i].Attribute = lastSegment(a)
			}
		}
	}
}

// member returns describe's spelling of the member name resolves to on
// entity, as spell gives it from the resolved attribute or association, when
// that spelling resolves to the same member; otherwise name as written.
func (s *memberSpeller) member(entity, name string, spell func(attr, assoc string) string) string {
	if entity == "" || name == "" {
		return name
	}
	attr, assoc, ok := s.resolve(entity, name)
	if !ok {
		return name
	}
	short := spell(attr, assoc)
	if short == name {
		return name
	}
	if a2, s2, ok := s.resolve(entity, short); !ok || a2 != attr || s2 != assoc {
		return name
	}
	return short
}

func (s *memberSpeller) resolve(entity, name string) (attr, assoc string, ok bool) {
	s.fb.errors = nil
	mc := &microflows.MemberChange{}
	s.fb.resolveMemberChange(mc, name, entity)
	if len(s.fb.errors) > 0 {
		return "", "", false
	}
	return mc.AttributeQualifiedName, mc.AssociationQualifiedName, true
}

// eachFlowStatement calls fn for every statement of body, at any depth, a
// statement before the statements it holds.
func eachFlowStatement(body []ast.MicroflowStatement, fn func(ast.MicroflowStatement)) {
	stmtType := reflect.TypeOf((*ast.MicroflowStatement)(nil)).Elem()
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if rv.IsNil() {
				return
			}
			if rv.Type() == stmtType {
				fn(rv.Interface().(ast.MicroflowStatement))
			}
			walk(rv.Elem())
		case reflect.Pointer:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(body))
}

// cloneAST deep-copies an AST value: every pointer, slice and map is new, so
// the copy can be rewritten without touching the statement it came from.
func cloneAST(rv reflect.Value) reflect.Value {
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return rv
		}
		out := reflect.New(rv.Type()).Elem()
		out.Set(cloneAST(rv.Elem()))
		return out
	case reflect.Pointer:
		if rv.IsNil() {
			return rv
		}
		out := reflect.New(rv.Type().Elem())
		out.Elem().Set(cloneAST(rv.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(rv.Type()).Elem()
		out.Set(rv)
		for i := 0; i < rv.NumField(); i++ {
			if rv.Type().Field(i).IsExported() {
				out.Field(i).Set(cloneAST(rv.Field(i)))
			}
		}
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out.Index(i).Set(cloneAST(rv.Index(i)))
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		for _, k := range rv.MapKeys() {
			out.SetMapIndex(k, cloneAST(rv.MapIndex(k)))
		}
		return out
	}
	return rv
}
