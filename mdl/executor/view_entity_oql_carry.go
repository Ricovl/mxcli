// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// describedViewQuery renders a view entity's stored OQL as DESCRIBE prints it:
// ` as (`, each stored line indented by two, then `)`. It is the one rendering —
// the describer prints it, and carriedViewQuery re-parses it — so the carry
// cannot drift from what describe actually emits.
func describedViewQuery(oql string) string {
	var b strings.Builder
	b.WriteString(" as (\n")
	for line := range strings.SplitSeq(oql, "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(")")
	return b.String()
}

// viewQueryAsReexecuted is the query `exec` stores for a describe of oql: the
// description parsed back, exactly as a script holding it would be.
//
// It is not the stored text. The parser keeps the query from its first token to
// its last and strips the indentation the lines share (ako/mxcli#653), so a
// describe cannot carry what lies outside that span — leading or trailing
// whitespace, a blank first line, a comment before the first token or after the
// last — nor tell an indentation the stored query has on every line from the
// two spaces describe added. ok is false when the description does not parse.
func viewQueryAsReexecuted(oql string) (string, bool) {
	src := "create view entity M.V (\n  A: Integer\n)" + describedViewQuery(oql) + ";"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 || prog == nil {
		return "", false
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			return s.Query.RawQuery, true
		}
	}
	return "", false
}

// carriedViewQuery returns the query a `create or modify` / `create or replace`
// of an existing view entity should store: the STORED text when the statement's
// query is exactly what that stored text describes to, and the statement's own
// otherwise.
//
// Without it, describe → exec rewrote any query Studio Pro had stored with an
// indented first line, a comment outside the tokens or a trailing newline — the
// "re-indents the OQL" of mendixlabs/mxcli#1627 — and did it on a statement that
// changes nothing. When the two are equal the statement denotes the stored
// query, so keeping it loses nothing; a real edit differs from the re-executed
// description and is written as authored.
func carriedViewQuery(authored, stored string) string {
	if stored == "" || authored == stored {
		return authored
	}
	if reexecuted, ok := viewQueryAsReexecuted(stored); ok && reexecuted == authored {
		return stored
	}
	return authored
}
