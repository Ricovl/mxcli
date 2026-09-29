// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func templateText(s string) *model.Text {
	return &model.Text{Translations: map[string]string{"en_US": s}}
}

// Under mdl 1 describe writes a template's line break into the literal, and a
// backslash as itself: a doubled apostrophe is the only escape (#746). The mdl 1 line parses
// back, under the header, to the stored template text.
func TestDescribeTemplateLineBreak(t *testing.T) {
	const stored = "It's done:\nC:\\temp\n"
	for _, tc := range []struct {
		action     microflows.MicroflowAction
		mdl1, mdl0 string
	}{
		{&microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'N'", MessageTemplate: templateText(stored)},
			"log node 'N' 'It''s done:\nC:\\temp\n';",
			`log node 'N' 'It''s done:\nC:\\temp\n';`},
		{&microflows.ShowMessageAction{Type: "Error", Template: templateText(stored), TemplateParameters: []string{"$S"}, Blocking: true},
			"show message 'It''s done:\nC:\\temp\n' type Error with ({1} = $S) blocking;",
			`show message 'It''s done:\nC:\\temp\n' type Error with ({1} = $S) blocking;`},
		{&microflows.ValidationFeedbackAction{ObjectVariable: "o", AttributeName: "M.E.Name", Template: templateText(stored)},
			"validation feedback $o/Name message 'It''s done:\nC:\\temp\n';",
			`validation feedback $o/Name message 'It''s done:\nC:\\temp\n';`},
	} {
		if got := formatAction(&ExecContext{LanguageVersion: langver.V1}, tc.action, nil, nil); got != tc.mdl1 {
			t.Errorf("mdl 1 describe:\n got  %q\n want %q", got, tc.mdl1)
		}
		// Control: outside an mdl 1 script describe keeps the mdl 0 escapes.
		if got := formatAction(nil, tc.action, nil, nil); got != tc.mdl0 {
			t.Errorf("mdl 0 describe:\n got  %q\n want %q", got, tc.mdl0)
		}

		for header, line := range map[string]string{"mdl 1;\n": tc.mdl1, "": tc.mdl0} {
			prog, errs := visitor.Build(header + "create microflow M.F ($o: M.E, $S: String) begin\n  " + line + "\nend;\n")
			if len(errs) > 0 {
				t.Fatalf("%q does not parse: %v", line, errs[0])
			}
			var msg ast.Expression
			switch s := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt).Body[0].(type) {
			case *ast.LogStmt:
				msg = s.Message
			case *ast.ShowMessageStmt:
				msg = s.Message
			case *ast.ValidationFeedbackStmt:
				msg = s.Message
			}
			if lit, ok := msg.(*ast.LiteralExpr); !ok || lit.Value != stored {
				t.Errorf("%q%s reads back as %#v, want the template text %q", header, line, msg, stored)
			}
		}
	}
}
