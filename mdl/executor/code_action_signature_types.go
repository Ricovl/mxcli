// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// A Java or JavaScript action's signature names entities and enumerations by
// a bare `Module.Name`, and the parser cannot tell the two apart: both arrive
// as TypeEnumeration with EnumRef set and ExplicitEnum false (CLAUDE.md,
// "TypeEnumeration vs TypeEntity Ambiguity"). The converters
// (astDataTypeToJavaActionParamType / …ReturnType) called every such name an
// entity, so `create java action M.JA (C: M.Color)` with M.Color an
// enumeration was written as an entity parameter: check and exec were clean
// and mxbuild reported CE1613 "The selected entity 'M.Color' no longer exists"
// (ako/mxcli#935). A comment there promised a resolveCodeActionParamType that
// would settle the name against the project; it never existed.
//
// settleCodeActionSignature is that resolution, run by exec before anything is
// written; codeActionSignatureErrors is check --references' copy of the
// verdict. Flow signatures have done the same since the entity-then-
// enumeration fallback of buildMicroflowFromStmt.

// settleCodeActionSignature returns params and ret with every bare qualified
// name decided against the project: an entity stays an entity, an enumeration
// becomes an explicit enumeration, and a name that is neither is refused. An
// explicit `enum M.E` is held to enumerations. Unqualified names — type
// parameters, `Microflow`, `Nanoflow` — are left to the converters, and a
// `list of` element is not resolved (exec never refused one). The returned
// slice is a copy; the statement is not changed.
//
// A System name is decided by the System module's enumeration list: its
// entities are not stored in the project, so one that is not an enumeration is
// written by name as before (isBuiltinModuleEntity).
func settleCodeActionSignature(ctx *ExecContext, what string, params []ast.JavaActionParam, ret ast.DataType) ([]ast.JavaActionParam, ast.DataType, error) {
	var entities map[string]bool // built on first use: most signatures are primitive
	settle := func(dt ast.DataType, where string) (ast.DataType, error) {
		qn := codeActionTypeName(dt)
		if qn == nil {
			return dt, nil
		}
		name := qn.String()
		asEnumeration := ast.DataType{Kind: ast.TypeEnumeration, EnumRef: qn, ExplicitEnum: true}
		if dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum {
			if isKnownEnumeration(ctx, qn.Module, qn.Name) {
				return dt, nil
			}
			return dt, mdlerrors.NewNotFoundMsg("enumeration", name,
				fmt.Sprintf("%s: %s is typed enum %s, and there is no such enumeration", what, where, name))
		}
		asEntity := ast.DataType{Kind: ast.TypeEntity, EntityRef: qn}
		if isBuiltinModuleEntity(qn.Module) {
			if systemEnumerations[name] {
				return asEnumeration, nil
			}
			return asEntity, nil
		}
		if entities == nil {
			entities = buildEntityQualifiedNames(ctx)
		}
		if entities[name] {
			return asEntity, nil
		}
		if findEnumeration(ctx, qn.Module, qn.Name) != nil {
			return asEnumeration, nil
		}
		return dt, mdlerrors.NewNotFoundMsg("entity or enumeration", name,
			fmt.Sprintf("%s: %s is typed %s, which is neither an entity nor an enumeration "+
				"(written as an entity, mxbuild reports CE1613)", what, where, name))
	}

	out := make([]ast.JavaActionParam, len(params))
	copy(out, params)
	for i := range out {
		dt, err := settle(out[i].Type, "parameter "+out[i].Name)
		if err != nil {
			return nil, ret, err
		}
		out[i].Type = dt
	}
	settledRet, err := settle(ret, "the return type")
	if err != nil {
		return nil, ret, err
	}
	return out, settledRet, nil
}

// codeActionTypeName returns the qualified entity-or-enumeration name a
// code-action signature type names, or nil for any other type — a primitive,
// a list, an `entity <T>` selector, or an unqualified name (a type parameter,
// `Microflow`, `Nanoflow`, `void`).
func codeActionTypeName(dt ast.DataType) *ast.QualifiedName {
	var qn *ast.QualifiedName
	switch dt.Kind {
	case ast.TypeEntity:
		qn = dt.EntityRef
	case ast.TypeEnumeration:
		qn = dt.EnumRef
	}
	if qn == nil || qn.Module == "" || qn.Name == "" {
		return nil
	}
	return qn
}

// codeActionSignatureErrors is check --references' copy of
// settleCodeActionSignature's refusals: a bare name resolves when it is an
// entity or an enumeration of the project or of the script, and an explicit
// `enum` when it is an enumeration. kind is "java action" or "javascript
// action".
func codeActionSignatureErrors(ctx *ExecContext, sc *scriptContext, kind string, name ast.QualifiedName,
	params []ast.JavaActionParam, ret ast.DataType) []string {
	if !ctx.Connected() {
		return nil
	}
	r := newDeclaredTypeResolver(ctx, sc)
	checkType := func(dt ast.DataType, where string) string {
		qn := codeActionTypeName(dt)
		if qn == nil {
			return ""
		}
		if !(dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum) {
			// A bare name: either kind resolves it, and the message says so.
			dt = ast.DataType{Kind: ast.TypeEntity, EntityRef: qn}
		}
		return r.check(dt, where)
	}
	var errs []string
	for _, p := range params {
		if msg := checkType(p.Type, fmt.Sprintf("parameter %s of %s %s", p.Name, kind, name.String())); msg != "" {
			errs = append(errs, msg)
		}
	}
	if msg := checkType(ret, fmt.Sprintf("return type of %s %s", kind, name.String())); msg != "" {
		errs = append(errs, msg)
	}
	return errs
}
