// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ALTER NAVIGATION / ALTER MENU (accepted proposal
// PROPOSAL_alter_navigation_menu_items): item-level edits to a web profile's
// menu and its page clauses, or to a menu document's items, spliced into the
// stored document (ADR-0012 decision 3) instead of rewriting the whole menu
// the way `create or modify` does.
//
// One function, applyMenuAlterOps, resolves every target, applies every
// refusal and drives the mutator. exec runs it and saves; `check -p` runs it on
// a mutator it never saves (validateAlterNavigationReferences). So the two
// cannot disagree about what a statement means or which one is refused.

// execAlterNavigation handles ALTER NAVIGATION <Profile> { … }.
func execAlterNavigation(ctx *ExecContext, s *ast.AlterNavigationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := validateMenuAlterForExec(ctx, s); err != nil {
		return err
	}
	m, err := openAlterNavigation(ctx, s.ProfileName)
	if err != nil {
		return err
	}
	if err := applyMenuAlterOps(ctx, m, alterNavigationLabel(s.ProfileName), s.Operations); err != nil {
		return err
	}
	if err := m.Save(); err != nil {
		return mdlerrors.NewBackend("save navigation", err)
	}
	ctx.ReportMutation("Altered", "navigation %s", s.ProfileName)
	return nil
}

// execAlterMenu handles ALTER MENU Module.Menu { … }.
func execAlterMenu(ctx *ExecContext, s *ast.AlterMenuStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := validateMenuAlterForExec(ctx, s); err != nil {
		return err
	}
	m, err := openAlterMenu(ctx, s.Name)
	if err != nil {
		return err
	}
	if err := applyMenuAlterOps(ctx, m, "alter menu "+s.Name.String(), s.Operations); err != nil {
		return err
	}
	if err := m.Save(); err != nil {
		return mdlerrors.NewBackend("save menu", err)
	}
	ctx.ReportMutation("Altered", "menu %s", s.Name.String())
	return nil
}

func alterNavigationLabel(profile string) string { return "alter navigation " + profile }

// openAlterNavigation finds the profile and opens its menu for mutation. A
// profile the project does not have, and a native one, are refused with what
// to write instead.
func openAlterNavigation(ctx *ExecContext, profile string) (backend.NavigationProfileMutator, error) {
	// Captions are read and written in the project's default language, as a
	// page's texts are; the backend reads it through model.AuthoringLanguage.
	authoringLanguage(ctx)
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return nil, mdlerrors.NewBackend("get navigation", err)
	}
	var found *types.NavigationProfile
	if nav != nil {
		for _, p := range nav.Profiles {
			if strings.EqualFold(p.Name, profile) {
				found = p
				break
			}
		}
	}
	label := alterNavigationLabel(profile)
	if found == nil {
		names := ""
		if nav != nil {
			names = profileNames(nav)
		}
		return nil, mdlerrors.NewNotFoundMsg("navigation profile", profile, fmt.Sprintf(
			"%s: the project has no navigation profile %s (it has: %s) — alter does not create one; "+
				"`create or modify navigation %s home page Module.Page` does", label, profile, names, profile))
	}
	if found.IsNative {
		return nil, mdlerrors.NewValidationf("%s: %s is a native profile, whose menu is a bottom bar mxcli does not write and whose home "+
			"pages are different elements — alter navigation edits a web profile; set a native profile's home with "+
			"`create or modify navigation %s home page Module.Page` (it keeps everything it does not state), and its bottom bar in Studio Pro",
			label, found.Name, found.Name)
	}
	m, err := ctx.Backend.OpenNavigationProfileForMutation(found.Name)
	if err != nil {
		return nil, mdlerrors.NewBackend("open navigation for mutation", err)
	}
	return m, nil
}

// openAlterMenu finds the menu document and opens its items for mutation.
func openAlterMenu(ctx *ExecContext, name ast.QualifiedName) (backend.MenuMutator, error) {
	authoringLanguage(ctx)
	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(name.Module, name.Name)
	if err != nil || md == nil {
		return nil, mdlerrors.NewNotFoundMsg("menu", name.String(), fmt.Sprintf(
			"alter menu %s: menu not found — `create menu %s { … }` creates one", name, name))
	}
	m, err := ctx.Backend.OpenMenuDocumentForMutation(md.ID)
	if err != nil {
		return nil, mdlerrors.NewBackend("open menu for mutation", err)
	}
	return m, nil
}

// applyMenuAlterOps applies the operations in order to m, each target resolved
// against the menu as the operations before it left it. The first refusal
// stops it; the caller then does not save, so the statement is atomic.
func applyMenuAlterOps(ctx *ExecContext, m backend.MenuMutator, label string, ops []ast.MenuAlterOp) error {
	mb := newMenuActionBuilder(ctx)
	for _, op := range ops {
		if err := applyMenuAlterOp(ctx, mb, m, op); err != nil {
			return mdlerrors.NewValidationf("%s: %s: %v", label, op.String(), err)
		}
	}
	return nil
}

// ce0548 is the refusal for an action on an item with sub-items.
const ce0548 = "an item with sub-items cannot have an action (Mendix CE0548)"

func applyMenuAlterOp(ctx *ExecContext, mb *menuActionBuilder, m backend.MenuMutator, op ast.MenuAlterOp) error {
	tree := m.MenuTree()
	resolve := func(a types.MenuItemAddress) (backend.MenuItemPath, error) {
		return backend.ResolveMenuItemAddress(a, tree)
	}
	switch o := op.(type) {
	case *ast.InsertMenuItemsOp:
		specs, err := menuFragmentSpecs(mb, o.Items)
		if err != nil {
			return err
		}
		if o.Target == nil {
			return m.InsertMenuItems(nil, len(tree), specs)
		}
		path, err := resolve(*o.Target)
		if err != nil {
			return err
		}
		switch o.Position {
		case "before":
			return m.InsertMenuItems(path.Parent(), path.Index(), specs)
		case "after":
			return m.InsertMenuItems(path.Parent(), path.Index()+1, specs)
		default: // into
			n := backend.MenuNodeAt(tree, path)
			if !n.NoAction {
				return fmt.Errorf("%s is not a sub-menu: it has an action, and %s — "+
					"`set ( OnClick: nothing ) on %s` first", o.Target, ce0548, o.Target)
			}
			return m.InsertMenuItems(path, len(n.Items), specs)
		}

	case *ast.ReplaceMenuItemOp:
		path, err := resolve(o.Target)
		if err != nil {
			return err
		}
		specs, err := menuFragmentSpecs(mb, o.Items)
		if err != nil {
			return err
		}
		return m.ReplaceMenuItem(path, specs)

	case *ast.MoveMenuItemOp:
		from, err := resolve(o.Item)
		if err != nil {
			return err
		}
		target, err := resolve(o.Target)
		if err != nil {
			return err
		}
		if target.HasPrefix(from) {
			if len(target) == len(from) {
				return fmt.Errorf("%s and %s are the same item", o.Item, o.Target)
			}
			return fmt.Errorf("%s is inside %s: an item cannot be moved into its own sub-items", o.Target, o.Item)
		}
		pos := backend.MenuPosition(o.Position)
		if pos == backend.MenuInto && !backend.MenuNodeAt(tree, target).NoAction {
			return fmt.Errorf("%s is not a sub-menu: it has an action, and %s — "+
				"`set ( OnClick: nothing ) on %s` first", o.Target, ce0548, o.Target)
		}
		return m.MoveMenuItem(from, target, pos)

	case *ast.DropMenuItemsOp:
		// Every target is resolved against the menu before any is removed, so
		// `drop 'A', 'B'` means the two items the reader sees. An item inside
		// another one dropped goes with it; the rest are removed last-first, so
		// earlier positions stay valid.
		var paths []backend.MenuItemPath
		for _, a := range o.Targets {
			p, err := resolve(a)
			if err != nil {
				return err
			}
			paths = append(paths, p)
		}
		var keep []backend.MenuItemPath
		for i, p := range paths {
			inside := false
			for j, q := range paths {
				if i != j && p.HasPrefix(q) && (len(q) < len(p) || j < i) {
					inside = true
				}
			}
			if !inside {
				keep = append(keep, p)
			}
		}
		sort.Slice(keep, func(i, j int) bool { return pathLess(keep[j], keep[i]) })
		for _, p := range keep {
			if err := m.DropMenuItem(p); err != nil {
				return err
			}
		}
		return nil

	case *ast.SetMenuItemOp:
		path, err := resolve(o.Target)
		if err != nil {
			return err
		}
		node := backend.MenuNodeAt(tree, path)
		if o.SetAction && len(node.Items) > 0 && (o.Item.Action == nil || o.Item.Action.Type != "none") {
			return fmt.Errorf("%s has sub-items, and %s — only `OnClick: nothing` is allowed on it", o.Target, ce0548)
		}
		if o.Caption != nil {
			if err := m.SetMenuItemCaption(path, *o.Caption); err != nil {
				return err
			}
		}
		if o.SetAction {
			action, err := mb.build(o.Item, o.Target.String())
			if err != nil {
				return err
			}
			if err := m.SetMenuItemAction(path, types.NavMenuItemSpec{Caption: node.Caption, Action: action}); err != nil {
				return err
			}
		}
		if o.SetIcon {
			spec := types.NavMenuItemSpec{Icon: o.Item.Icon, IconKind: o.Item.IconKind, IconCode: o.Item.IconCode}
			if err := m.SetMenuItemIcon(path, spec); err != nil {
				return err
			}
		}
		return nil

	case *ast.SetNavPageClauseOp, *ast.DropNavPageClauseOp:
		pm, ok := m.(backend.NavigationProfileMutator)
		if !ok {
			return fmt.Errorf("a menu document has no page clauses; they belong to a navigation profile")
		}
		return applyNavPageClause(pm, op)
	}
	return fmt.Errorf("unsupported operation %T", op)
}

func applyNavPageClause(pm backend.NavigationProfileMutator, op ast.MenuAlterOp) error {
	switch o := op.(type) {
	case *ast.SetNavPageClauseOp:
		switch o.Clause {
		case "home":
			spec := types.NavHomePageSpec{IsPage: o.Home.IsPage, Target: o.Home.Target.String()}
			if o.Home.ForRole != nil {
				spec.ForRole = o.Home.ForRole.String()
			}
			return pm.SetHomePage(spec)
		case "login":
			return pm.SetLoginPage(o.Page.String())
		default:
			return pm.SetNotFoundPage(o.Page.String())
		}
	case *ast.DropNavPageClauseOp:
		switch o.Clause {
		case "home":
			if o.Role == nil {
				return fmt.Errorf("a web profile's default home page is required (Mendix CE0527), so it cannot be dropped — " +
					"`set home page Module.Page` changes it; `drop home page for <Role>` removes a role's")
			}
			return pm.DropRoleHomePage(o.Role.String())
		case "login":
			return pm.SetLoginPage("")
		default:
			return pm.SetNotFoundPage("")
		}
	}
	return nil
}

// pathLess orders positions in describe order.
func pathLess(a, b backend.MenuItemPath) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// menuFragmentSpecs converts a fragment's items the way `create navigation`
// converts its menu block, each action built by the page builder. A fragment
// is new content, so nothing stored is paired with it.
func menuFragmentSpecs(mb *menuActionBuilder, defs []ast.NavMenuItemDef) ([]types.NavMenuItemSpec, error) {
	return convertMenuItemDefs(mb, defs, nil, "", map[string]string{})
}
