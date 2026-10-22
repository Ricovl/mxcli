// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1608 (ako/mxcli#1536): `describe consumed rest service`
// printed `Response: json` for an operation created with
// `Response: json as $Result`, and the parser rejected it:
//
//	line 14:2 missing VARIABLE at ')'
//
// The variable is not stored — a consumed REST operation has no response
// variable; it is named by the call activity — so there is nothing for
// describe to print, and the bare form has to be the language's.

func TestIssue1608_BareResponseKindParses(t *testing.T) {
	for _, kind := range []string{"json", "string", "file", "status"} {
		src := "create consumed rest service M.S (BaseUrl: 'https://x', Authentication: none) {\n" +
			"  operation Op (Method: get, Path: '/x', Response: " + kind + ")\n};"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Errorf("Response: %s does not parse: %v", kind, errs)
			continue
		}
		stmt := prog.Statements[0].(*ast.CreateRestClientStmt)
		if got := stmt.Operations[0].ResponseType; got != kind {
			t.Errorf("Response: %s read as ResponseType %q", kind, got)
		}
	}
}

// The response variable is not stored, so describe prints each kind bare and
// the output runs again as the same response.
func TestIssue1608_DescribeResponseKindsReparse(t *testing.T) {
	for _, kind := range []string{"JSON", "STRING", "FILE", "STATUS", "NONE"} {
		op := &model.RestClientOperation{Name: "Op", HttpMethod: "GET", Path: "/x", ResponseType: kind}
		var buf bytes.Buffer
		outputRestOperation(&buf, op)
		src := "create consumed rest service M.S (BaseUrl: 'https://x', Authentication: none) {\n" + buf.String() + "};"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Errorf("%s: describe output does not re-parse: %v\n%s", kind, errs, buf.String())
			continue
		}
		reop, err := buildRestClientOperation(prog.Statements[0].(*ast.CreateRestClientStmt).Operations[0])
		if err != nil {
			t.Fatal(err)
		}
		if reop.ResponseType != kind {
			t.Errorf("%s re-parsed as %s:\n%s", kind, reop.ResponseType, buf.String())
		}
	}
}
