// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/suggest"
)

// Phase 3.6 of PROPOSAL_mdl_beta_syntax_freeze.md (ako/mxcli#755): a
// constant's and a demo user's header is a ( Key: value ) list keyed by the
// Studio Pro property names, as a user role's (#707) and a scheduled event's
// already are. The clause forms are the deprecated aliases MDL-DEPR136/137,
// each with the rewrite fmt --upgrade applies.
//
// The lists are new syntax, so an unknown key, a repeated key or a value of the
// wrong shape is an error under every language version: no older script can
// depend on it being ignored.

var constantKeys = []string{"Type", "DefaultValue", "ExposedToClient"}

var demoUserKeys = []string{"Password", "Entity", "UserRoles"}

// canonicalKey is key spelled as in known, or "" when it is none of them.
func canonicalKey(key string, known []string) string {
	for _, k := range known {
		if strings.EqualFold(k, key) {
			return k
		}
	}
	return ""
}

// unknownKeyError is the error for a key a property list does not take.
func unknownKeyError(line int, what, key string, known []string) error {
	detail := fmt.Sprintf("line %d: %s has no property '%s'", line, what, key)
	if near := suggest.Closest(key, known); near != "" {
		detail += fmt.Sprintf(" — did you mean '%s'?", near)
	}
	return fmt.Errorf("%s It takes %s", detail, strings.Join(known, ", "))
}

// constantProperties reads a constant's property list into stmt.
func (b *Builder) constantProperties(stmt *ast.CreateConstantStmt, pl *parser.ConstantPropertyListContext) {
	what := "constant " + stmt.Name.String()
	seen := map[string]bool{}
	for _, p := range pl.AllConstantProperty() {
		pc, ok := p.(*parser.ConstantPropertyContext)
		if !ok || pc == nil || pc.IdentifierOrKeyword() == nil {
			continue
		}
		line := pc.GetStart().GetLine()
		key := identifierOrKeywordText(pc.IdentifierOrKeyword())
		canonical := canonicalKey(key, constantKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, constantKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		lit, dt := pc.Literal(), pc.DataType()
		switch canonical {
		case "Type":
			if dt == nil {
				b.addError(fmt.Errorf("line %d: %s: Type takes a data type (String, Integer, Long, Decimal, Boolean, DateTime), not %s",
					line, what, nodeText(lit)))
				continue
			}
			stmt.DataType = buildDataType(dt)
		case "DefaultValue":
			if lit == nil {
				b.addError(fmt.Errorf("line %d: %s: DefaultValue takes a literal ('text', 42, true), not %s",
					line, what, nodeText(dt)))
				continue
			}
			stmt.DefaultValue = extractLiteralValue(lit)
		case "ExposedToClient":
			bl := (*parser.BooleanLiteralContext)(nil)
			if l, ok := lit.(*parser.LiteralContext); ok && l != nil {
				bl, _ = l.BooleanLiteral().(*parser.BooleanLiteralContext)
			}
			if bl == nil {
				b.addError(fmt.Errorf("line %d: %s: ExposedToClient takes true or false, not %s", line, what, nodeText(pc.GetChild(2))))
				continue
			}
			stmt.ExposedToClient = strings.EqualFold(bl.GetText(), "true")
		}
	}
	for _, k := range []string{"Type", "DefaultValue"} {
		if !seen[k] {
			b.addError(fmt.Errorf("line %d: %s needs %s in its property list", pl.GetStart().GetLine(), what, k))
		}
	}
}

// constantClausesFix rewrites `type T default v [exposed to client]` as
// `( Type: T, DefaultValue: v[, ExposedToClient: true] )`. It edits only the
// keywords around the type and the value, so both are kept exactly as written
// and a rewrite inside them (a string escape) never overlaps this one.
func constantClausesFix(ctx *parser.CreateConstantStatementContext) *ast.Fix {
	// The listener also runs over a statement the parser recovered from, so a
	// token the rule requires may be missing: no rewrite then.
	if ctx.TYPE() == nil || ctx.DEFAULT() == nil || ctx.DataType() == nil || ctx.Literal() == nil ||
		ctx.DataType().GetStop() == nil || ctx.Literal().GetStop() == nil {
		return nil
	}
	typ, def := ctx.TYPE().GetSymbol(), ctx.DEFAULT().GetSymbol()
	dt, lit := ctx.DataType(), ctx.Literal()
	edits := []ast.TextEdit{
		replaceSpan(typ, typ, "( Type:"),
		replaceGap(dt.GetStop().GetStop(), def.GetStop()+1, ", DefaultValue:"),
	}
	closing := " )"
	if opts, ok := ctx.ConstantOptions().(*parser.ConstantOptionsContext); ok && opts != nil {
		for _, o := range opts.AllConstantOption() {
			oc := o.(*parser.ConstantOptionContext)
			if oc.EXPOSED() == nil {
				continue
			}
			closing = ", ExposedToClient: " + keywordLike(typ.GetText(), "true") + " )"
			edits = append(edits, ast.TextEdit{Start: startAfterSpace(oc.GetStart()), Stop: oc.GetStop().GetStop() + 1})
		}
	}
	edits = append(edits, insertAt(lit.GetStop().GetStop()+1, closing))
	return &ast.Fix{Edits: edits}
}

// demoUserProperties reads a demo user's property list into stmt.
func (b *Builder) demoUserProperties(stmt *ast.CreateDemoUserStmt, pl *parser.DemoUserPropertyListContext) {
	what := "demo user '" + stmt.UserName + "'"
	seen := map[string]bool{}
	for _, p := range pl.AllDemoUserProperty() {
		pc, ok := p.(*parser.DemoUserPropertyContext)
		if !ok || pc == nil {
			continue
		}
		ioks := pc.AllIdentifierOrKeyword()
		if len(ioks) == 0 {
			continue
		}
		line := pc.GetStart().GetLine()
		key := identifierOrKeywordText(ioks[0])
		canonical := canonicalKey(key, demoUserKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, demoUserKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		value := nodeText(pc.GetChild(2))
		switch canonical {
		case "Password":
			if pc.STRING_LITERAL() == nil {
				b.addError(fmt.Errorf("line %d: %s: Password takes a string, not %s", line, what, value))
				continue
			}
			stmt.Password = unquoteStringLit(pc.STRING_LITERAL())
		case "Entity":
			if pc.QualifiedName() == nil {
				b.addError(fmt.Errorf("line %d: %s: Entity takes the user entity, Module.Entity, not %s", line, what, value))
				continue
			}
			stmt.Entity = buildQualifiedName(pc.QualifiedName()).String()
		case "UserRoles":
			if pc.LPAREN() == nil {
				b.addError(fmt.Errorf("line %d: %s: UserRoles takes a list of user roles: UserRoles: (Role, …)", line, what))
				continue
			}
			for _, iok := range ioks[1:] {
				stmt.UserRoles = append(stmt.UserRoles, identifierOrKeywordText(iok))
			}
		}
	}
	// UserRoles may be empty or left out: the clause form could not say so,
	// so a demo user without roles described into a statement that did not
	// parse. Password has no default to fall back on.
	if !seen["Password"] {
		b.addError(fmt.Errorf("line %d: %s needs Password in its property list", pl.GetStart().GetLine(), what))
	}
}

// demoUserClausesFix rewrites `password 'p' [entity M.E] (R1, R2)` as
// `( Password: 'p'[, Entity: M.E], UserRoles: (R1, R2) )`, editing only the
// keywords so the password, entity and roles are kept as written.
func demoUserClausesFix(ctx *parser.CreateDemoUserStatementContext) *ast.Fix {
	sls := ctx.AllSTRING_LITERAL()
	if ctx.PASSWORD() == nil || len(sls) < 2 || ctx.LPAREN() == nil || ctx.RPAREN() == nil ||
		(ctx.ENTITY() != nil && (ctx.QualifiedName() == nil || ctx.QualifiedName().GetStop() == nil)) {
		return nil // a statement the parser recovered from (see constantClausesFix)
	}
	pw := ctx.PASSWORD().GetSymbol()
	prev := sls[1].GetSymbol().GetStop()
	edits := []ast.TextEdit{replaceSpan(pw, pw, "( Password:")}
	if e := ctx.ENTITY(); e != nil {
		edits = append(edits, replaceGap(prev, e.GetSymbol().GetStop()+1, ", Entity:"))
		prev = ctx.QualifiedName().GetStop().GetStop()
	}
	lp, rp := ctx.LPAREN().GetSymbol(), ctx.RPAREN().GetSymbol()
	edits = append(edits,
		replaceGap(prev, lp.GetStop()+1, ", UserRoles: ("),
		insertAt(rp.GetStop()+1, " )"))
	return &ast.Fix{Edits: edits}
}

// recordDemoUserClauses records MDL-DEPR137 on the clause form.
func (b *Builder) recordDemoUserClauses(ctx *parser.CreateDemoUserStatementContext) {
	b.recordDeprecation(deprecation.DemoUserClauses, ctx.PASSWORD().GetSymbol(), "")
	b.fixLastDeprecation(deprecation.DemoUserClauses, demoUserClausesFix(ctx), "")
}
