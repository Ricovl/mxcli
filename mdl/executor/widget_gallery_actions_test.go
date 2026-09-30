// SPDX-License-Identifier: Apache-2.0

// A Gallery's row action (ako/mxcli#842).
//
// `gallery g (…, onClick: call nanoflow M.Open(Item = $currentObject))` passed
// `check`, `exec` printed "Created page", and nothing was stored: the Gallery is
// one of the widgets whose definition is HAND-WRITTEN and embedded
// (sdk/widgets/definitions), not generated from its .mpk, and the generator is
// the only thing that learnt to emit action mappings (ledger #67). With no
// mapping the engine had nothing to write, and because `onClick:` is a builtin
// MDL property name the validator accepted it on every widget without asking
// whether this one routes it. Re-executing the page with only the onClick
// changed therefore rebuilt byte-identical BSON and reported "Unchanged".
//
// The data grid beside it kept its row action all along — its definition is
// generated — which is why the drop was specific to the Gallery.
package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The Gallery's definition maps all three of its action slots: the row click
// under the `onClick:` keyword, the other two by their own keys — the same
// shape GenerateDefJSON gives the data grid.
func TestGalleryDefMapsItsActionSlots(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("gallery")
	if !ok {
		t.Fatal("gallery not registered")
	}
	got := map[string]string{}
	for _, m := range def.PropertyMappings {
		if m.Operation == "action" {
			got[m.PropertyKey] = m.Source
		}
	}
	want := map[string]string{
		"onClick":               "OnClick",
		"onSelectionChange":     "",
		"onConfigurationChange": "",
	}
	for k, src := range want {
		s, ok := got[k]
		if !ok {
			t.Errorf("gallery has no action mapping for %q — an action written on it "+
				"is silently dropped (#842)", k)
			continue
		}
		if s != src {
			t.Errorf("%s: source %q, want %q", k, s, src)
		}
	}
}

// `onClick:` / `OnChange:` are builtin MDL names, accepted on every widget. On a
// definition-driven widget that has no slot for them they were dropped on write
// with nothing said; they are refused instead. The control rows are widgets that
// DO route them — the rule must not fire there, or it refuses working pages.
func TestValidate_ActionKeywordWithoutASlotIsRefused(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	act := &ast.ActionV3{Type: "nanoflow", Target: "M.Handler"}
	cases := []struct {
		widgetType string
		key        string
		refused    bool
	}{
		{"gallery", "Action", false},     // #842: written now
		{"image", "Action", false},       // control: always routed
		{"combobox", "OnChange", false},  // control: routed through both modes
		{"combobox", "Action", true},     // a combo box has no click slot
		{"textfilter", "OnChange", true}, // the filters' onChange has no mapping
		{"dropdownsort", "Action", true}, // no action slot at all
	}
	for _, c := range cases {
		t.Run(c.widgetType+" "+c.key, func(t *testing.T) {
			w := &ast.WidgetV3{Type: c.widgetType, Name: "w1", Properties: map[string]any{c.key: act}}
			var found bool
			for _, v := range validatePluggableWidgetProperties(w, reg, "page M.P") {
				if v.RuleID == actionSlotRefused.Code {
					found = true
				}
			}
			if found != c.refused {
				t.Errorf("refused = %v, want %v", found, c.refused)
			}
		})
	}
}

// buildGalleryActionWidget is a stored Gallery whose onClick and
// onSelectionChange slots each hold a nanoflow call.
func buildGalleryActionWidget() map[string]any {
	nf := func(name string) map[string]any {
		return map[string]any{
			"$Type":    "Forms$CallNanoflowClientAction",
			"Nanoflow": name,
		}
	}
	return map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "gal",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.gallery.Gallery",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{"$ID": "t-click", "PropertyKey": "onClick"},
					map[string]any{"$ID": "t-sel", "PropertyKey": "onSelectionChange"},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{"TypePointer": "t-click", "Value": map[string]any{"Action": nf("M.OpenCard")}},
				map[string]any{"TypePointer": "t-sel", "Value": map[string]any{"Action": nf("M.Selected")}},
			},
		},
	}
}

// DESCRIBE reads a Gallery's action slots back. Without this the write fix is
// one-way: describe → exec deletes the row action, and the pluggable passthrough
// (#721 L4) cannot tell a statement that removed it from one that kept it.
func TestParseRawWidget_GalleryReadsItsActions(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	got := parseRawWidget(ctx, buildGalleryActionWidget(), "")
	if len(got) != 1 {
		t.Fatalf("parseRawWidget returned %d widgets, want 1", len(got))
	}
	g := got[0]
	if !strings.Contains(g.OnClick, "M.OpenCard") {
		t.Errorf("OnClick = %q, want the M.OpenCard call", g.OnClick)
	}
	var sel string
	for _, na := range g.NamedActions {
		if na.Key == "onSelectionChange" {
			sel = na.MDL
		}
		if na.Key == "onClick" {
			t.Errorf("onClick read twice: once as OnClick and again as a named slot")
		}
	}
	if !strings.Contains(sel, "M.Selected") {
		t.Errorf("onSelectionChange = %q, want the M.Selected call (named actions: %v)", sel, g.NamedActions)
	}
}

func TestDescribeEmitsGalleryActions(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type:         "CustomWidgets$CustomWidget",
		RenderMode:   "gallery",
		Name:         "gal",
		OnClick:      "call nanoflow M.OpenCard",
		NamedActions: []rawNamedAction{{Key: "onSelectionChange", MDL: "call nanoflow M.Selected"}},
	}, 0)
	out := buf.String()
	for _, want := range []string{"onClick: call nanoflow M.OpenCard", "onSelectionChange: call nanoflow M.Selected"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe has no %q:\n%s", want, out)
		}
	}
}

// The build applies what check applies, so `exec --no-check` cannot drop the
// action in silence either: refused under mdl 1, dropped with the
// MDL-V1-ACTIONSLOT warning under mdl 0 (ADR-0011: a new refusal applies only
// under the header). Control: the same action on a definition that routes it.
func TestEngineRefusesAnUnroutedActionKeyword(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	act := &ast.ActionV3{Type: "nanoflow", Target: "M.Handler"}
	sort, _ := reg.Get("dropdownsort")
	w := &ast.WidgetV3{Type: "dropdownsort", Name: "s1", Properties: map[string]any{"Action": act}}

	var out bytes.Buffer
	v1 := &ExecContext{Output: &out, LanguageVersion: langver.V1}
	if err := refuseUnroutedActionKeywords(v1, sort, sort.PropertyMappings, w); err == nil {
		t.Error("mdl 1: an onClick on a widget with no click slot was accepted — it is dropped on write")
	}

	out.Reset()
	v0 := &ExecContext{Output: &out, LanguageVersion: langver.V0}
	if err := refuseUnroutedActionKeywords(v0, sort, sort.PropertyMappings, w); err != nil {
		t.Errorf("mdl 0: refused (%v) — a new refusal applies only under the header", err)
	}
	if !strings.Contains(out.String(), actionSlotRefused.Code) {
		t.Errorf("mdl 0: the drop was silent, want the %s warning; output:\n%s", actionSlotRefused.Code, out.String())
	}

	gal, _ := reg.Get("gallery")
	g := &ast.WidgetV3{Type: "gallery", Name: "g1", Properties: map[string]any{"Action": act}}
	if err := refuseUnroutedActionKeywords(v1, gal, gal.PropertyMappings, g); err != nil {
		t.Errorf("the gallery's routed onClick was refused: %v", err)
	}
}

// The check-time twin: an error under the rule's own ID under mdl 1, a warning
// that names the change under mdl 0; violations of other rules are untouched.
func TestGateWidgetViolations(t *testing.T) {
	mk := func() []linter.Violation {
		return []linter.Violation{
			{RuleID: actionSlotRefused.Code, Severity: linter.SeverityError, Message: "a"},
			{RuleID: galleryClickRefused.Code, Severity: linter.SeverityError, Message: "b"},
			{RuleID: "MDL-WIDGET01", Severity: linter.SeverityError, Message: "c"},
		}
	}
	v1 := GateWidgetViolations(mk(), langver.V1)
	for i, want := range []string{"MDL-WIDGET37", "MDL-WIDGET36", "MDL-WIDGET01"} {
		if v1[i].RuleID != want || v1[i].Severity != linter.SeverityError {
			t.Errorf("mdl 1 [%d]: %s/%v, want %s error", i, v1[i].RuleID, v1[i].Severity, want)
		}
	}
	v0 := GateWidgetViolations(mk(), langver.V0)
	for i, want := range []string{actionSlotRefused.Code, galleryClickRefused.Code} {
		if v0[i].RuleID != want || v0[i].Severity != linter.SeverityWarning {
			t.Errorf("mdl 0 [%d]: %s/%v, want %s warning", i, v0[i].RuleID, v0[i].Severity, want)
		}
		if !strings.Contains(v0[i].Message, "mdl 1") {
			t.Errorf("mdl 0 [%d]: the warning does not say what the header changes: %q", i, v0[i].Message)
		}
	}
	if v0[2].Severity != linter.SeverityError {
		t.Error("mdl 0: an ungated rule was downgraded")
	}
}

// The two settings that make a row click legal on a Gallery must survive
// describe → exec, or the round trip produces the ambiguous gallery MDL-WIDGET36
// refuses: `Selection: None` (describe used to omit None, and exec's default is
// Single) and a double-click trigger (describe never read it; exec's is single).
func TestParseRawWidget_GalleryReadsSelectionNoneAndTrigger(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	w := buildGalleryActionWidget()
	typ := w["Type"].(map[string]any)["ObjectType"].(map[string]any)
	typ["PropertyTypes"] = append(typ["PropertyTypes"].([]any),
		map[string]any{"$ID": "t-isel", "PropertyKey": "itemSelection"},
		map[string]any{"$ID": "t-trig", "PropertyKey": "onClickTrigger"})
	obj := w["Object"].(map[string]any)
	obj["Properties"] = append(obj["Properties"].([]any),
		map[string]any{"TypePointer": "t-isel", "Value": map[string]any{"Selection": "None"}},
		map[string]any{"TypePointer": "t-trig", "Value": map[string]any{"PrimitiveValue": "double"}})

	got := parseRawWidget(ctx, w, "")
	if len(got) != 1 {
		t.Fatalf("parseRawWidget returned %d widgets, want 1", len(got))
	}
	if got[0].Selection != "None" {
		t.Errorf("Selection = %q, want None", got[0].Selection)
	}
	if got[0].OnClickTrigger != "double" {
		t.Errorf("OnClickTrigger = %q, want double", got[0].OnClickTrigger)
	}

	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got[0], 0)
	for _, want := range []string{"Selection: None", "onClickTrigger: double"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("describe has no %q:\n%s", want, buf.String())
		}
	}
}

// Control for the above: the defaults stay unsaid, so a gallery without a row
// action describes exactly as before.
func TestParseRawWidget_GalleryDefaultTriggerStaysUnsaid(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	w := buildGalleryActionWidget()
	typ := w["Type"].(map[string]any)["ObjectType"].(map[string]any)
	typ["PropertyTypes"] = append(typ["PropertyTypes"].([]any),
		map[string]any{"$ID": "t-trig", "PropertyKey": "onClickTrigger"})
	obj := w["Object"].(map[string]any)
	obj["Properties"] = append(obj["Properties"].([]any),
		map[string]any{"TypePointer": "t-trig", "Value": map[string]any{"PrimitiveValue": "single"}})
	got := parseRawWidget(ctx, w, "")
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got[0], 0)
	if strings.Contains(buf.String(), "onClickTrigger") {
		t.Errorf("the default trigger was described:\n%s", buf.String())
	}
}

// Once the row click is written, the Gallery's own editor check applies to it:
// Gallery 3.4.0's editorConfig reports "The item click action is ambiguous"
// (an error from `mx check`) when the gallery has a selection, an onClick, and
// the single-click trigger — and mxcli's defaults are Selection Single and
// trigger single. Measured on ako/TestApp (mx 11.14): the page built from
// `gallery g (DataSource: …, onClick: …)` fails the check. `check` says so first.
func TestValidateGalleryClickAmbiguity(t *testing.T) {
	act := &ast.ActionV3{Type: "nanoflow", Target: "M.Open"}
	cases := []struct {
		name  string
		props map[string]any
		want  bool
	}{
		{"defaults: selection single, trigger single", map[string]any{"Action": act}, true},
		{"explicit multi selection", map[string]any{"Action": act, "Selection": "Multi"}, true},
		{"selection none", map[string]any{"Action": act, "Selection": "None"}, false},
		{"double-click trigger", map[string]any{"Action": act, "onClickTrigger": "double"}, false},
		{"control: no onClick", map[string]any{"Selection": "Single"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := &ast.WidgetV3{Type: "gallery", Name: "g", Properties: c.props}
			got := validateGalleryClickAmbiguity(w, "page M.P")
			if (len(got) > 0) != c.want {
				t.Errorf("reported %v, want reported=%v", got, c.want)
			}
		})
	}
	// Not a gallery: a data grid has no such rule.
	dg := &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{"Action": act}}
	if got := validateGalleryClickAmbiguity(dg, "page M.P"); len(got) > 0 {
		t.Errorf("a data grid was reported: %v", got)
	}
}

// The two refusals end to end, through the path `check` and `exec` take:
// parse → ValidateWidgetProperties → the language gate. The unit tests above
// call each validator directly, so removing its call from the widget-tree walk
// — or the gate from ValidateWidgetProperties — left them green while `check`
// stopped reporting anything.
func TestWidgetActionRefusalsThroughCheck(t *testing.T) {
	const page = `create page W.P ( Title: 'P' )
{
  gallery g (DataSource: database from W.Card, onClick: call nanoflow W.Open) {
    template { dynamictext t (Content: 'x') }
  }
  combobox cb (Attribute: Name, onClick: call nanoflow W.Open)
};`
	cases := []struct {
		header       string
		galleryRule  string
		actionRule   string
		wantSeverity linter.Severity
	}{
		{"mdl 1;\n", "MDL-WIDGET36", "MDL-WIDGET37", linter.SeverityError},
		{"", galleryClickRefused.Code, actionSlotRefused.Code, linter.SeverityWarning},
	}
	for _, c := range cases {
		name := "mdl 0"
		if c.header != "" {
			name = "mdl 1"
		}
		t.Run(name, func(t *testing.T) {
			for _, rule := range []string{c.galleryRule, c.actionRule} {
				vs := widgetViolations(t, c.header+page, rule)
				if len(vs) != 1 {
					t.Fatalf("%s: got %d violations, want 1", rule, len(vs))
				}
				if vs[0].Severity != c.wantSeverity {
					t.Errorf("%s: severity %v, want %v", rule, vs[0].Severity, c.wantSeverity)
				}
			}
		})
	}
}
