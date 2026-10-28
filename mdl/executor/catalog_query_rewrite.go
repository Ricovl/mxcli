// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// rewriteCatalogTableRefs turns each `CATALOG.<table>` table reference in a
// catalog SELECT into the bare <table> the SQLite catalog holds, and returns
// the tables it rewrote (lower case, first appearance first, no duplicates).
//
// It reads the query the way SQLite will, token by token, because the prefix
// is only a table reference where SQLite would read one. A regex over the raw
// text also matched `'Catalog.Product'` inside a string literal, so a filter on
// a module named Catalog compared against 'Product' and never matched
// (ako/mxcli#1450). Skipped verbatim:
//
//   - string literals in single quotes, where a doubled quote is an escaped
//     one, not the end;
//   - quoted identifiers in double quotes, backticks or brackets (a doubled
//     quote or backtick likewise escapes itself) — a name the user quoted is
//     spelled exactly, even when it reads Catalog.x;
//   - -- line comments and /* */ block comments.
//
// Of what is left, `catalog` followed by `.` and a name is the prefix, in any
// case and with whitespace or comments around the dot (the grammar reads
// CATALOG DOT name as three tokens, so `CATALOG . ENTITIES` parses; SQLite
// would read it as an unknown schema). `ProductCatalog.Item` is one identifier
// followed by a dot, so a name that merely contains the word is never touched.
// A `catalog` that itself follows a dot (`x.catalog.y`) is a column path, not
// the prefix.
//
// An unknown table name is still rewritten, so a new catalog view needs no
// allowlist entry: it yields SQLite's "no such table" rather than a silent
// passthrough.
func rewriteCatalogTableRefs(query string) (string, []string) {
	var out strings.Builder
	out.Grow(len(query))
	var tables []string
	seen := map[string]bool{}

	copied := 0      // query[:copied] has been written to out
	prevDot := false // the previous significant token was '.'
	for i := 0; i < len(query); {
		end, kind := sqlTokenEnd(query, i)
		switch kind {
		case sqlTokSpace, sqlTokComment:
			i = end
			continue
		case sqlTokIdent:
			if !prevDot && strings.EqualFold(query[i:end], "catalog") {
				dot := skipSQLTrivia(query, end)
				if dot < len(query) && query[dot] == '.' {
					name := skipSQLTrivia(query, dot+1)
					if name < len(query) {
						if nameEnd, nameKind := sqlTokenEnd(query, name); nameKind == sqlTokIdent || nameKind == sqlTokQuotedIdent {
							out.WriteString(query[copied:i])
							copied = name
							table := strings.ToLower(unquoteSQLIdent(query[name:nameEnd]))
							if !seen[table] {
								seen[table] = true
								tables = append(tables, table)
							}
							// Resume at the table name, which is an ordinary
							// identifier from here on.
							prevDot = false
							i = name
							continue
						}
					}
				}
			}
		}
		prevDot = kind == sqlTokOther && query[i:end] == "."
		i = end
	}
	out.WriteString(query[copied:])
	return out.String(), tables
}

type sqlTokKind int

const (
	sqlTokSpace sqlTokKind = iota
	sqlTokComment
	sqlTokString
	sqlTokQuotedIdent
	sqlTokIdent
	sqlTokOther
)

// sqlTokenEnd returns the end of the token starting at i and its kind,
// following SQLite's tokenizer for the cases that matter here. An unterminated
// literal, quoted identifier or block comment runs to the end of the input, as
// it does for SQLite (which then reports it).
func sqlTokenEnd(s string, i int) (int, sqlTokKind) {
	c := s[i]
	switch {
	case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
		j := i + 1
		for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r' || s[j] == '\f') {
			j++
		}
		return j, sqlTokSpace
	case c == '-' && i+1 < len(s) && s[i+1] == '-':
		if nl := strings.IndexByte(s[i:], '\n'); nl >= 0 {
			return i + nl + 1, sqlTokComment
		}
		return len(s), sqlTokComment
	case c == '/' && i+1 < len(s) && s[i+1] == '*':
		if close := strings.Index(s[i+2:], "*/"); close >= 0 {
			return i + 2 + close + 2, sqlTokComment
		}
		return len(s), sqlTokComment
	case c == '\'':
		return quotedEnd(s, i, '\''), sqlTokString
	case c == '"' || c == '`':
		return quotedEnd(s, i, c), sqlTokQuotedIdent
	case c == '[':
		if close := strings.IndexByte(s[i+1:], ']'); close >= 0 {
			return i + 1 + close + 1, sqlTokQuotedIdent
		}
		return len(s), sqlTokQuotedIdent
	case isSQLIdentByte(c) && !(c >= '0' && c <= '9'):
		j := i + 1
		for j < len(s) && (isSQLIdentByte(s[j]) || s[j] == '$') {
			j++
		}
		return j, sqlTokIdent
	case c >= '0' && c <= '9':
		// A number, including a decimal point and exponent, so `1.5` is never
		// read as an identifier followed by a dot.
		j := i + 1
		for j < len(s) && (isSQLIdentByte(s[j]) || s[j] == '.') {
			j++
		}
		return j, sqlTokOther
	case c == '$' || c == ':' || c == '@' || c == '#' || c == '?':
		// A bound parameter: `$catalog` is one token, not the prefix.
		j := i + 1
		for j < len(s) && (isSQLIdentByte(s[j]) || s[j] == '$') {
			j++
		}
		return j, sqlTokOther
	}
	return i + 1, sqlTokOther
}

// quotedEnd returns the end of a token opened by q at s[i], where a doubled q
// stands for one q inside it.
func quotedEnd(s string, i int, q byte) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] == q {
			if j+1 < len(s) && s[j+1] == q {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(s)
}

// isSQLIdentByte reports whether c can appear in an unquoted SQLite
// identifier. Bytes >= 0x80 are identifier characters in SQLite, which keeps a
// UTF-8 sequence inside the identifier it belongs to.
func isSQLIdentByte(c byte) bool {
	return c == '_' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// skipSQLTrivia returns the index of the first byte at or after i that is not
// whitespace or a comment.
func skipSQLTrivia(s string, i int) int {
	for i < len(s) {
		end, kind := sqlTokenEnd(s, i)
		if kind != sqlTokSpace && kind != sqlTokComment {
			return i
		}
		i = end
	}
	return i
}

// unquoteSQLIdent strips the quotes from a quoted identifier token and leaves
// a bare one alone.
func unquoteSQLIdent(tok string) string {
	if len(tok) < 2 {
		return tok
	}
	switch tok[0] {
	case '"', '`':
		q := string(tok[0])
		return strings.ReplaceAll(strings.TrimSuffix(tok[1:], q), q+q, q)
	case '[':
		return strings.TrimSuffix(tok[1:], "]")
	}
	return tok
}
