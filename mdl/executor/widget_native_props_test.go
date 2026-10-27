// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1698 (mendixlabs/mxcli#1651): `textbox tb (Attribute: Name,
// Caption: 'Item name')` passed check and exec, and the text box was written
// with no label — inputs take `Label:`. MDL-WIDGET07 judged keys against the
// union of every built-in widget's vocabulary, where Caption is a button's.

const itemEditPage = `create page M.ItemEdit (Title: 'Item', Layout: Atlas_Core.PopupLayout, Params: ($Job: M.Job)) {
  dataview dv (DataSource: $Job) {
    textbox tbCaption (Attribute: Title, Caption: 'Item name')
    textbox tbLabel (Attribute: Title, Label: 'Item name (Label)')
    actionbutton btnSave (Caption: 'Save', Action: save changes)
  }
};
`

// nativePropViolations runs the page-widget checks over the first statement
// and returns the ones this rule reports, gated by the script's header.
func nativePropViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	reg, _ := NewWidgetRegistry()
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, reg) {
			if v.RuleID == nativeWidgetPropDropped.Code || v.RuleID == "MDL-WIDGET52" {
				out = append(out, v)
			}
		}
	}
	return GateWidgetViolations(out, prog.LanguageVersion)
}

func violationText(vs []linter.Violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(v.RuleID + " " + v.Message + "\n")
	}
	return b.String()
}

func TestNativeWidgetProps_CaptionOnTextboxIsRefused(t *testing.T) {
	vs := nativePropViolations(t, "mdl 1;\n"+itemEditPage)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want exactly one — for tbCaption's Caption. Label on a text box and Caption "+
			"on a button are what those widgets take (controls):\n%s", len(vs), violationText(vs))
	}
	v := vs[0]
	if v.RuleID != "MDL-WIDGET52" || v.Severity != linter.SeverityError {
		t.Errorf("mdl 1: %s/%v, want an MDL-WIDGET52 error", v.RuleID, v.Severity)
	}
	for _, want := range []string{"tbCaption", "`Caption`", "Forms$TextBox", "`Label:`"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message lacks %q:\n%s", want, v.Message)
		}
	}
	if !strings.Contains(v.Suggestion, "Label") || strings.Contains(v.Suggestion, "Caption") {
		t.Errorf("suggestion should list what a text box takes (Label, not Caption): %s", v.Suggestion)
	}
}

// A script without the header keeps the old outcome and is told (ADR-0011).
func TestNativeWidgetProps_HeaderlessWarns(t *testing.T) {
	vs := nativePropViolations(t, itemEditPage)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want one:\n%s", len(vs), violationText(vs))
	}
	if vs[0].RuleID != nativeWidgetPropDropped.Code || vs[0].Severity != linter.SeverityWarning {
		t.Errorf("mdl 0: %s/%v, want a %s warning", vs[0].RuleID, vs[0].Severity, nativeWidgetPropDropped.Code)
	}
}

// The other direction of the mix-up, and a typo: each is a key the widget does
// not take, named with what was most likely meant.
func TestNativeWidgetProps_HintsNameTheIntendedProperty(t *testing.T) {
	vs := nativePropViolations(t, `mdl 1;
create page M.P (Title: 'x', Layout: A.L, Params: ($Job: M.Job)) {
  dataview dv (DataSource: $Job) {
    actionbutton btn (Label: 'Save', Action: save changes)
    dynamictext dt (Contnet: 'hi')
    groupbox gb (Label: 'Group') { dynamictext inner (Content: 'x') }
  }
};
`)
	text := violationText(vs)
	for _, want := range []string{
		"btn`: `Label`", "a button's text is `Caption:`",
		"dt`: `Contnet`", "did you mean `Content`?",
		"gb`: `Label`", "its text is `Caption:`",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no violation says %q:\n%s", want, text)
		}
	}
}

// Visible on an element whose stored type has no ConditionalVisibilitySettings
// was applied to the Go struct by buildWidgetV3's tail and then not encoded.
// Control: the same key on a container, which stores it.
func TestNativeWidgetProps_VisibleWhereNotStored(t *testing.T) {
	vs := nativePropViolations(t, `mdl 1;
create page M.P (Title: 'x', Layout: A.L) {
  snippetcall sc (Snippet: M.S, Visible: false)
  container c (Visible: false) { dynamictext t (Content: 'x') }
};
`)
	text := violationText(vs)
	if len(vs) != 1 || !strings.Contains(text, "sc`: `Visible`") || !strings.Contains(text, "Forms$SnippetCallWidget has no conditional visibility") {
		t.Errorf("want one refusal, for the snippet call's Visible:\n%s", text)
	}
}

// A layout's elements: a region takes Class, Size, SizeMode and ToggleMode; its
// Style was read by nothing. Control: those four.
func TestNativeWidgetProps_LayoutRegions(t *testing.T) {
	vs := nativePropViolations(t, `mdl 1;
create layout M.L (layouttype: 'Responsive') {
  scrollcontainer sc {
    region top (Size: 48, SizeMode: 'Pixels', ToggleMode: 'None', Class: 'top') { placeholder Top }
    region center (Style: 'color: red') { placeholder Main }
  }
};
`)
	text := violationText(vs)
	if len(vs) != 1 || !strings.Contains(text, "scrollcontainer region `center`: `Style`") ||
		!strings.Contains(text, "only `Class:` is written") {
		t.Errorf("want one refusal, for the centre region's Style:\n%s", text)
	}
}

// A positional keyword is judged only where its parent is a built-in element.
// A data grid's `column` (an object-list item of a pluggable widget) and the
// root of an ALTER … INSERT (whose parent is out of sight) are not this rule's;
// a layout-grid column is.
func TestNativeWidgetProps_PositionalKeywords(t *testing.T) {
	vs := nativePropViolations(t, `mdl 1;
create page M.P (Title: 'x', Layout: A.L) {
  pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid' dg (DataSource: database M.Job) {
    column colTitle (Attribute: Title, Caption: 'Title')
  }
  layoutgrid lg {
    row {
      column (DesktopWidth: 6, Caption: 'not a column property') { dynamictext t (Content: 'x') }
    }
  }
};
alter page M.Q {
  insert after dg.colA {
    column colB (Attribute: Title, Caption: 'B')
  }
};
`)
	text := violationText(vs)
	if len(vs) != 1 || !strings.Contains(text, "layoutgrid column: `Caption`") {
		t.Errorf("want one refusal, for the layout-grid column's Caption — a data grid column's Caption and "+
			"Attribute are its own:\n%s", text)
	}
}

// exec applies the same verdict, so `exec --no-check` refuses too; without
// the header it warns and writes what it wrote before. Controls: the text box
// with Label: and the button with Caption: keep them.
func TestNativeWidgetProps_ExecRefuses(t *testing.T) {
	prog, errs := visitor.Build(itemEditPage)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	// The data view's children, built in its context (M.Job).
	children := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children
	build := func(v langver.Version, w *ast.WidgetV3) (pages.Widget, string, error) {
		var out bytes.Buffer
		pb := visibleWhenPB("M.Job")
		pb.ctx = &ExecContext{Output: &out, LanguageVersion: v}
		built, err := pb.buildWidgetV3(w)
		return built, out.String(), err
	}

	if _, _, err := build(langver.V1, children[0]); err == nil || !strings.Contains(err.Error(), "MDL-WIDGET52") ||
		!strings.Contains(err.Error(), "Caption") {
		t.Errorf("mdl 1: err = %v, want an MDL-WIDGET52 refusal naming Caption", err)
	}
	built, out, err := build(langver.V0, children[0])
	if err != nil {
		t.Fatalf("mdl 0: refused (%v) — a new refusal applies only under the header", err)
	}
	if !strings.Contains(out, nativeWidgetPropDropped.Code) || !strings.Contains(out, "tbCaption") {
		t.Errorf("mdl 0: the drop was silent, want the %s warning; output:\n%s", nativeWidgetPropDropped.Code, out)
	}
	if tb := built.(*pages.TextBox); tb.Label != "" {
		t.Errorf("tbCaption's Label = %q: Caption is not mapped to the label, it is refused", tb.Label)
	}

	// Controls, under mdl 1: what each widget does take is written.
	built, _, err = build(langver.V1, children[1])
	if err != nil {
		t.Fatalf("control: tbLabel refused: %v", err)
	}
	if tb := built.(*pages.TextBox); tb.Label != "Item name (Label)" {
		t.Errorf("control: tbLabel's Label = %q, want it written", tb.Label)
	}
	built, _, err = build(langver.V1, children[2])
	if err != nil {
		t.Fatalf("control: btnSave refused: %v", err)
	}
	if btn := built.(*pages.ActionButton); btn.CaptionTemplate == nil {
		t.Errorf("control: btnSave's Caption was not written")
	}
}

// Every kind's own keys are accepted by the verdict — no kind refuses what its
// builder reads (a sanity net under the source-derived test).
func TestNativeWidgetProps_EveryReadIsAccepted(t *testing.T) {
	for kind, k := range nativeWidgetKinds {
		keyword := kind
		if i := strings.LastIndex(kind, " "); i >= 0 {
			keyword = kind[i+1:]
		}
		for _, key := range k.reads {
			if !nativeWidgetAccepts(kind, keyword, key) {
				t.Errorf("%s: reads %s but does not accept it", kind, key)
			}
		}
	}
}
