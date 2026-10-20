// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// alterNavigationExample is what an error about the statement shows.
const alterNavigationExample = `alter navigation Responsive {
  insert after 'Home' { menu item 'Orders' ( OnClick: show page Shop.Orders ) }
  set ( Caption: 'User accounts' ) on 'Admin' > 'Users';
  drop 'Old import';
  set login page Shop.Login;
}`

// exitAlterNavigationStatement builds `alter navigation <Profile> { … }`.
func (b *Builder) exitAlterNavigationStatement(ctx *parser.AlterStatementContext) {
	name := ""
	if qn := ctx.QualifiedName(); qn != nil {
		name = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		name = id.GetText()
	}
	stmt := &ast.AlterNavigationStmt{ProfileName: name}
	for _, opCtx := range ctx.AllAlterNavigationOperation() {
		if op := b.buildAlterNavigationOperation(opCtx.(*parser.AlterNavigationOperationContext)); op != nil {
			stmt.Operations = append(stmt.Operations, op)
		}
	}
	b.statements = append(b.statements, stmt)
}

// exitAlterMenuStatement builds `alter menu Module.Menu { … }`.
func (b *Builder) exitAlterMenuStatement(ctx *parser.AlterStatementContext) {
	stmt := &ast.AlterMenuStmt{}
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}
	for _, opCtx := range ctx.AllAlterMenuOperation() {
		if op := b.buildAlterMenuOperation(opCtx.(*parser.AlterMenuOperationContext)); op != nil {
			stmt.Operations = append(stmt.Operations, op)
		}
	}
	b.statements = append(b.statements, stmt)
}

func (b *Builder) buildAlterNavigationOperation(ctx *parser.AlterNavigationOperationContext) ast.MenuAlterOp {
	if m := ctx.AlterMenuOperation(); m != nil {
		return b.buildAlterMenuOperation(m.(*parser.AlterMenuOperationContext))
	}
	names := ctx.AllQualifiedName()
	switch {
	case ctx.SET() != nil && ctx.HOME() != nil:
		op := &ast.SetNavPageClauseOp{Clause: "home"}
		op.Home.IsPage = ctx.PAGE() != nil
		if len(names) > 0 {
			op.Home.Target = buildQualifiedName(names[0])
		}
		if ctx.FOR() != nil && len(names) > 1 {
			role := buildQualifiedName(names[1])
			op.Home.ForRole = &role
		}
		return op
	case ctx.DROP() != nil && ctx.HOME() != nil:
		op := &ast.DropNavPageClauseOp{Clause: "home"}
		if ctx.FOR() != nil && len(names) > 0 {
			role := buildQualifiedName(names[0])
			op.Role = &role
		}
		return op
	case ctx.LOGIN() != nil:
		if ctx.DROP() != nil {
			return &ast.DropNavPageClauseOp{Clause: "login"}
		}
		op := &ast.SetNavPageClauseOp{Clause: "login"}
		if len(names) > 0 {
			op.Page = buildQualifiedName(names[0])
		}
		return op
	case ctx.FOUND() != nil:
		if ctx.DROP() != nil {
			return &ast.DropNavPageClauseOp{Clause: "notfound"}
		}
		op := &ast.SetNavPageClauseOp{Clause: "notfound"}
		if len(names) > 0 {
			op.Page = buildQualifiedName(names[0])
		}
		return op
	}
	return nil
}

func (b *Builder) buildAlterMenuOperation(ctx *parser.AlterMenuOperationContext) ast.MenuAlterOp {
	targets := ctx.AllAlterMenuTarget()
	position := func() string {
		switch {
		case ctx.BEFORE() != nil:
			return "before"
		case ctx.AFTER() != nil:
			return "after"
		case ctx.INTO() != nil:
			return "into"
		}
		return ""
	}
	items := func() []ast.NavMenuItemDef {
		var out []ast.NavMenuItemDef
		if ch, ok := ctx.NavMenuChildren().(*parser.NavMenuChildrenContext); ok && ch != nil {
			for _, it := range ch.AllNavMenuItemDef() {
				out = append(out, b.buildNavMenuItemDef(it))
			}
		}
		return out
	}
	switch {
	case ctx.INSERT() != nil:
		op := &ast.InsertMenuItemsOp{Items: items()}
		if len(targets) > 0 {
			t := buildMenuItemAddress(targets[0])
			op.Target, op.Position = &t, position()
		}
		return op
	case ctx.REPLACE() != nil:
		return &ast.ReplaceMenuItemOp{Target: buildMenuItemAddress(targets[0]), Items: items()}
	case ctx.MOVE() != nil:
		if len(targets) < 2 {
			return nil
		}
		return &ast.MoveMenuItemOp{Item: buildMenuItemAddress(targets[0]), Position: position(), Target: buildMenuItemAddress(targets[1])}
	case ctx.DROP() != nil:
		op := &ast.DropMenuItemsOp{}
		for _, t := range targets {
			op.Targets = append(op.Targets, buildMenuItemAddress(t))
		}
		return op
	case ctx.SET() != nil:
		return b.buildSetMenuItemOp(ctx, targets)
	}
	return nil
}

// buildSetMenuItemOp reads `set ( Key: value, … ) on <item>`. The list is new
// syntax, so it is strict: a key written twice is an error.
func (b *Builder) buildSetMenuItemOp(ctx *parser.AlterMenuOperationContext, targets []parser.IAlterMenuTargetContext) ast.MenuAlterOp {
	op := &ast.SetMenuItemOp{}
	if len(targets) > 0 {
		op.Target = buildMenuItemAddress(targets[0])
	}
	line := ctx.GetStart().GetLine()
	seen := map[string]bool{}
	for _, pc := range ctx.AllAlterMenuItemProp() {
		prop := pc.(*parser.AlterMenuItemPropContext)
		key := prop.GetStart().GetText()
		if seen[strings.ToLower(key)] {
			b.addErrorWithExample(fmt.Sprintf("line %d: set … on %s: %s is written twice", line, op.Target, key), alterNavigationExample)
			continue
		}
		seen[strings.ToLower(key)] = true
		switch {
		case prop.CAPTION() != nil:
			c := unquoteStringLit(prop.STRING_LITERAL())
			op.Caption = &c
		case prop.ONCLICK() != nil:
			op.SetAction = true
			if n := len(op.Target.Segments); n > 0 {
				op.Item.Caption = op.Target.Segments[n-1].Caption // names the item in the refusal
			}
			b.applyNavMenuAction(&op.Item, prop.NavMenuAction(), line)
		case prop.ICON() != nil:
			op.SetIcon = true
			if prop.NONE() == nil {
				applyNavMenuIcon(&op.Item, prop.NavMenuIconValue())
			}
		}
	}
	return op
}

// buildMenuItemAddress reads `'Admin'@2 > 'Users'`.
func buildMenuItemAddress(ctx parser.IAlterMenuTargetContext) types.MenuItemAddress {
	var a types.MenuItemAddress
	c, ok := ctx.(*parser.AlterMenuTargetContext)
	if !ok || c == nil {
		return a
	}
	for _, sc := range c.AllAlterMenuTargetSegment() {
		seg := sc.(*parser.AlterMenuTargetSegmentContext)
		s := types.MenuItemAddressSegment{Caption: unquoteStringLit(seg.STRING_LITERAL())}
		if n := seg.NUMBER_LITERAL(); n != nil {
			if v, err := strconv.Atoi(n.GetText()); err == nil {
				s.Ordinal = v
			}
		}
		a.Segments = append(a.Segments, s)
	}
	return a
}
