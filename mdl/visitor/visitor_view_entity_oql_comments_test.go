// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1632: the stored query was the text from its first token to its
// last, and the lexer skips comments, so a comment before the first token or
// after the last was dropped — from a script, and from every describe → exec
// into another project. A comment between clauses always survived; the cases
// without an outer comment are the control and must store exactly what they did.
func TestViewEntityOQL_KeepsCommentsOutsideTheTokens(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "line comment before the first token",
			src:  "create view entity M.V (Code: String(20)) as (\n  -- view over targets\n  select t.Code as Code\n  from M.T as t\n);",
			want: "-- view over targets\nselect t.Code as Code\nfrom M.T as t",
		},
		{
			name: "line comment after the last token",
			src:  "create view entity M.V (Code: String(20)) as (\n  select t.Code as Code\n  from M.T as t -- end\n);",
			want: "select t.Code as Code\nfrom M.T as t -- end",
		},
		{
			name: "comment on its own line after the query",
			src:  "create view entity M.V (Code: String(20)) as (\n  select t.Code as Code\n  from M.T as t\n  -- end of view\n);",
			want: "select t.Code as Code\nfrom M.T as t\n-- end of view",
		},
		{
			name: "block comments at both ends",
			src:  "create view entity M.V (Code: String(20)) as (\n  /* header */\n  select t.Code as Code\n  from M.T as t\n  /* footer */\n);",
			want: "/* header */\nselect t.Code as Code\nfrom M.T as t\n/* footer */",
		},
		{
			name: "two leading comments",
			src:  "create view entity M.V (Code: String(20)) as (\n  -- first\n  -- second\n  select t.Code as Code\n  from M.T as t\n);",
			want: "-- first\n-- second\nselect t.Code as Code\nfrom M.T as t",
		},
		{
			name: "control: no outer comment",
			src:  "create view entity M.V (Code: String(20)) as (\n  select t.Code as Code\n  from M.T as t\n);",
			want: "select t.Code as Code\nfrom M.T as t",
		},
		{
			name: "control: query on the as ( line",
			src:  "create view entity M.V (Code: String(20)) as (select t.Code as Code\n    from M.T as t);",
			want: "select t.Code as Code\nfrom M.T as t",
		},
		{
			name: "control: no parentheses",
			src:  "create view entity M.V (Code: String(20)) as\n  select t.Code as Code\n  from M.T as t;",
			want: "select t.Code as Code\nfrom M.T as t",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse failed: %v", errs)
			}
			stmt, ok := prog.Statements[0].(*ast.CreateViewEntityStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.CreateViewEntityStmt", prog.Statements[0])
			}
			if stmt.Query.RawQuery != tc.want {
				t.Errorf("stored OQL\nwant %q\n got %q", tc.want, stmt.Query.RawQuery)
			}
		})
	}
}
