// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateDatabaseConnectionStatement handles CREATE DATABASE CONNECTION statements.
func (b *Builder) ExitCreateDatabaseConnectionStatement(ctx *parser.CreateDatabaseConnectionStatementContext) {
	stmt := &ast.CreateDatabaseConnectionStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	// Canonical form: ( Key: value, … ) { query Q ( … ) } (R2)
	b.applyDatabaseConnectionProps(stmt, ctx)

	// Old form: clauses and a begin … end block (MDL-DEPR127)
	for _, optCtx := range ctx.AllDatabaseConnectionOption() {
		opt := optCtx.(*parser.DatabaseConnectionOptionContext)

		if opt.TYPE() != nil && opt.STRING_LITERAL() != nil {
			stmt.DatabaseType = unquoteStringLit(opt.STRING_LITERAL())
		}
		if opt.CONNECTION() != nil && opt.STRING_TYPE() != nil {
			// CONNECTION STRING <value>
			if opt.AT() != nil && opt.QualifiedName() != nil {
				// @Module.Constant reference
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.ConnectionString = qn.String()
				stmt.ConnectionStringIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.ConnectionString = unquoteStringLit(opt.STRING_LITERAL())
			}
		}
		if opt.HOST() != nil && opt.STRING_LITERAL() != nil {
			stmt.Host = unquoteStringLit(opt.STRING_LITERAL())
		}
		if opt.PORT() != nil && opt.NUMBER_LITERAL() != nil {
			stmt.Port, _ = strconv.Atoi(opt.NUMBER_LITERAL().GetText())
		}
		if opt.DATABASE() != nil && opt.STRING_LITERAL() != nil {
			stmt.Database = unquoteStringLit(opt.STRING_LITERAL())
		}
		if opt.USERNAME() != nil {
			if opt.AT() != nil && opt.QualifiedName() != nil {
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.UserName = qn.String()
				stmt.UserNameIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.UserName = unquoteStringLit(opt.STRING_LITERAL())
			}
		}
		if opt.PASSWORD() != nil {
			if opt.AT() != nil && opt.QualifiedName() != nil {
				qn := buildQualifiedName(opt.QualifiedName())
				stmt.Password = qn.String()
				stmt.PasswordIsRef = true
			} else if opt.STRING_LITERAL() != nil {
				stmt.Password = unquoteStringLit(opt.STRING_LITERAL())
			}
		}
	}

	// Parse queries
	for _, qCtx := range ctx.AllDatabaseQuery() {
		qc := qCtx.(*parser.DatabaseQueryContext)
		q := ast.DatabaseQueryDef{}

		// Query name (first identifierOrKeyword in the rule)
		if iok := qc.IdentifierOrKeyword(0); iok != nil {
			q.Name = identifierOrKeywordText(iok)
		}

		// SQL string. The DOLLAR_STRING form is checked FIRST: when the SQL is
		// written as $$…$$, STRING_LITERAL(0) is not the query at all — it is the
		// first parameter's DEFAULT '…'. Taking it produced a connection whose
		// query body was silently replaced by that default, with mx check
		// reporting 0 errors either way. The parameter-default indexing below
		// already compensates for the dollar-string case; this did not.
		if ds := qc.DOLLAR_STRING(); ds != nil {
			q.SQL = unquoteDollarString(ds.GetText())
		} else if sl := qc.STRING_LITERAL(0); sl != nil {
			q.SQL = unquoteStringLit(sl)
		}

		// RETURNS entity
		if qn := qc.QualifiedName(); qn != nil {
			q.Returns = buildQualifiedName(qn)
		}

		// PARAMETER clauses
		// Each PARAMETER has: identifierOrKeyword COLON dataType (DEFAULT STRING_LITERAL | NULL)?
		// identifierOrKeyword(0) is the query name, so params start at index 1
		paramTokens := qc.AllPARAMETER()
		defaultIdx := 0 // tracks DEFAULT occurrence index
		nullIdx := 0    // tracks NULL occurrence index
		for pi := range paramTokens {
			paramDef := ast.DatabaseQueryParamDef{}
			// Parameter name is identifierOrKeyword at index pi+1 (0 is query name)
			if iok := qc.IdentifierOrKeyword(pi + 1); iok != nil {
				paramDef.Name = identifierOrKeywordText(iok)
			}
			// DataType at index pi
			if dt := qc.DataType(pi); dt != nil {
				paramDef.DataType = buildDataType(dt)
			}
			// Check for DEFAULT or NULL — scan children to see what follows this PARAMETER's dataType
			// Use positional scanning: find the token that appears after the dataType for this param
			hasDefault := false
			hasNull := false
			paramParseChildren(qc, pi, &hasDefault, &hasNull)
			if hasDefault {
				// STRING_LITERAL index: 0 is SQL (if not dollar), then one per DEFAULT
				slIdx := defaultIdx + 1 // +1 because STRING_LITERAL(0) is the SQL string
				if qc.DOLLAR_STRING() != nil {
					slIdx = defaultIdx // SQL used DOLLAR_STRING
				}
				if sl := qc.STRING_LITERAL(slIdx); sl != nil {
					paramDef.DefaultValue = unquoteStringLit(sl)
				}
				defaultIdx++
			} else if hasNull {
				paramDef.TestWithNull = true
				nullIdx++
			}
			q.Parameters = append(q.Parameters, paramDef)
		}

		// MAP clause
		for _, mapCtx := range qc.AllDatabaseQueryMapping() {
			mc := mapCtx.(*parser.DatabaseQueryMappingContext)
			ioks := mc.AllIdentifierOrKeyword()
			if len(ioks) >= 2 {
				q.Mappings = append(q.Mappings, ast.DatabaseQueryMappingDef{
					ColumnName:    identifierOrKeywordText(ioks[0]),
					AttributeName: identifierOrKeywordText(ioks[1]),
				})
			}
		}

		stmt.Queries = append(stmt.Queries, q)
	}

	// Check for CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}

// paramParseChildren scans the parse tree children of a DatabaseQueryContext to determine
// whether the parameter at index paramIdx has a DEFAULT or NULL modifier.
func paramParseChildren(qc *parser.DatabaseQueryContext, paramIdx int, hasDefault, hasNull *bool) {
	paramCount := 0
	children := qc.GetChildren()
	for i, child := range children {
		tn, ok := child.(antlr.TerminalNode)
		if !ok {
			continue
		}
		if tn.GetSymbol().GetTokenType() == parser.MDLParserPARAMETER {
			if paramCount == paramIdx {
				// Found our PARAMETER, now look ahead for DEFAULT or NULL before next PARAMETER/RETURNS/SEMICOLON
				for j := i + 1; j < len(children); j++ {
					tn2, ok := children[j].(antlr.TerminalNode)
					if !ok {
						continue
					}
					tt := tn2.GetSymbol().GetTokenType()
					if tt == parser.MDLParserPARAMETER || tt == parser.MDLParserRETURNS || tt == parser.MDLParserSEMICOLON {
						return
					}
					if tt == parser.MDLParserDEFAULT {
						*hasDefault = true
						return
					}
					if tt == parser.MDLParserNULL {
						*hasNull = true
						return
					}
				}
				return
			}
			paramCount++
		}
	}
}

// unquoteDollarString removes $$ delimiters from dollar-quoted strings.
func unquoteDollarString(s string) string {
	if strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") {
		return s[2 : len(s)-2]
	}
	return s
}

// applyDatabaseConnectionProps reads the canonical database connection: its
// properties in ( ) and its queries as { query Q ( … ) } children (R2,
// ako/mxcli#754). The keys are those of the old clauses; they are new syntax,
// so an unknown key or a value of the wrong kind is an error rather than
// something to warn about and drop.
func (b *Builder) applyDatabaseConnectionProps(stmt *ast.CreateDatabaseConnectionStmt, ctx *parser.CreateDatabaseConnectionStatementContext) {
	for _, pc := range ctx.AllDatabaseConnectionProp() {
		p := pc.(*parser.DatabaseConnectionPropContext)
		key := identifierOrKeywordText(p.IdentifierOrKeyword())
		line := p.GetStart().GetLine()
		str, ref, num := p.STRING_LITERAL(), p.QualifiedName(), p.NUMBER_LITERAL()
		refOrString := func(val *string, isRef *bool) {
			switch {
			case ref != nil:
				*val, *isRef = buildQualifiedName(ref).String(), true
			case str != nil:
				*val = unquoteStringLit(str)
			default:
				b.addError(fmt.Errorf("line %d: database connection %s: %s takes a string or a constant (@Module.Constant)", line, stmt.Name, key))
			}
		}
		onlyString := func(val *string) {
			if str == nil {
				b.addError(fmt.Errorf("line %d: database connection %s: %s takes a string", line, stmt.Name, key))
				return
			}
			*val = unquoteStringLit(str)
		}
		switch strings.ToLower(key) {
		case "type":
			onlyString(&stmt.DatabaseType)
		case "connectionstring":
			refOrString(&stmt.ConnectionString, &stmt.ConnectionStringIsRef)
		case "username":
			refOrString(&stmt.UserName, &stmt.UserNameIsRef)
		case "password":
			refOrString(&stmt.Password, &stmt.PasswordIsRef)
		case "host":
			onlyString(&stmt.Host)
		case "databasename":
			onlyString(&stmt.Database)
		case "port":
			if num == nil {
				b.addError(fmt.Errorf("line %d: database connection %s: Port takes a number", line, stmt.Name))
				continue
			}
			stmt.Port, _ = strconv.Atoi(num.GetText())
		default:
			b.addError(fmt.Errorf("line %d: unknown property '%s' on database connection %s: the properties are "+
				"Type, ConnectionString, Host, Port, DatabaseName, Username and Password", line, key, stmt.Name))
		}
	}

	for _, qc := range ctx.AllDatabaseQueryDef() {
		stmt.Queries = append(stmt.Queries, b.buildDatabaseQueryDef(stmt.Name, qc.(*parser.DatabaseQueryDefContext)))
	}
}

// buildDatabaseQueryDef reads `query Q ( Sql: …, Parameters: ( … ), Returns:
// M.E, Map: ( Attr = column ) )`.
func (b *Builder) buildDatabaseQueryDef(conn ast.QualifiedName, qc *parser.DatabaseQueryDefContext) ast.DatabaseQueryDef {
	q := ast.DatabaseQueryDef{Name: identifierOrKeywordText(qc.IdentifierOrKeyword())}
	for _, pc := range qc.AllDatabaseQueryProp() {
		p := pc.(*parser.DatabaseQueryPropContext)
		key := identifierOrKeywordText(p.IdentifierOrKeyword())
		line := p.GetStart().GetLine()
		wrong := func(want string) {
			b.addError(fmt.Errorf("line %d: query %s on database connection %s: %s takes %s", line, q.Name, conn, key, want))
		}
		switch strings.ToLower(key) {
		case "sql":
			switch {
			case p.DOLLAR_STRING() != nil:
				q.SQL = unquoteDollarString(p.DOLLAR_STRING().GetText())
			case p.STRING_LITERAL() != nil:
				q.SQL = unquoteStringLit(p.STRING_LITERAL())
			default:
				wrong("the SQL, as $$…$$ or a string")
			}
		case "returns":
			if p.QualifiedName() == nil {
				wrong("an entity")
				continue
			}
			q.Returns = buildQualifiedName(p.QualifiedName())
		case "parameters":
			if len(p.AllDatabaseQueryParam()) == 0 {
				wrong("a parameter list, ( name: Type [default '…' | null], … )")
				continue
			}
			for _, dc := range p.AllDatabaseQueryParam() {
				d := dc.(*parser.DatabaseQueryParamContext)
				param := ast.DatabaseQueryParamDef{Name: identifierOrKeywordText(d.IdentifierOrKeyword())}
				if dt := d.DataType(); dt != nil {
					param.DataType = buildDataType(dt)
				}
				switch {
				case d.DEFAULT() != nil && d.STRING_LITERAL() != nil:
					param.DefaultValue = unquoteStringLit(d.STRING_LITERAL())
				case d.NULL() != nil:
					param.TestWithNull = true
				}
				q.Parameters = append(q.Parameters, param)
			}
		case "map":
			if len(p.AllDatabaseQueryColumn()) == 0 {
				wrong("a column map, ( Attribute = column, … )")
				continue
			}
			for _, mc := range p.AllDatabaseQueryColumn() {
				m := mc.(*parser.DatabaseQueryColumnContext)
				ioks := m.AllIdentifierOrKeyword()
				if len(ioks) < 2 {
					continue
				}
				q.Mappings = append(q.Mappings, ast.DatabaseQueryMappingDef{
					ColumnName:    identifierOrKeywordText(ioks[1]),
					AttributeName: identifierOrKeywordText(ioks[0]),
				})
			}
		default:
			b.addError(fmt.Errorf("line %d: unknown property '%s' on query %s of database connection %s: "+
				"a query takes Sql, Parameters, Returns and Map", line, key, q.Name, conn))
		}
	}
	return q
}
