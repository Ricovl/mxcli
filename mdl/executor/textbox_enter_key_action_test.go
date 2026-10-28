// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1743 (mendixlabs/mxcli#1662): a text box's "On enter key press"
// action — Forms$TextBox.OnEnterKeyPressAction — had no MDL property. Every
// spelling was reported as a property mxcli does not write and dropped on
// write, describe printed nothing, and mx check passed, so a search box that
// searches on Enter could not be generated. `OnChange:` on the same widget is
// the control: it was written and described.

const enterKeyPage = `create page ModK.SearchPage (Title: 'Search', Layout: Atlas_Core.Atlas_Default) {
  dataview dvQuery (DataSource: microflow ModK.DS_Query) {
    textbox tbChange (Label: 'On change', Attribute: SearchText, OnChange: call microflow ModK.ACT_Search(Query = $currentObject))
    textbox tbEnter1 (Label: 'Enter 1', Attribute: SearchText, OnEnterKeyPress: call microflow ModK.ACT_Search(Query = $currentObject))
    textbox tbEnter2 (Label: 'Enter 2', Attribute: SearchText, OnEnterKeyPressAction: call microflow ModK.ACT_Search(Query = $currentObject))
    textbox tbEnter3 (Label: 'Enter 3', Attribute: SearchText, OnEnterKey: call microflow ModK.ACT_Search(Query = $currentObject))
  }
};
`

// enterKeyViolations runs the page-widget checks over src and returns every
// violation, gated by the script's header.
func enterKeyViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	reg, _ := NewWidgetRegistry()
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		out = append(out, ValidateWidgetPropertiesForStatement(stmt, reg)...)
	}
	return GateWidgetViolations(out, prog.LanguageVersion)
}

func violationsNaming(vs []linter.Violation, widget string) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if strings.Contains(v.Message, "`"+widget+"`") {
			out = append(out, v)
		}
	}
	return out
}

func TestTextBoxEnterKeyPress_CheckAcceptsBothSpellings(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		vs := enterKeyViolations(t, header+enterKeyPage)
		for _, tb := range []string{"tbChange", "tbEnter1", "tbEnter2"} {
			if got := violationsNaming(vs, tb); len(got) > 0 {
				t.Errorf("header %q: %s is reported, but its action is written:\n%s", header, tb, violationText(got))
			}
		}
		// Control: a spelling that is not the property is still reported, and
		// pointed at the one that is.
		got := violationsNaming(vs, "tbEnter3")
		if len(got) == 0 {
			t.Fatalf("header %q: `OnEnterKey:` passed check — a misspelling must still be reported", header)
		}
		if !strings.Contains(got[0].Message, "OnEnterKeyPress") {
			t.Errorf("header %q: the message does not name `OnEnterKeyPress`:\n%s", header, violationText(got))
		}
	}
}

// buildEnterKeyTextBox runs the text box builder over one widget.
func buildEnterKeyTextBox(t *testing.T, w *ast.WidgetV3) (*pages.TextBox, error) {
	t.Helper()
	mod := mkModule("Mod")
	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)
	pb := newPageBuilder(&mock.MockBackend{}, h, "Mod")
	pb.entityContext = "Mod.Ent"
	built, err := pb.buildWidgetV3(w)
	if err != nil {
		return nil, err
	}
	tb, ok := built.(*pages.TextBox)
	if !ok {
		t.Fatalf("built %T, want *pages.TextBox", built)
	}
	return tb, nil
}

func enterKeyTextBox(props map[string]any) *ast.WidgetV3 {
	p := map[string]any{"Attribute": "Value", "Label": "L"}
	for k, v := range props {
		p[k] = v
	}
	return &ast.WidgetV3{Type: "textbox", Name: "tb", Properties: p}
}

func TestTextBoxEnterKeyPress_BuilderWritesTheAction(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
	}{
		{"OnEnterKeyPress", map[string]any{"OnEnterKeyPress": &ast.ActionV3{Type: "close"}}},
		{"stored name", map[string]any{"OnEnterKeyPressAction": &ast.ActionV3{Type: "close"}}},
		{"any case", map[string]any{"onenterkeypress": &ast.ActionV3{Type: "close"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tb, err := buildEnterKeyTextBox(t, enterKeyTextBox(c.props))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if tb.OnEnterKeyPressAction == nil {
				t.Fatal("OnEnterKeyPressAction is nil — the enter-key action was dropped by the builder")
			}
			if tb.OnChangeAction != nil {
				t.Errorf("OnChangeAction = %T; the enter-key action went into the change slot", tb.OnChangeAction)
			}
		})
	}
	// Control: a text box without one leaves the slot empty.
	tb, err := buildEnterKeyTextBox(t, enterKeyTextBox(nil))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if tb.OnEnterKeyPressAction != nil {
		t.Errorf("OnEnterKeyPressAction = %T on a text box that sets none", tb.OnEnterKeyPressAction)
	}
}

// `nanoflow M.F` (no `call`) and `$name` parse as a data source in a generic
// key's position; they are the flow call and the fragment action `OnChange:`
// accepts, and reach the builder as that action.
func TestTextBoxEnterKeyPress_FlowCallParsedAsDataSource(t *testing.T) {
	prog, errs := visitor.Build(`create page Mod.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: microflow Mod.DS) {
    textbox tb (Attribute: Value, OnEnterKeyPress: nanoflow Mod.NF)
  }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	tb := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children[0]
	if _, ok := tb.Properties["OnEnterKeyPress"].(*ast.DataSourceV3); !ok {
		t.Fatalf("OnEnterKeyPress parsed as %T; this test is about the data-source shape", tb.Properties["OnEnterKeyPress"])
	}
	a, err := inputEventValue("OnEnterKeyPress", tb.Properties["OnEnterKeyPress"])
	if err != nil {
		t.Fatalf("inputEventValue: %v", err)
	}
	if a == nil || a.Type != "nanoflow" || a.Target != "Mod.NF" {
		t.Errorf("action = %+v, want the nanoflow call Mod.NF", a)
	}
}

func TestTextBoxEnterKeyPress_RefusesWhatIsNotAnAction(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{"scalar", map[string]any{"OnEnterKeyPress": "TOTALLY_MADE_UP"}, "not an action"},
		{"database source", map[string]any{"OnEnterKeyPress": &ast.DataSourceV3{Type: "database", Reference: "Mod.Ent"}}, "database"},
		{"both spellings", map[string]any{
			"OnEnterKeyPress":       &ast.ActionV3{Type: "close"},
			"OnEnterKeyPressAction": &ast.ActionV3{Type: "save"},
		}, "both"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := buildEnterKeyTextBox(t, enterKeyTextBox(c.props))
			if err == nil {
				t.Fatal("built without error — the value would be written as no action")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
			// check says the same thing, as an error.
			w := enterKeyTextBox(c.props)
			vs := validateWidgetTree([]*ast.WidgetV3{w}, LoadWidgetRegistry(""), "page Mod.P")
			found := false
			for _, v := range vs {
				if v.Severity == linter.SeverityError && strings.Contains(v.Message, c.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("check reports no error mentioning %q:\n%s", c.want, violationText(vs))
			}
		})
	}
}

// A text area has no OnEnterKeyPressAction (only Forms$TextBox declares one),
// so the property is refused there, with the reason — not written, not
// dropped in silence.
func TestTextAreaEnterKeyPress_RefusedWithTheReason(t *testing.T) {
	src := `create page ModK.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: microflow ModK.DS_Query) {
    textarea taNotes (Attribute: SearchText, OnEnterKeyPress: call microflow ModK.ACT_Search(Query = $currentObject))
  }
};
`
	vs := violationsNaming(enterKeyViolations(t, "mdl 1;\n"+src), "taNotes")
	if len(vs) == 0 {
		t.Fatal("OnEnterKeyPress on a textarea passed check — it would be dropped on write")
	}
	if vs[0].RuleID != "MDL-WIDGET52" {
		t.Errorf("rule = %s, want MDL-WIDGET52 under mdl 1", vs[0].RuleID)
	}
	if !strings.Contains(vs[0].Message, "only a text box") {
		t.Errorf("the message does not say why a text area cannot take it:\n%s", violationText(vs))
	}
}

// DESCRIBE reads the stored slot back and prints it under the name the
// builder reads, so describe → exec keeps the action.
func TestDescribeTextBox_EnterKeyPressAction(t *testing.T) {
	ctx, buf := newMockCtx(t)
	raw := map[string]any{
		"$Type": "Forms$TextBox",
		"Name":  "tbSearch",
		"OnEnterKeyPressAction": map[string]any{
			"$Type": "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{
				"$Type":     "Forms$MicroflowSettings",
				"Microflow": "ModK.ACT_Search",
			},
		},
		"OnChangeAction": map[string]any{"$Type": "Forms$NoAction"},
	}
	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	outputWidgetMDLV3(ctx, got[0], 0)
	out := buf.String()
	if !strings.Contains(out, "OnEnterKeyPress: call microflow ModK.ACT_Search") {
		t.Fatalf("describe does not print the enter-key action:\n%s", out)
	}
	if strings.Contains(out, "OnChange:") {
		t.Errorf("describe printed an OnChange the widget does not have:\n%s", out)
	}

	// Control: a stored NoAction prints nothing.
	ctx2, buf2 := newMockCtx(t)
	raw["OnEnterKeyPressAction"] = map[string]any{"$Type": "Forms$NoAction"}
	outputWidgetMDLV3(ctx2, parseRawWidget(ctx2, raw)[0], 0)
	if out := buf2.String(); strings.Contains(out, "OnEnterKeyPress") {
		t.Errorf("a NoAction slot was described as an action:\n%s", out)
	}
}
