// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// The rest of R2 (ako/mxcli#754), after the integration documents: the
// navigation and menu documents, the property maps, and the database
// connection.
//
//   - a REST header list binds `'Name' = value` where a map has `'Name': value`
//     (MDL-DEPR120);
//   - menu items are in ( ) with `;` after each, where children are in { }
//     (MDL-DEPR121), and an item's action and icon are clauses after its
//     caption rather than `( OnClick: …, Icon: … )` (MDL-DEPR122);
//   - the page and snippet header's `Params:` / `Variables:` maps are in
//     braces (MDL-DEPR123), a text template's parameters in brackets
//     (MDL-DEPR124), `DesignProperties:` in brackets (MDL-DEPR125), and a
//     snippet call's arguments are a brace map `{$P: $v}` (MDL-DEPR126);
//   - the database connection is clauses and a begin … end block of queries
//     (MDL-DEPR127).
//
// r2RestUse is called for every node of the statement by EnterStatement's
// walk, which records each code once per statement.
func r2RestUse(n antlr.Tree, use func(code string, at antlr.Token) *r2Use) {
	switch x := n.(type) {
	case *parser.RestClientHeaderItemContext:
		if eq := x.EQUALS(); eq != nil && len(x.AllSTRING_LITERAL()) > 0 {
			name := x.STRING_LITERAL(0).GetSymbol()
			u := use(deprecation.RestHeaderEquals, eq.GetSymbol())
			u.edits = append(u.edits, replaceGap(name.GetStop(), eq.GetSymbol().GetStop()+1, ":"))
		}
	case *parser.NavigationClauseContext:
		if x.MENU_KW() != nil && x.LPAREN() != nil && x.RPAREN() != nil {
			u := use(deprecation.MenuChildrenParens, x.LPAREN().GetSymbol())
			u.edits = append(u.edits,
				replaceSpan(x.MENU_KW().GetSymbol(), x.LPAREN().GetSymbol(), "{"),
				replaceSpan(x.RPAREN().GetSymbol(), x.RPAREN().GetSymbol(), "}"))
		}
	case *parser.CreateMenuStatementContext:
		if x.LPAREN() != nil && x.RPAREN() != nil {
			use(deprecation.MenuChildrenParens, x.LPAREN().GetSymbol()).swap(x.LPAREN(), x.RPAREN(), "{", "}")
		}
	case *parser.NavMenuItemDefContext:
		if x.LPAREN() != nil && x.RPAREN() != nil {
			use(deprecation.MenuChildrenParens, x.LPAREN().GetSymbol()).swap(x.LPAREN(), x.RPAREN(), "{", "}")
		}
		if semi := x.SEMICOLON(); semi != nil {
			u := use(deprecation.MenuChildrenParens, semi.GetSymbol())
			u.edits = append(u.edits, replaceSpan(semi.GetSymbol(), semi.GetSymbol(), ""))
		}
		if first := menuItemClauseStart(x); first != nil {
			use(deprecation.MenuItemClauses, first).menuItemClauses(x)
		}
	case *parser.PageHeaderPropertyV3Context:
		if x.LBRACE() != nil && x.RBRACE() != nil {
			use(deprecation.HeaderMapBraces, x.LBRACE().GetSymbol()).swap(x.LBRACE(), x.RBRACE(), "(", ")")
		}
	case *parser.SnippetHeaderPropertyV3Context:
		if x.LBRACE() != nil && x.RBRACE() != nil {
			use(deprecation.HeaderMapBraces, x.LBRACE().GetSymbol()).swap(x.LBRACE(), x.RBRACE(), "(", ")")
		}
	case *parser.ParamListV3Context:
		if x.LBRACKET() != nil && x.RBRACKET() != nil {
			use(deprecation.TemplateParamsBrackets, x.LBRACKET().GetSymbol()).swap(x.LBRACKET(), x.RBRACKET(), "(", ")")
		}
	case *parser.DesignPropertyListV3Context:
		if x.LBRACKET() != nil && x.RBRACKET() != nil {
			use(deprecation.DesignPropertiesBrackets, x.LBRACKET().GetSymbol()).swap(x.LBRACKET(), x.RBRACKET(), "(", ")")
		}
	case *parser.SnippetCallParamListV3Context:
		if x.LBRACE() != nil && x.RBRACE() != nil {
			use(deprecation.SnippetCallParamsBraces, x.LBRACE().GetSymbol()).snippetCallArgs(x)
		}
	case *parser.CreateDatabaseConnectionStatementContext:
		if opts := x.AllDatabaseConnectionOption(); len(opts) > 0 {
			use(deprecation.DatabaseConnectionClauses, opts[0].GetStart()).databaseConnection(x)
		}
	}
}

// menuItemClauseStart is the first token of a menu item's action or icon
// clause — `page`, `microflow`, `sign out` or `icon` — or nil when it has none.
func menuItemClauseStart(x *parser.NavMenuItemDefContext) antlr.Token {
	for _, t := range []antlr.TerminalNode{x.PAGE(), x.MICROFLOW(), x.SIGN_OUT()} {
		if t != nil {
			return t.GetSymbol()
		}
	}
	if ic := x.NavMenuIcon(); ic != nil {
		return ic.GetStart()
	}
	return nil
}

// menuItemClauses rewrites `menu item 'X' page M.P icon I` as
// `menu item 'X' ( OnClick: show page M.P, Icon: I )`, and a sub-menu's
// `menu 'X' icon I (` as `menu 'X' ( Icon: I ) (` (the parentheses around
// its items are MDL-DEPR121's). Only the clause words are replaced, so the
// target and the icon stay as written.
func (u *r2Use) menuItemClauses(x *parser.NavMenuItemDefContext) {
	like := x.MENU_KW().GetText()
	var last antlr.Token
	switch {
	case x.PAGE() != nil && x.QualifiedName() != nil:
		t := x.PAGE().GetSymbol()
		u.edits = append(u.edits, replaceSpan(t, t, "( OnClick: "+keywordLike(like, "show page")))
		last = x.QualifiedName().GetStop()
	case x.MICROFLOW() != nil && x.QualifiedName() != nil:
		t := x.MICROFLOW().GetSymbol()
		u.edits = append(u.edits, replaceSpan(t, t, "( OnClick: "+keywordLike(like, "call microflow")))
		last = x.QualifiedName().GetStop()
	case x.SIGN_OUT() != nil:
		// Inserted before the word rather than replacing it: `sign_out` has
		// MDL-DEPR020's own rewrite of that token.
		t := x.SIGN_OUT().GetSymbol()
		u.edits = append(u.edits, insertAt(t.GetStart(), "( OnClick: "))
		last = t
	}
	if ic, ok := x.NavMenuIcon().(*parser.NavMenuIconContext); ok && ic != nil {
		t := ic.ICON().GetSymbol()
		if last != nil {
			u.edits = append(u.edits, insertAt(last.GetStop()+1, ","), replaceSpan(t, t, "Icon:"))
		} else {
			u.edits = append(u.edits, replaceSpan(t, t, "( Icon:"))
		}
		last = ic.GetStop()
	}
	if last != nil {
		u.edits = append(u.edits, insertAt(last.GetStop()+1, " )"))
	}
}

// snippetCallArgs rewrites a snippet call's `{$Asset: $var, Other: $o}` as
// `(Asset = $var, Other = $o)`: a call binds its arguments `Param = value`,
// without a `$` on the parameter's name (R4).
func (u *r2Use) snippetCallArgs(x *parser.SnippetCallParamListV3Context) {
	u.swap(x.LBRACE(), x.RBRACE(), "(", ")")
	for _, mc := range x.AllSnippetCallParamMappingV3() {
		m, ok := mc.(*parser.SnippetCallParamMappingV3Context)
		if !ok || m.COLON() == nil {
			continue
		}
		var name antlr.Token
		if iok := m.IdentifierOrKeyword(); iok != nil {
			name = iok.GetStop()
		} else if vars := m.AllVARIABLE(); len(vars) >= 2 {
			name = vars[0].GetSymbol()
			u.edits = append(u.edits, replaceSpan(name, name, strings.TrimPrefix(name.GetText(), "$")))
		}
		if name == nil {
			continue
		}
		u.edits = append(u.edits, replaceGap(name.GetStop(), m.COLON().GetSymbol().GetStop()+1, " ="))
	}
}

// databaseConnection rewrites the clause form of a database connection,
//
//	type 'PostgreSQL' connection string @M.Url username @M.U password @M.P
//	begin
//	  query Q sql $$…$$ parameter p: Integer default '1' returns M.E map (c as A);
//	end
//
// as its property list and query children:
//
//	( Type: 'PostgreSQL', ConnectionString: @M.Url, Username: @M.U, Password: @M.P )
//	{
//	  query Q ( Sql: $$…$$, Parameters: ( p: Integer default '1' ), Returns: M.E, Map: ( A = c ) )
//	}
//
// Only the clause words are replaced and punctuation inserted: every value —
// the strings, the SQL, the constants — stays where it is, as written.
func (u *r2Use) databaseConnection(x *parser.CreateDatabaseConnectionStatementContext) {
	opts := x.AllDatabaseConnectionOption()
	for i, oc := range opts {
		o := oc.(*parser.DatabaseConnectionOptionContext)
		key, keyEnd := databaseConnectionKey(o)
		if key == "" {
			continue
		}
		if i == 0 {
			// The list opens after the name (or folder), before the clauses'
			// line break: `M.Db (` rather than `M.Db\n  ( Type:`.
			if prev := childTokenBefore(x, o); prev != nil {
				u.edits = append(u.edits, insertAt(prev.GetStop()+1, " ("))
			}
		}
		u.edits = append(u.edits, replaceSpan(o.GetStart(), keyEnd, key+":"))
		closing := ","
		if i == len(opts)-1 {
			closing = " )"
		}
		u.edits = append(u.edits, insertAt(o.GetStop().GetStop()+1, closing))
	}
	if x.BEGIN() != nil && x.END() != nil {
		u.swap(x.BEGIN(), x.END(), "{", "}")
	}
	for _, qc := range x.AllDatabaseQuery() {
		u.databaseQuery(qc.(*parser.DatabaseQueryContext))
	}
}

// databaseConnectionKey is the property key of an old connection clause, and
// the last token of the clause's words (`connection string` is two).
func databaseConnectionKey(o *parser.DatabaseConnectionOptionContext) (string, antlr.Token) {
	switch {
	case o.TYPE() != nil:
		return "Type", o.TYPE().GetSymbol()
	case o.CONNECTION() != nil && o.STRING_TYPE() != nil:
		return "ConnectionString", o.STRING_TYPE().GetSymbol()
	case o.HOST() != nil:
		return "Host", o.HOST().GetSymbol()
	case o.PORT() != nil:
		return "Port", o.PORT().GetSymbol()
	case o.DATABASE() != nil:
		return "DatabaseName", o.DATABASE().GetSymbol()
	case o.USERNAME() != nil:
		return "Username", o.USERNAME().GetSymbol()
	case o.PASSWORD() != nil:
		return "Password", o.PASSWORD().GetSymbol()
	}
	return "", nil
}

// databaseQuery rewrites one `query Q sql … parameter … returns M.E map (…);`
// as `query Q ( Sql: …, Parameters: ( … ), Returns: M.E, Map: ( … ) )`.
func (u *r2Use) databaseQuery(q *parser.DatabaseQueryContext) {
	children := q.GetChildren()
	// last is the last token before child i; next the first token of child i.
	last := func(i int) antlr.Token {
		for j := i - 1; j >= 0; j-- {
			switch n := children[j].(type) {
			case antlr.TerminalNode:
				return n.GetSymbol()
			case antlr.ParserRuleContext:
				return n.GetStop()
			}
		}
		return nil
	}
	inParams := false
	for i, c := range children {
		tn, ok := c.(antlr.TerminalNode)
		if !ok {
			continue
		}
		tok := tn.GetSymbol()
		switch tok.GetTokenType() {
		case parser.MDLParserSQL:
			u.edits = append(u.edits, insertAt(last(i).GetStop()+1, " ("), replaceSpan(tok, tok, "Sql:"))
		case parser.MDLParserPARAMETER:
			prev := last(i)
			if !inParams {
				u.edits = append(u.edits, insertAt(prev.GetStop()+1, ","), replaceSpan(tok, tok, "Parameters: ("))
				inParams = true
				continue
			}
			// A later parameter: a comma after the one before, and the word
			// dropped with the space after it.
			stop := tok.GetStop() + 1
			if i+1 < len(children) {
				if pr, ok := children[i+1].(antlr.ParserRuleContext); ok && pr.GetStart() != nil {
					stop = pr.GetStart().GetStart()
				}
			}
			u.edits = append(u.edits, insertAt(prev.GetStop()+1, ","),
				ast.TextEdit{Start: tok.GetStart(), Stop: stop})
		case parser.MDLParserRETURNS, parser.MDLParserSEMICOLON:
			prev := last(i)
			if inParams {
				u.edits = append(u.edits, insertAt(prev.GetStop()+1, " )"))
				inParams = false
			}
			if tok.GetTokenType() == parser.MDLParserRETURNS {
				u.edits = append(u.edits, insertAt(prev.GetStop()+1, ","), replaceSpan(tok, tok, "Returns:"))
			} else {
				u.edits = append(u.edits, replaceSpan(tok, tok, padWord(tok, ")")))
			}
		case parser.MDLParserMAP:
			u.edits = append(u.edits, insertAt(last(i).GetStop()+1, ","), replaceSpan(tok, tok, "Map:"))
		}
	}
	for _, mc := range q.AllDatabaseQueryMapping() {
		mm := mc.(*parser.DatabaseQueryMappingContext)
		ioks := mm.AllIdentifierOrKeyword()
		if len(ioks) < 2 {
			continue
		}
		// `column as Attr` binds Attr = column, the way a mapping side does.
		u.edits = append(u.edits, replaceSpan(mm.GetStart(), mm.GetStop(),
			nodeText(ioks[1])+" = "+nodeText(ioks[0])))
	}
}

// childTokenBefore is the last token of the child of parent that precedes child.
func childTokenBefore(parent antlr.ParserRuleContext, child antlr.Tree) antlr.Token {
	var prev antlr.Token
	for _, c := range parent.GetChildren() {
		if c == child {
			return prev
		}
		switch n := c.(type) {
		case antlr.TerminalNode:
			prev = n.GetSymbol()
		case antlr.ParserRuleContext:
			prev = n.GetStop()
		}
	}
	return nil
}
