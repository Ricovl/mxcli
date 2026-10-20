// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// Only what an ALTER NAVIGATION / ALTER MENU says is validated: the items its
// fragments add, the OnClick and Icon a `set ( … )` writes, and the page
// clauses it sets. Untouched stored items are not re-validated — that is what
// made one caption change in a Studio Pro menu refuse over a glyph on an item
// it never named (ako/TestApp, MDL078).
//
// The checks are the ones `create navigation` / `create menu` run, reached
// through a view: a CreateNavigationStmt / CreateMenuStmt holding just what the
// alter states. Every reference pass that switches on the create statements
// asks menuAlterView first, so an alter is checked by the same code.

// menuAlterFragmentItems are the items the operations add as new content.
func menuAlterFragmentItems(ops []ast.MenuAlterOp) []ast.NavMenuItemDef {
	var out []ast.NavMenuItemDef
	for _, op := range ops {
		switch o := op.(type) {
		case *ast.InsertMenuItemsOp:
			out = append(out, o.Items...)
		case *ast.ReplaceMenuItemOp:
			out = append(out, o.Items...)
		}
	}
	return out
}

// menuAlterSetItems is one item per `set ( … )` that writes an OnClick or an
// Icon, carrying just those, so their targets and icons are checked.
func menuAlterSetItems(ops []ast.MenuAlterOp) []ast.NavMenuItemDef {
	var out []ast.NavMenuItemDef
	for _, op := range ops {
		if o, ok := op.(*ast.SetMenuItemOp); ok && (o.SetAction || o.SetIcon) {
			it := o.Item
			if n := len(o.Target.Segments); n > 0 {
				it.Caption = o.Target.Segments[n-1].Caption
			}
			if !o.SetAction {
				it.Action, it.Page, it.Microflow, it.SignOut = nil, nil, nil, false
			}
			out = append(out, it)
		}
	}
	return out
}

// menuAlterView is the create statement that states what an ALTER NAVIGATION
// or ALTER MENU states, for the reference checks; nil for any other statement.
func menuAlterView(stmt ast.Statement) ast.Statement {
	switch s := stmt.(type) {
	case *ast.AlterNavigationStmt:
		v := &ast.CreateNavigationStmt{
			ProfileName:    s.ProfileName,
			MenuItems:      append(menuAlterFragmentItems(s.Operations), menuAlterSetItems(s.Operations)...),
			CreateOrModify: true,
		}
		for _, op := range s.Operations {
			if o, ok := op.(*ast.SetNavPageClauseOp); ok {
				switch o.Clause {
				case "home":
					v.HomePages = append(v.HomePages, o.Home)
				case "login":
					p := o.Page
					v.LoginPage = &p
				case "notfound":
					p := o.Page
					v.NotFoundPage = &p
				}
			}
		}
		return v
	case *ast.AlterMenuStmt:
		return &ast.CreateMenuStmt{
			Name:           s.Name,
			Items:          append(menuAlterFragmentItems(s.Operations), menuAlterSetItems(s.Operations)...),
			CreateOrModify: true,
		}
	}
	return nil
}

// validateMenuAlterForExec runs, before anything is written, the reference
// checks exec runs for `create navigation`: the user roles of `set home … for`
// (a module-qualified role makes the project unloadable, mendixlabs/mxcli#1001)
// and the pages and flows the fragments and page clauses name (CE1613).
func validateMenuAlterForExec(ctx *ExecContext, stmt ast.Statement) error {
	view := menuAlterView(stmt)
	if nv, ok := view.(*ast.CreateNavigationStmt); ok {
		if err := validateNavigationRoleForExec(ctx, nv); err != nil {
			return err
		}
	}
	return validateNavigationTargetsForExec(ctx, view)
}

// validateAlterNavigationReferences is `check -p --references` for ALTER
// NAVIGATION / ALTER MENU: every statement is run through applyMenuAlterOps —
// the function exec runs — on a mutator over the stored document that is never
// saved, so every target exec would refuse is reported with exec's message.
// Successive alters of one document share the mutator, so a later statement
// sees what an earlier one inserted.
//
// "Could not establish" stays silent: a document the script creates or
// rewrites before the alter (its stored state is not what the alter will
// meet), one that will not open, and anything after a refusal on the same
// document.
func validateAlterNavigationReferences(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	opened := map[string]backend.MenuMutator{}
	untrusted := map[string]bool{}
	var errs []error
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateNavigationStmt:
			untrusted["navigation "+strings.ToLower(s.ProfileName)] = true
		case *ast.CreateMenuStmt:
			untrusted["menu "+s.Name.String()] = true
		case *ast.DropMenuStmt:
			untrusted["menu "+s.Name.String()] = true
		case *ast.AlterNavigationStmt:
			key := "navigation " + strings.ToLower(s.ProfileName)
			if untrusted[key] {
				continue
			}
			m, ok := opened[key]
			if !ok {
				pm, err := openAlterNavigation(ctx, s.ProfileName)
				if err != nil {
					untrusted[key] = true
					if !isBackendError(err) {
						errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					}
					continue
				}
				m, opened[key] = pm, pm
			}
			if err := applyMenuAlterOps(ctx, m, alterNavigationLabel(s.ProfileName), s.Operations); err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
				untrusted[key] = true
			}
		case *ast.AlterMenuStmt:
			key := "menu " + s.Name.String()
			if untrusted[key] {
				continue
			}
			m, ok := opened[key]
			if !ok {
				mm, err := openAlterMenu(ctx, s.Name)
				if err != nil {
					untrusted[key] = true
					if !isBackendError(err) {
						errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					}
					continue
				}
				m, opened[key] = mm, mm
			}
			if err := applyMenuAlterOps(ctx, m, "alter menu "+s.Name.String(), s.Operations); err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
				untrusted[key] = true
			}
		}
	}
	return errs
}

// isBackendError reports a failure to read or open, which check cannot judge.
func isBackendError(err error) bool {
	_, ok := err.(*mdlerrors.BackendError)
	return ok
}
