// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R2 (ako/mxcli#754): properties in ( ), declarative children in { }. For each
// integration document the canonical form must build without a warning under
// both versions, and the old form must build the SAME statement, record its
// code once, and carry a rewrite.

type r2Case struct {
	name      string
	code      string
	old       string
	canonical string
}

var r2Cases = []r2Case{
	{
		name: "rest operation",
		code: deprecation.RestOperationBraces,
		old: `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation GetUser { Method: get, Path: '/u/{id}', Parameters: ($id: Integer), Headers: ('Accept': 'application/json'), Response: none }
  operation Ping { Method: get, Path: '/ping', Response: none }
};`,
		canonical: `create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {
  operation GetUser ( Method: get, Path: '/u/{id}', Parameters: ($id: Integer), Headers: ('Accept': 'application/json'), Response: none, )
  operation Ping ( Method: get, Path: '/ping', Response: none )
};`,
	},
	{
		name: "agent attachments",
		code: deprecation.AgentAttachmentBraces,
		old: `create agent M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x') {
  mcp service M.Weather { Enabled: true }
  knowledge base Docs { Source: M.KB, Collection: 'docs', MaxResults: 3 }
  tool Lookup { Description: 'Find', Enabled: true }
};`,
		canonical: `create agent M.A (UsageType: Task, Model: M.Gpt, SystemPrompt: 'x') {
  mcp service M.Weather ( Enabled: true )
  knowledge base Docs ( Source: M.KB, Collection: 'docs', MaxResults: 3, )
  tool Lookup ( Description: 'Find', Enabled: true )
};`,
	},
	{
		name:      "alter agent add",
		code:      deprecation.AgentAttachmentBraces,
		old:       `alter agent M.A add tool Lookup { Description: 'Find' };`,
		canonical: `alter agent M.A add tool Lookup ( Description: 'Find' );`,
	},
	{
		name: "image collection",
		code: deprecation.ImageCollectionParens,
		old: `create image collection M.Icons export level 'Public' (
  image Logo from file 'assets/logo.png',
  image "Home" from file 'assets/home.png'
);`,
		canonical: `create image collection M.Icons export level 'Public' {
  image Logo ( File: 'assets/logo.png' )
  image "Home" ( File: 'assets/home.png', )
};`,
	},
	{
		name: "message definition collection",
		code: deprecation.MessageTreeParens,
		old: `create message definition collection M.Msgs folder 'Messages' (
  definition Order for M.Order as 'Orders' (
    Number,
    M.Order_Line/M.Line as 'Lines' ( Sku, Quantity example '3' )
  ),
  definition Customer for M.Customer ( Name )
);`,
		canonical: `create message definition collection M.Msgs folder 'Messages' {
  definition Order for M.Order as 'Orders' {
    Number,
    M.Order_Line/M.Line as 'Lines' { Sku, Quantity example '3' }
  }
  definition Customer for M.Customer { Name }
};`,
	},
	{
		name:      "add definition",
		code:      deprecation.MessageTreeParens,
		old:       `alter message definition collection M.Msgs add definition X for M.X as 'Xs' ( A, M.X_Y/M.Y ( B ) );`,
		canonical: `alter message definition collection M.Msgs add definition X for M.X as 'Xs' { A, M.X_Y/M.Y { B } };`,
	},
	{
		name: "alter microflow fragments",
		code: deprecation.AlterFlowFragmentBraces,
		old: `alter microflow M.F {
  insert after $IsValid { log info node 'F' 'checked'; }
  insert before 'Save order' { if $X then log info 'y'; end if; };
  replace commit $Order with { commit $Order with events; }
  drop log * node 'Debug' *;
};`,
		canonical: `alter microflow M.F {
  insert after $IsValid begin log info node 'F' 'checked'; end
  insert before 'Save order' begin if $X then log info 'y'; end if; end;
  replace commit $Order with begin commit $Order with events; end;
  drop log * node 'Debug' *;
};`,
	},
	{
		name:      "add member",
		code:      deprecation.MessageTreeParens,
		old:       `alter message definition M.Msgs.Order add member M.Order_Line/M.Line ( Sku );`,
		canonical: `alter message definition M.Msgs.Order add member M.Order_Line/M.Line { Sku };`,
	},
}

func buildNoErrors(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q: unexpected errors: %v", src, errs)
	}
	return prog
}

func TestR2Children_CanonicalFormBuildsWithoutWarning(t *testing.T) {
	for _, c := range r2Cases {
		for _, header := range []string{"", "mdl 1;\n"} {
			prog := buildNoErrors(t, header+c.canonical)
			if len(prog.Statements) != 1 {
				t.Errorf("%s, header %q: %d statements", c.name, header, len(prog.Statements))
			}
			if len(prog.Deprecations) != 0 {
				t.Errorf("%s, header %q: the canonical form recorded %+v", c.name, header, prog.Deprecations)
			}
		}
	}
}

func TestR2Children_OldFormIsADeprecatedAlias(t *testing.T) {
	for _, c := range r2Cases {
		for _, header := range []string{"", "mdl 1;\n"} {
			old := buildNoErrors(t, header+c.old)
			canon := buildNoErrors(t, header+c.canonical)
			if n := countDeprecations(old, c.code); n != 1 {
				t.Errorf("%s, header %q: recorded %s %d times, want once per statement (%+v)", c.name, header, c.code, n, old.Deprecations)
			}
			for _, d := range old.Deprecations {
				if d.Code == c.code && (d.Fix == nil || len(d.Fix.Edits) == 0) {
					t.Errorf("%s: %s carries no rewrite", c.name, c.code)
				}
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("%s, header %q: the two forms build different statements:\nold:   %#v\ncanon: %#v",
					c.name, header, old.Statements[0], canon.Statements[0])
			}
		}
	}
}

// The recorded rewrite, applied to the old form, gives a script that records
// nothing and builds the same statement.
func TestR2Children_RewriteReachesTheCanonicalForm(t *testing.T) {
	for _, c := range r2Cases {
		prog := buildNoErrors(t, c.old)
		out := applyDeprecationFixes(t, c.old, prog, c.code)
		again := buildNoErrors(t, out)
		if len(again.Deprecations) != 0 {
			t.Errorf("%s: the rewrite still records %+v:\n%s", c.name, again.Deprecations, out)
		}
		if !reflect.DeepEqual(prog.Statements, again.Statements) {
			t.Errorf("%s: the rewrite changed the statement:\n%s", c.name, out)
		}
	}
}

func applyDeprecationFixes(t *testing.T, src string, prog *ast.Program, code string) string {
	t.Helper()
	runes := []rune(src)
	var edits []ast.TextEdit
	for _, d := range prog.Deprecations {
		if d.Code == code && d.Fix != nil {
			edits = append(edits, d.Fix.Edits...)
		}
	}
	// Apply from the end so earlier offsets stay valid.
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			if edits[j].Start > edits[i].Start {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	for _, e := range edits {
		runes = append(runes[:e.Start], append([]rune(e.Text), runes[e.Stop:]...)...)
	}
	return string(runes)
}

func TestR2Children_ImageRejectsUnknownOrMissingFile(t *testing.T) {
	for src, want := range map[string]string{
		`create image collection M.I { image Logo ( Path: 'x.png' ) };`:              "unknown property 'Path'",
		`create image collection M.I { image Logo ( File: 'x.png', Size: 'big' ) };`: "unknown property 'Size'",
	} {
		_, errs := Build(src)
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), want) {
			t.Errorf("%q: errors %v, want one containing %q", src, errs, want)
		}
	}
}

func TestR2Children_EmptyAssociationTree(t *testing.T) {
	prog := buildNoErrors(t, `create message definition collection M.Msgs { definition O for M.O { M.O_L/M.L { } } };`)
	s := prog.Statements[0].(*ast.CreateMessageDefinitionCollectionStmt)
	m := s.Definitions[0].Members[0]
	if m.Entity.Name != "L" || len(m.Members) != 0 {
		t.Errorf("member = %+v", m)
	}
}
