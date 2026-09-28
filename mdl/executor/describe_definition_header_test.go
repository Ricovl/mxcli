// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// R6 (ako/mxcli#755, PROPOSAL_mdl_beta_syntax_freeze.md §3): `describe` of a
// definition or lookup answers with a report, not MDL, and says so on its first
// line; the JSON form marks it not executable. A model element's describe is
// runnable MDL and carries no such header.
func TestDescribeDefinitionReportIsMarkedNotExecutable(t *testing.T) {
	cases := []struct {
		stmt *ast.DescribeStmt
		kind string
	}{
		{&ast.DescribeStmt{ObjectType: ast.DescribeWidget, Name: ast.QualifiedName{Name: "combobox"}}, "widget type"},
		{&ast.DescribeStmt{ObjectType: ast.DescribeGlyph, Qualifier: "star"}, "glyph"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			ctx, out := newGlyphTestContext()
			if err := execDescribe(ctx, c.stmt); err != nil {
				t.Fatalf("describe: %v", err)
			}
			want := "-- " + c.kind + " definition (not executable)\n"
			if !strings.HasPrefix(out.String(), want) {
				t.Errorf("output does not start with %q:\n%s", want, out.String())
			}

			ctx, out = newGlyphTestContext()
			ctx.Format = FormatJSON
			if err := execDescribe(ctx, c.stmt); err != nil {
				t.Fatalf("describe --json: %v", err)
			}
			var doc map[string]any
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, out.String())
			}
			if doc["executable"] != false {
				t.Errorf("executable = %v, want false: %s", doc["executable"], out.String())
			}
		})
	}
}

// Control: the header is only on definitions.
func TestDefinitionReportKind(t *testing.T) {
	if _, ok := definitionReportKind(&ast.DescribeStmt{ObjectType: ast.DescribeEntity}); ok {
		t.Error("describe entity is a model element, not a definition report")
	}
	if _, ok := definitionReportKind(&ast.DescribeStmt{ObjectType: ast.DescribeContractEntity, Format: "mdl"}); ok {
		t.Error("describe contract entity … format mdl emits MDL")
	}
	if k, ok := definitionReportKind(&ast.DescribeStmt{ObjectType: ast.DescribeContractEntity}); !ok || k != "contract entity" {
		t.Errorf("contract entity: %q %v", k, ok)
	}
}
