// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-ledger FINDINGS #170 — `describe page` → `exec` rewrote two stored
// values MDL could not express:
//
//	Forms$TextBox / Forms$TextArea .SubmitBehaviour   WhileEditing → OnEndEditing
//	Forms$TextBox / Forms$TextArea .SubmitOnInputDelay  <n>        → 300
//	Forms$LayoutGrid.Width                             FixedWidth  → FullWidth
//
// The writer hard-coded all three; the model had no field; describe read and
// printed nothing; the builder consumed nothing. Worse, `check` accepted
// `SubmitBehaviour: WhileEditing` (MDL-WIDGET07 is a warning) and exec dropped
// it, and `Width: FixedWidth` on a layout grid drew not even the warning,
// because "Width" is in the cross-widget known-property union.
//
// These tests cover the executor's three layers: describe parse, describe
// output and the builder, plus the check-time refusal of an invalid value.
// The writer is covered in mdl/backend/modelsdk, and the end-to-end round trip
// over a stored page in widget_submit_width_pedapp_test.go.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestParseRawWidget_TextBoxSubmitBehaviour(t *testing.T) {
	ctx, _ := newMockCtx(t)
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea"} {
		got := parseRawWidget(ctx, map[string]any{
			"$Type": typ, "Name": "tb",
			"SubmitBehaviour": "WhileEditing", "SubmitOnInputDelay": int32(500),
		})
		if len(got) != 1 {
			t.Fatalf("%s: parsed %d widgets", typ, len(got))
		}
		if got[0].SubmitBehaviour != "WhileEditing" {
			t.Errorf("%s: SubmitBehaviour = %q, want WhileEditing", typ, got[0].SubmitBehaviour)
		}
		if got[0].SubmitOnInputDelay == nil || *got[0].SubmitOnInputDelay != 500 {
			t.Errorf("%s: SubmitOnInputDelay = %v, want 500", typ, got[0].SubmitOnInputDelay)
		}
	}
}

func TestParseRawWidget_LayoutGridWidth(t *testing.T) {
	ctx, _ := newMockCtx(t)
	got := parseRawWidget(ctx, map[string]any{"$Type": "Forms$LayoutGrid", "Name": "lg", "Width": "FixedWidth"})
	if got[0].LayoutGridWidth != "FixedWidth" {
		t.Errorf("LayoutGridWidth = %q, want FixedWidth", got[0].LayoutGridWidth)
	}
}

func TestDescribeTextInput_EmitsSubmitBehaviour(t *testing.T) {
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea"} {
		got := describeOneWidget(t, rawWidget{
			Type: typ, Name: "tb", Content: "Name",
			SubmitBehaviour: "WhileEditing", SubmitOnInputDelay: intPtr(500),
		})
		for _, want := range []string{"SubmitBehaviour: WhileEditing", "SubmitOnInputDelay: 500"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: describe should print %q, got:\n\t%s", typ, want, got)
			}
		}
	}
}

// Control: the defaults are what every ordinary input stores, so printing them
// would put a clause in every described page that its author never wrote.
func TestDescribeTextInput_OmitsDefaultSubmitBehaviour(t *testing.T) {
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea"} {
		for _, w := range []rawWidget{
			{Type: typ, Name: "tb", Content: "Name"},
			{Type: typ, Name: "tb", Content: "Name", SubmitBehaviour: "OnEndEditing", SubmitOnInputDelay: intPtr(300)},
		} {
			if got := describeOneWidget(t, w); strings.Contains(got, "Submit") {
				t.Errorf("%s: describe printed a default submit setting: %s", typ, got)
			}
		}
	}
}

func TestDescribeLayoutGrid_EmitsFixedWidth(t *testing.T) {
	got := describeOneWidget(t, rawWidget{Type: "Forms$LayoutGrid", Name: "lg", LayoutGridWidth: "FixedWidth"})
	if !strings.Contains(got, "layoutgrid lg (Width: FixedWidth)") {
		t.Errorf("describe dropped a layout grid's FixedWidth, got:\n%s", got)
	}
	for _, def := range []string{"", "FullWidth"} {
		got := describeOneWidget(t, rawWidget{Type: "Forms$LayoutGrid", Name: "lg", LayoutGridWidth: def})
		if strings.Contains(got, "Width") {
			t.Errorf("describe printed the default width for %q:\n%s", def, got)
		}
	}
}

func TestBuildTextInput_SubmitBehaviour(t *testing.T) {
	for _, tc := range []struct {
		name      string
		props     map[string]any
		behaviour string
		delay     *int
	}{
		{"unset keeps the defaults", map[string]any{}, "", nil},
		{"WhileEditing", map[string]any{"SubmitBehaviour": "WhileEditing"}, "WhileEditing", nil},
		{"case-insensitive, canonicalised", map[string]any{"SubmitBehaviour": "whileediting"}, "WhileEditing", nil},
		{"explicit OnEndEditing", map[string]any{"SubmitBehaviour": "OnEndEditing"}, "OnEndEditing", nil},
		{"delay", map[string]any{"SubmitBehaviour": "WhileEditing", "SubmitOnInputDelay": 500}, "WhileEditing", intPtr(500)},
		{"delay zero is a value, not unset", map[string]any{"SubmitOnInputDelay": 0}, "", intPtr(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb, err := newPropsBuilder().buildTextBoxV3(&ast.WidgetV3{Name: "tb", Type: "textbox", Properties: tc.props})
			if err != nil {
				t.Fatalf("buildTextBoxV3: %v", err)
			}
			ta, err := newPropsBuilder().buildTextAreaV3(&ast.WidgetV3{Name: "ta", Type: "textarea", Properties: tc.props})
			if err != nil {
				t.Fatalf("buildTextAreaV3: %v", err)
			}
			for kind, got := range map[string][2]any{
				"textbox":  {tb.SubmitBehaviour, tb.SubmitOnInputDelay},
				"textarea": {ta.SubmitBehaviour, ta.SubmitOnInputDelay},
			} {
				if got[0] != tc.behaviour {
					t.Errorf("%s: SubmitBehaviour = %q, want %q", kind, got[0], tc.behaviour)
				}
				d, _ := got[1].(*int)
				if (d == nil) != (tc.delay == nil) || (d != nil && *d != *tc.delay) {
					t.Errorf("%s: SubmitOnInputDelay = %v, want %v", kind, d, tc.delay)
				}
			}
		})
	}
}

func TestBuildTextInput_SubmitBehaviourRejectsInvalid(t *testing.T) {
	for _, props := range []map[string]any{
		{"SubmitBehaviour": "Immediately"},
		{"SubmitOnInputDelay": -1},
		{"SubmitOnInputDelay": "fast"},
	} {
		if _, err := newPropsBuilder().buildTextBoxV3(&ast.WidgetV3{Name: "tb", Type: "textbox", Properties: props}); err == nil {
			t.Errorf("textbox %v was accepted", props)
		}
		if _, err := newPropsBuilder().buildTextAreaV3(&ast.WidgetV3{Name: "ta", Type: "textarea", Properties: props}); err == nil {
			t.Errorf("textarea %v was accepted", props)
		}
	}
}

func TestBuildLayoutGrid_Width(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{nil, ""},
		{"FixedWidth", "FixedWidth"},
		{"fixedwidth", "FixedWidth"},
		{"FullWidth", "FullWidth"},
	} {
		w := &ast.WidgetV3{Name: "lg", Type: "layoutgrid", Properties: map[string]any{}}
		if tc.value != nil {
			w.Properties["Width"] = tc.value
		}
		lg, err := newPropsBuilder().buildLayoutGridV3(w)
		if err != nil {
			t.Fatalf("%v: %v", tc.value, err)
		}
		if lg.Width != tc.want {
			t.Errorf("%v: Width = %q, want %q", tc.value, lg.Width, tc.want)
		}
	}
	if _, err := newPropsBuilder().buildLayoutGridV3(&ast.WidgetV3{
		Name: "lg", Type: "layoutgrid", Properties: map[string]any{"Width": "Wide"},
	}); err == nil {
		t.Error("layoutgrid Width: Wide was accepted")
	}
}

// check must refuse what exec refuses — the same function decides both.
func TestValidateWidgets_SubmitAndWidthValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		w       *ast.WidgetV3
		wantErr string // "" = no MDL-WIDGET18 / MDL-WIDGET07
	}{
		{"valid textbox", &ast.WidgetV3{Name: "tb", Type: "textbox", Properties: map[string]any{"SubmitBehaviour": "WhileEditing", "SubmitOnInputDelay": 500}}, ""},
		{"valid textarea", &ast.WidgetV3{Name: "ta", Type: "textarea", Properties: map[string]any{"SubmitBehaviour": "WhileEditing"}}, ""},
		{"valid grid", &ast.WidgetV3{Name: "lg", Type: "layoutgrid", Properties: map[string]any{"Width": "FixedWidth"}}, ""},
		{"bad behaviour", &ast.WidgetV3{Name: "tb", Type: "textbox", Properties: map[string]any{"SubmitBehaviour": "Immediately"}}, "SubmitBehaviour"},
		{"bad delay", &ast.WidgetV3{Name: "ta", Type: "textarea", Properties: map[string]any{"SubmitOnInputDelay": -5}}, "SubmitOnInputDelay"},
		{"bad grid width", &ast.WidgetV3{Name: "lg", Type: "layoutgrid", Properties: map[string]any{"Width": "Wide"}}, "Width"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errs []string
			for _, v := range validateInputSubmitAndGridWidth(tc.w, "page M.P") {
				errs = append(errs, v.Message)
			}
			for _, v := range validateStaticWidgetUnknownProps(tc.w, "page M.P") {
				errs = append(errs, v.Message)
			}
			if tc.wantErr == "" && len(errs) > 0 {
				t.Errorf("unexpected diagnostics: %v", errs)
			}
			if tc.wantErr != "" && (len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), tc.wantErr)) {
				t.Errorf("want a diagnostic naming %s, got %v", tc.wantErr, errs)
			}
		})
	}
}

// The submit keys are a text box's and a text area's. On any other widget no
// builder reads them, so they must keep drawing MDL-WIDGET07 rather than join
// the cross-widget union and pass in silence.
func TestValidateWidgets_SubmitBehaviourOnOtherWidgetStillWarns(t *testing.T) {
	w := &ast.WidgetV3{Name: "dp", Type: "datepicker", Properties: map[string]any{"SubmitBehaviour": "WhileEditing"}}
	if len(validateStaticWidgetUnknownProps(w, "page M.P")) == 0 {
		t.Error("SubmitBehaviour on a date picker is dropped on write and must warn MDL-WIDGET07")
	}
}
