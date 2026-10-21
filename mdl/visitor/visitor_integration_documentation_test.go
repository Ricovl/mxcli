// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1510: QUAL002 asks for documentation on import and export
// mappings, published REST services and data transformers, and a `/** … */`
// doc comment above their create was dropped with MDL089 — the statements had
// nowhere to put it, so the finding could never be closed from MDL. Each now
// reads the comment, and records whether one was written at all, so a rewrite
// that states none keeps the stored documentation (#1018).
func TestIntegrationDocumentsReadDocComment_Issue1510(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		doc  func(ast.Statement) (string, bool)
	}{{
		name: "import mapping",
		src: "create import mapping M.IMM_Order\n  with json structure M.JSON_Order\n" +
			"{\n  create M.Order {\n    Id = id\n  }\n};",
		doc: func(s ast.Statement) (string, bool) {
			st := s.(*ast.CreateImportMappingStmt)
			return st.Documentation, st.DocumentationSet
		},
	}, {
		name: "export mapping",
		src: "create export mapping M.EMM_Order\n  with json structure M.JSON_Order\n" +
			"{\n  M.Order {\n    id = Id\n  }\n};",
		doc: func(s ast.Statement) (string, bool) {
			st := s.(*ast.CreateExportMappingStmt)
			return st.Documentation, st.DocumentationSet
		},
	}, {
		name: "published rest service",
		src:  "create or modify published rest service M.OrdersApi (\n  Path: 'rest/orders/v1'\n)\n{\n};",
		doc: func(s ast.Statement) (string, bool) {
			st := s.(*ast.CreatePublishedRestServiceStmt)
			return st.Documentation, st.DocumentationSet
		},
	}, {
		name: "data transformer",
		src:  "create data transformer M.Flatten\n  source json '{\"a\": 1}'\n{\n  jslt '{\"b\": .a}';\n};",
		doc: func(s ast.Statement) (string, bool) {
			st := s.(*ast.CreateDataTransformerStmt)
			return st.Documentation, st.DocumentationSet
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build("/** Line one.\n *\n * Line two. */\n" + tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			if len(prog.DetachedDocComments) != 0 {
				t.Errorf("doc comment reported as lost (MDL089): %+v", prog.DetachedDocComments)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("got %d statements, want 1", len(prog.Statements))
			}
			doc, set := tc.doc(prog.Statements[0])
			if !set || doc != "Line one.\n\nLine two." {
				t.Errorf("documentation = %q (set %v), want %q", doc, set, "Line one.\n\nLine two.")
			}

			// Control: no comment is "not stated", which a rewrite must read as
			// "keep what is stored", not as "clear it".
			prog, errs = Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			if doc, set := tc.doc(prog.Statements[0]); set || doc != "" {
				t.Errorf("without a comment: documentation = %q (set %v), want unset", doc, set)
			}
		})
	}
}
