// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1510: QUAL002 reports import and export mappings,
// published REST services and data transformers without documentation, and MDL
// had no way to write it — the doc comment above their create was dropped with
// MDL089. Worse, a rewrite wrote an empty Documentation over whatever Studio Pro
// had stored, so copying a mapping through describe -> exec deleted its text.
//
// Measured on a copy of the Studio Pro-authored PedApp (11.13), with the real
// engine, so what is asserted is the stored document's own `Documentation` key.

type issue1510Doctype struct {
	name     string
	create   string // without its doc comment
	rewrite  string // a create or modify that states none
	describe string
	unitID   func(t *testing.T, ctx *ExecContext) model.ID
}

func issue1510Doctypes() []issue1510Doctype {
	return []issue1510Doctype{{
		name: "import mapping",
		create: "create import mapping MyFirstModule.IMM_Doc1510\n  with json structure MyFirstModule.JSON_Doc1510\n" +
			"{\n  create MyFirstModule.Doc1510 {\n    Code = code\n  }\n};",
		rewrite: "create or modify import mapping MyFirstModule.IMM_Doc1510\n  with json structure MyFirstModule.JSON_Doc1510\n" +
			"{\n  create MyFirstModule.Doc1510 {\n    Code = code,\n    Amount = amount\n  }\n};",
		describe: "describe import mapping MyFirstModule.IMM_Doc1510",
		unitID: func(t *testing.T, ctx *ExecContext) model.ID {
			m, err := ctx.Backend.GetImportMappingByQualifiedName("MyFirstModule", "IMM_Doc1510")
			if err != nil {
				t.Fatal(err)
			}
			return m.ID
		},
	}, {
		name: "export mapping",
		create: "create export mapping MyFirstModule.EMM_Doc1510\n  with json structure MyFirstModule.JSON_Doc1510\n" +
			"{\n  MyFirstModule.Doc1510 {\n    code = Code\n  }\n};",
		rewrite: "create or modify export mapping MyFirstModule.EMM_Doc1510\n  with json structure MyFirstModule.JSON_Doc1510\n" +
			"{\n  MyFirstModule.Doc1510 {\n    code = Code,\n    amount = Amount\n  }\n};",
		describe: "describe export mapping MyFirstModule.EMM_Doc1510",
		unitID: func(t *testing.T, ctx *ExecContext) model.ID {
			m, err := ctx.Backend.GetExportMappingByQualifiedName("MyFirstModule", "EMM_Doc1510")
			if err != nil {
				t.Fatal(err)
			}
			return m.ID
		},
	}, {
		name:     "published rest service",
		create:   "create published rest service MyFirstModule.Api_Doc1510 (\n  Path: 'rest/doc1510/v1',\n  Version: '1.0.0'\n)\n{\n};",
		rewrite:  "create or modify published rest service MyFirstModule.Api_Doc1510 (\n  Path: 'rest/doc1510/v2',\n  Version: '2.0.0'\n)\n{\n};",
		describe: "describe published rest service MyFirstModule.Api_Doc1510",
		unitID: func(t *testing.T, ctx *ExecContext) model.ID {
			svc, err := findPublishedRestService(ctx, "MyFirstModule", "Api_Doc1510")
			if err != nil {
				t.Fatal(err)
			}
			return svc.ID
		},
	}, {
		name:     "data transformer",
		create:   "create data transformer MyFirstModule.DT_Doc1510\n  source json '{\"a\": 1}'\n{\n  jslt '{\"b\": .a}';\n};",
		rewrite:  "create or modify data transformer MyFirstModule.DT_Doc1510\n  source json '{\"a\": 2}'\n{\n  jslt '{\"c\": .a}';\n};",
		describe: "describe data transformer MyFirstModule.DT_Doc1510",
		unitID: func(t *testing.T, ctx *ExecContext) model.ID {
			_, id := findDataTransformer(ctx, "MyFirstModule", "DT_Doc1510")
			if id == "" {
				t.Fatal("data transformer not found")
			}
			return id
		},
	}}
}

const issue1510Prereqs = "create persistent entity MyFirstModule.Doc1510 (Code: String(20), Amount: Integer);\n" +
	"create json structure MyFirstModule.JSON_Doc1510 sample '{\"code\": \"x\", \"amount\": 1}';\n"

// storedDocumentation is the unit's top-level Documentation key, and whether
// the key is there at all.
func storedDocumentation(t *testing.T, ctx *ExecContext, id model.ID) (string, bool) {
	t.Helper()
	raw, err := ctx.Backend.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	for _, e := range d {
		if e.Key == "Documentation" {
			s, _ := e.Value.(string)
			return s, true
		}
	}
	return "", false
}

func TestIntegrationDocumentation_Issue1510(t *testing.T) {
	const doc = "Doc 1510: the orders payload.\n\nSecond paragraph."
	const comment = "/**\n * Doc 1510: the orders payload.\n *\n * Second paragraph.\n */\n"
	for _, tc := range issue1510Doctypes() {
		t.Run(tc.name, func(t *testing.T) {
			exec, out := openPedAppFixture(t)
			ctx := exec.newExecContext(context.Background())
			run := func(src string) {
				t.Helper()
				if err := afRun(t, exec, src); err != nil {
					t.Fatalf("%v\n%s", err, src)
				}
			}
			describe := func() string {
				t.Helper()
				out.Reset()
				run(tc.describe + ";")
				return out.String()
			}
			run(issue1510Prereqs)

			// 1. The doc comment reaches the stored document's Documentation.
			run(comment + tc.create)
			id := tc.unitID(t, ctx)
			if got, ok := storedDocumentation(t, ctx, id); !ok || got != doc {
				t.Fatalf("stored Documentation = %q (key present %v), want %q", got, ok, doc)
			}

			// 2. Describe prints it back as the doc comment, and the output
			// re-executes to the same text: the round trip.
			described := describe()
			if !strings.Contains(described, comment) {
				t.Fatalf("describe does not print the documentation as a doc comment:\n%s", described)
			}
			run(described)
			if got, _ := storedDocumentation(t, ctx, id); got != doc {
				t.Errorf("describe -> exec changed the documentation to %q", got)
			}

			// 3. A rewrite that states none keeps the stored text (#1018).
			run(tc.rewrite)
			if got, _ := storedDocumentation(t, ctx, id); got != doc {
				t.Errorf("a rewrite with no doc comment changed the documentation to %q, want it kept", got)
			}

			// 4. Control: an empty doc comment clears it, so the carry is the
			// absence of a comment and not a write path that ignores the text.
			run("/** */\n" + tc.rewrite)
			if got, ok := storedDocumentation(t, ctx, id); !ok || got != "" {
				t.Errorf("an empty doc comment left Documentation = %q (key present %v), want empty", got, ok)
			}
		})
	}
}

// The Studio Pro-authored mappings in PedApp: a rewrite through describe ->
// exec, and one with no doc comment, must not delete documentation Studio Pro
// wrote. Before #1510 the executor never set it, so every rewrite stored "".
func TestIntegrationDocumentation_StudioProMappingKeepsIt_Issue1510(t *testing.T) {
	exec, out := openPedAppFixture(t)
	ctx := exec.newExecContext(context.Background())
	im, err := ctx.Backend.GetImportMappingByQualifiedName("FeedbackModule", "IMM_PostResponse")
	if err != nil {
		t.Fatalf("fixture mapping: %v", err)
	}
	// Give the Studio Pro mapping documentation the way Studio Pro would store
	// it, without going through the executor being tested.
	im.Documentation = "Written in Studio Pro."
	if err := ctx.Backend.UpdateImportMapping(im); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := afRun(t, exec, "describe import mapping FeedbackModule.IMM_PostResponse;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, "Written in Studio Pro.") {
		t.Fatalf("precondition: describe does not show the documentation:\n%s", described)
	}
	// Strip the doc comment: the statement now states none.
	body := described[strings.Index(described, "create or modify"):]
	if err := afRun(t, exec, body); err != nil {
		t.Fatalf("re-execute describe body: %v\n%s", err, body)
	}
	if got, _ := storedDocumentation(t, ctx, im.ID); got != "Written in Studio Pro." {
		t.Errorf("a rewrite with no doc comment changed Studio Pro's documentation to %q", got)
	}
}
