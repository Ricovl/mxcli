// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateReservedParameterNames (MDL-PARAM01) refuses a page, snippet, Java
// action or JavaScript action parameter named with a word the Mendix platform
// reserves (ako/mxcli#1651, mirroring mendixlabs/mxcli#1642). mxbuild reports it
// as
//
//	CE7247 "The name 'Context' is a reserved word." at Page parameter 'Context'
//
// after exec has written the page and every caller that passes the parameter by
// name — so renaming it then means changing all of them.
//
// The list is types.PlatformReservedWords, the one attribute names (MDL021) and
// enumeration values (MDL010) are held to; nothing here keeps a second one.
// Measured on mx 11.13.0 in a blank app, one element per name and context: every
// listed name tried (Java keywords such as `Class`, `New`, `this`, `int`,
// `null`, `true`, `Long`, `Package`; Mendix's own `Context`, `CurrentUser`,
// `GUID`, `Id`, `MendixObject`, `Owner`, `Type`, `con`, `submetaobjectname`,
// `ChangedDate`, `changedby`, `createddate`) is CE7247 as a page parameter, a
// snippet parameter and a Java action parameter, and `Context`/`Type` as a
// JavaScript action parameter — in any case (`Class`, `New`, `Id` as well as
// `int`, `this`), and a quoted name is stored, and refused, as its bare self. `Object` was the one reserved name the list
// lacked (CE7247 in all of those places, and for an attribute, an enumeration
// value and an entity), and was added to it. Names not on it built clean: the
// system variables `currentObject`, `latestError`, `latestHttpResponse`,
// `latestSoapFault`, `currentSession`, `currentDeviceType`, and `String`,
// `System`, `User`, `List`, `Var`, `record`, `currentPage`, `Session`.
//
// A microflow or nanoflow parameter is NOT judged against this list: measured,
// `$Context`, `$Type`, `$Class` and the rest build clean there. A flow parameter
// or variable is refused for a different set of names (the system variables:
// CE0110 "Variable name 'latestError' is reserved.", CE0039 for a variable named
// `CurrentUser`), which this rule does not cover.
func validateReservedParameterNames(stmt ast.Statement) []linter.Violation {
	type param struct {
		name   string
		entity string // the parameter's entity, for the suggested name
	}
	var (
		kind   string
		owner  ast.QualifiedName
		dollar = "$"
		params []param
	)
	pageParams := func(ps []ast.PageParameter) {
		for _, p := range ps {
			params = append(params, param{name: p.Name, entity: parameterEntityName(p.EntityType, p.Type)})
		}
	}
	codeParams := func(ps []ast.JavaActionParam) {
		dollar = ""
		for _, p := range ps {
			params = append(params, param{name: p.Name, entity: parameterEntityName(ast.QualifiedName{}, p.Type)})
		}
	}
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		kind, owner = "page", s.Name
		pageParams(s.Parameters)
	case *ast.CreateSnippetStmtV3:
		kind, owner = "snippet", s.Name
		pageParams(s.Parameters)
	case *ast.AlterPageStmt:
		kind, owner = strings.ToLower(s.ContainerType), s.PageName
		if kind != "page" && kind != "snippet" {
			return nil
		}
		for _, op := range s.Operations {
			if add, ok := op.(*ast.AddParameterOp); ok {
				pageParams([]ast.PageParameter{add.Parameter})
			}
		}
	case *ast.CreateJavaActionStmt:
		kind, owner = "java action", s.Name
		codeParams(s.Parameters)
	case *ast.CreateJavaScriptActionStmt:
		kind, owner = "javascript action", s.Name
		codeParams(s.Parameters)
	default:
		return nil
	}

	var out []linter.Violation
	for _, p := range params {
		name := strings.Trim(strings.TrimSpace(p.name), `"`)
		if !types.IsPlatformReserved(name) {
			continue
		}
		alt := p.entity
		if alt == "" || types.IsPlatformReserved(alt) || strings.EqualFold(alt, name) {
			alt = name + "Value"
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-PARAM01",
			Severity: linter.SeverityError,
			Location: linter.Location{Module: owner.Module, DocumentType: kind, DocumentName: owner.Name},
			Message: fmt.Sprintf("%s parameter %s%s of %s is a reserved word — mxbuild rejects it with "+
				"CE7247 \"The name '%s' is a reserved word.\"", kind, dollar, name, owner.String(), name),
			Suggestion: fmt.Sprintf("rename it, e.g. %s%s, and pass it under that name wherever the %s is "+
				"called. Quoting does not help: the platform checks the bare name.", dollar, alt, kind),
		})
	}
	return out
}

// parameterEntityName is the entity a parameter carries, "" for a primitive.
// A bare `Module.Name` parses as an enumeration reference (see
// TypeEnumeration vs TypeEntity in CLAUDE.md), so that is read too.
func parameterEntityName(entity ast.QualifiedName, t ast.DataType) string {
	switch {
	case entity.Name != "":
		return entity.Name
	case t.EntityRef != nil:
		return t.EntityRef.Name
	case t.EnumRef != nil && !t.ExplicitEnum:
		return t.EnumRef.Name
	}
	return ""
}

// refuseReservedParameterNames is the same verdict in the exec handlers, so a
// script run with --no-check cannot write what check refuses.
func refuseReservedParameterNames(stmt ast.Statement) error {
	if v := firstBlockingViolation(validateReservedParameterNames(stmt)); v != nil {
		return mdlerrors.NewValidationf("%s — %s", v.Message, v.Suggestion)
	}
	return nil
}
