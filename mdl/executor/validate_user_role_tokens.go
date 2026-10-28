// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// An XPath constraint names a user role with a token, `'[%UserRole_Manager%]'`:
// the documented way to target a role's users from a user task, and as valid in
// a retrieve, a data source or an access rule. The name after `UserRole_` must
// be one of the project's user roles, or mxbuild refuses the constraint — and
// nothing asked, so a typo passed `check --references` and exec wrote it
// (ako/mxcli#1723). Measured on mxbuild 11.15.0, the same script with the role
// as the only variable:
//
//	'[%UserRole_User%]'      clean
//	'[%UserRole_Manager%]'   CE1613 "The selected user role 'Manager' no longer exists."
//	'[%UserRole_user%]'      CE1613 — the name is matched case-sensitively
//
// at an access rule, a list view's database source and a retrieve alike (and a
// workflow's targeting, #1711). It costs more than the one error: with a CE1613
// anywhere in the app, mx check reported no CE0161 at all.
//
// Could-not-establish is silence: without a project the roles are unknown, and a
// role the script creates is known.

// userRoleTokenRe matches a user role token and captures the role's name.
var userRoleTokenRe = regexp.MustCompile(`\[%UserRole_([^%\]]+)%\]`)

// validateUserRoleTokens reports the user role tokens naming no user role.
func validateUserRoleTokens(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	var known map[string]bool // nil until a statement carries a token
	var names []string        // the project's and the script's roles, for the message

	var errs []error
	for i, stmt := range prog.Statements {
		var bad []string
		seen := map[string]bool{}
		walkXPathConstraints(stmt, func(xpath string) {
			for _, m := range userRoleTokenRe.FindAllStringSubmatch(xpath, -1) {
				role := m[1]
				if seen[role] {
					continue
				}
				seen[role] = true
				if known == nil {
					known, names = userRoleNames(ctx, prog)
				}
				if known == nil || known[role] {
					continue
				}
				bad = append(bad, role)
			}
		})
		if len(bad) == 0 {
			continue
		}
		var msgs []string
		for _, role := range bad {
			msg := fmt.Sprintf("[%%UserRole_%s%%]: the project has no user role %s (its user roles: %s)",
				role, role, strings.Join(names, ", "))
			for _, n := range names {
				if strings.EqualFold(n, role) {
					msg += fmt.Sprintf(" — the name is case-sensitive: [%%UserRole_%s%%]", n)
					break
				}
			}
			msgs = append(msgs, msg)
		}
		if kind, name, excluded := excludedFlow(stmt); excluded {
			sc.warnExcluded(kind, name, msgs)
			continue
		}
		errs = append(errs, mdlerrors.NewValidationf("statement %d: unknown user role in an XPath token "+
			"(mxbuild: CE1613 \"The selected user role '…' no longer exists.\"):\n  - %s",
			i+1, strings.Join(msgs, "\n  - ")))
	}
	return errs
}

// userRoleNames is the set of user role names a token may name — the project's
// and those the script creates — and the same names sorted. The set is nil when
// the project's roles cannot be read, which makes every token unjudged.
func userRoleNames(ctx *ExecContext, prog *ast.Program) (map[string]bool, []string) {
	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil || ps == nil {
		return nil, nil
	}
	known := map[string]bool{}
	for _, r := range ps.UserRoles {
		if r != nil {
			known[r.Name] = true
		}
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateUserRoleStmt); ok {
			known[s.Name] = true
		}
	}
	names := make([]string, 0, len(known))
	for n := range known {
		names = append(names, n)
	}
	sort.Strings(names)
	return known, names
}

// walkXPathConstraints calls fn with every XPath constraint v carries, at any
// depth: a database retrieve's, a data source's, an access rule's, a user
// task's targeting (as created and as altered) and an offline sync's.
func walkXPathConstraints(v any, fn func(xpath string)) {
	walkXPathSlots(v, func(_, xpath string) { fn(xpath) })
}

// walkXPathSlots is walkXPathConstraints that also names the slot each
// constraint sits in ("retrieve", "data source", "access rule", "user task
// targeting", "offline sync"), for a message that says where to look.
func walkXPathSlots(v any, fn func(slot, xpath string)) {
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			switch n := rv.Interface().(type) {
			case *ast.RetrieveStmt:
				if n.StartVariable == "" && n.Where != nil {
					fn("retrieve", expressionToXPath(n.Where))
				}
				return
			case *ast.DataSourceV3:
				fn("data source", n.Where)
				return
			case *ast.GrantEntityAccessStmt:
				fn("access rule", n.XPathConstraint)
				return
			case *ast.WorkflowTargetingNode:
				fn("user task targeting", n.XPath)
				return
			case *ast.SetActivityPropertyOp:
				if n.Property == "targeting_xpath" {
					fn("user task targeting", n.Value)
				}
				return
			case *ast.NavSyncDef:
				fn("offline sync", n.Constraint)
				return
			}
			walk(rv.Elem())
		case reflect.Struct:
			if rv.CanAddr() {
				// A node held by value is judged like one held by pointer.
				switch rv.Addr().Interface().(type) {
				case *ast.DataSourceV3, *ast.WorkflowTargetingNode, *ast.NavSyncDef:
					walk(rv.Addr())
					return
				}
			}
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Value())
			}
		}
	}
	walk(reflect.ValueOf(v))
}
