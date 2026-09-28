// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// R9 (ako/mxcli#755): documentation moves into a `/** … */` doc comment before
// the statement, the folder into a `folder '…'` clause after the name, and a
// workflow activity's `comment` becomes `caption`. Each rewrite must build the
// statements the original built.
func TestUpgrade_MetadataPlacement(t *testing.T) {
	for _, tc := range []struct {
		name, src, want, code string
	}{
		{"constant comment clause",
			"create constant M.Url type string default 'https://x'\n  comment 'Base URL';\n",
			"/** Base URL */\ncreate constant M.Url type string default 'https://x';\n",
			deprecation.DocumentationClause},
		{"indented statement keeps its indent",
			"  create constant M.Url type string default 'x' comment 'It''s the URL' exposed to client;\n",
			"  /** It's the URL */\n  create constant M.Url type string default 'x' exposed to client;\n",
			deprecation.DocumentationClause},
		{"association comment clause",
			"CREATE ASSOCIATION M.A_B FROM M.A TO M.B TYPE Reference COMMENT 'Links';\n",
			"/** Links */\nCREATE ASSOCIATION M.A_B FROM M.A TO M.B TYPE Reference;\n",
			deprecation.DocumentationClause},
		{"json structure comment clause",
			"create json structure M.J folder 'Json' comment 'Shape' sample '{\"a\": 1}';\n",
			"/** Shape */\ncreate json structure M.J folder 'Json' sample '{\"a\": 1}';\n",
			deprecation.DocumentationClause},
		{"image collection comment clause",
			"create image collection M.Icons export level 'Public' comment 'Icons';\n",
			"/** Icons */\ncreate image collection M.Icons export level 'Public';\n",
			deprecation.DocumentationClause},
		// A statement with both spellings keeps the documentation it stored:
		// on a constant the clause wins, so the doc comment is demoted to a
		// plain comment and the clause becomes the doc comment; on an
		// association the doc comment wins, so the clause is deleted.
		{"constant with both, the clause wins",
			"/**\n * Level 1: a constant\n */\ncreate constant M.Url type string default 'x'\ncomment 'Clause';\n",
			"/*\n * Level 1: a constant\n */\n/** Clause */\ncreate constant M.Url type string default 'x';\n",
			deprecation.DocumentationClause},
		{"association with both, the doc comment wins",
			"/** Doc */\ncreate association M.A_B from M.A to M.B comment 'Clause';\n",
			"/** Doc */\ncreate association M.A_B from M.A to M.B;\n",
			deprecation.DocumentationClause},
		{"documentation property, first",
			"create task queue M.Q (Documentation: 'Jobs', Parallelism: 2);\n",
			"/** Jobs */\ncreate task queue M.Q (Parallelism: 2);\n",
			deprecation.DocumentationProperty},
		{"documentation property, last",
			"create regular expression M.Zip (Expression: '[0-9]{4}', Documentation: 'Zip');\n",
			"/** Zip */\ncreate regular expression M.Zip (Expression: '[0-9]{4}');\n",
			deprecation.DocumentationProperty},
		{"page Folder property",
			"create page M.P (Title: 'P', Folder: 'Admin', Layout: Atlas_Core.Atlas_Default) { };\n",
			"create page M.P folder 'Admin' (Title: 'P', Layout: Atlas_Core.Atlas_Default) { };\n",
			deprecation.FolderProperty},
		{"snippet Folder property",
			"create snippet M.S (Params: { $C: M.E }, Folder: 'Common') { };\n",
			"create snippet M.S folder 'Common' (Params: { $C: M.E }) { };\n",
			deprecation.FolderProperty},
		{"snippet with Folder as its only property",
			"create snippet M.S(\n  folder: 'Snippets'\n)\n{ };\n",
			"create snippet M.S folder 'Snippets'\n{ };\n",
			deprecation.FolderProperty},
		{"consumed rest service Folder property, upper case",
			"CREATE CONSUMED REST SERVICE M.Api (BaseUrl: 'https://x', FOLDER: 'Integration', Authentication: none) { };\n",
			"CREATE CONSUMED REST SERVICE M.Api FOLDER 'Integration' (BaseUrl: 'https://x', Authentication: none) { };\n",
			deprecation.FolderProperty},
		{"published rest service Folder property",
			"create published rest service M.Api (Path: 'rest/api', Folder: 'It''s') { };\n",
			"create published rest service M.Api folder 'It''s' (Path: 'rest/api') { };\n",
			deprecation.FolderProperty},
		{"consumed odata service Folder property",
			"create consumed odata service M.Crm (Folder: 'Svc', Version: '1.0', ODataVersion: OData4, MetadataUrl: 'https://x/$metadata');\n",
			"create consumed odata service M.Crm folder 'Svc' (Version: '1.0', ODataVersion: OData4, MetadataUrl: 'https://x/$metadata');\n",
			deprecation.FolderProperty},
		{"workflow comment is the caption",
			"create workflow M.W parameter $WorkflowContext: M.E begin notification Ready COMMENT 'Ready'; end workflow;\n",
			"create workflow M.W parameter $WorkflowContext: M.E begin notification Ready CAPTION 'Ready'; end workflow;\n",
			deprecation.WorkflowCommentCaption},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mustUpgrade(t, tc.src, Options{})
			if res.Source != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", res.Source, tc.want)
			}
			if res.Rewritten[tc.code] != 1 {
				t.Errorf("Rewritten = %v, want one %s", res.Rewritten, tc.code)
			}
			before, errs := visitor.Build(tc.src)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			after, errs := visitor.Build(res.Source)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			if !reflect.DeepEqual(before.Statements, after.Statements) {
				t.Errorf("the rewrite changed the statement:\n before: %#v\n after:  %#v", before.Statements, after.Statements)
			}
			if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
				t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
			}
		})
	}
}

// A use the rewrite cannot move without guessing is reported and left alone:
// a doc comment that removing one `*` does not demote, a text a doc comment
// cannot hold, both folder spellings, and a property that is the only one in a
// list that may not be empty.
func TestUpgrade_MetadataPlacementUnrewritable(t *testing.T) {
	for _, src := range []string{
		"/*** Doc */\ncreate constant M.Url type string default 'x' comment 'Clause';\n",
		"create constant M.Url type string default 'x' comment 'ends with a space ';\n",
		"create constant M.Url type string default 'x' comment 'has */ in it';\n",
		"create page M.P folder 'A' (Title: 'P', Layout: Atlas_Core.Atlas_Default, Folder: 'B') { };\n",
		"create task queue M.Q (Documentation: 'Only');\n",
	} {
		res := mustUpgrade(t, src, Options{})
		if res.Source != src {
			t.Errorf("rewrote what it should have reported:\n%s\n->\n%s", src, res.Source)
		}
		if len(res.Unrewritten) != 1 {
			t.Errorf("%q: Unrewritten = %v, want one", src, res.Unrewritten)
		}
	}
}
